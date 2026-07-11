// ---------------------------------------------------------------------------
// BigFile upload client (browser side)
//
// Implements the reverse-engineered protocol from BIGFILE_API_DOC.md:
//   - Files <= 96MB: single POST /v1/upload with Hash + Content-Range.
//   - Files  > 96MB: chunked upload of 96MB pieces. First chunk carries no
//     Hash/Next; middle chunks carry the Next token from the previous reply;
//     the final chunk carries both Next and the full-file Hash.
//   - Content-Range is the inclusive byte range "{start}-{end}".
//   - The body is the raw bytes (Blob), NOT multipart/form-data.
//   - A share is created by uploading a list.json describing the files.
//
// Browser uploads use the same-origin /api/upload proxy because BigFile's
// upload node rejects cross-origin preflight requests. The Vite dev server and
// the Go backend both expose that route.
// ---------------------------------------------------------------------------

import { hashBlob } from "./hash";
import type { ShareFileItem } from "../types";

export const CHUNK_SIZE = 96 * 1024 * 1024; // 96 MiB

export interface UploadClientOptions {
  /** Base URL for the upload proxy, e.g. "" for same origin or
   * "https://your-backend.example". The client POSTs to `${uploadBase}/api/upload`. */
  uploadBase?: string;
  /** Base URL used to build share/download links shown to the user. */
  siteBase?: string;
}

interface UploadResponse {
  status: boolean;
  uri?: string;
  next?: string;
  message?: string;
}

export interface UploadProgress {
  /** Bytes confirmed uploaded so far. */
  uploaded: number;
  /** Total bytes for this file. */
  total: number;
}

function parseResponse(text: string): UploadResponse {
  let data: UploadResponse;
  try {
    data = JSON.parse(text) as UploadResponse;
  } catch {
    throw new Error(`上传响应不是合法 JSON: ${text.slice(0, 200)}`);
  }
  if (data.status === false) {
    throw new Error(data.message || "上传失败");
  }
  return data;
}

// Single POST with XHR so we get upload progress events.
function postChunk(
  url: string,
  body: Blob,
  headers: Record<string, string>,
  onProgress?: (loadedInChunk: number) => void,
  signal?: AbortSignal
): Promise<UploadResponse> {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    xhr.open("POST", url, true);
    for (const [k, v] of Object.entries(headers)) {
      xhr.setRequestHeader(k, v);
    }
    xhr.responseType = "text";

    if (onProgress) {
      xhr.upload.onprogress = (e) => {
        if (e.lengthComputable) onProgress(e.loaded);
      };
    }

    xhr.onload = () => {
      if (xhr.status >= 200 && xhr.status < 300) {
        try {
          resolve(parseResponse(xhr.responseText));
        } catch (err) {
          reject(err);
        }
      } else {
        reject(new Error(`HTTP ${xhr.status}: ${xhr.responseText.slice(0, 200)}`));
      }
    };
    xhr.onerror = () =>
      reject(new Error("网络错误：无法连接上传节点（可能是 CORS 或代理未配置）"));
    xhr.ontimeout = () => reject(new Error("上传超时"));
    xhr.onabort = () => reject(new DOMException("Aborted", "AbortError"));

    if (signal) {
      if (signal.aborted) {
        xhr.abort();
        return;
      }
      signal.addEventListener("abort", () => xhr.abort(), { once: true });
    }

    xhr.send(body);
  });
}

export class BigFileClient {
  private uploadBase: string;
  readonly siteBase: string;

  constructor(opts: UploadClientOptions = {}) {
    this.uploadBase = (opts.uploadBase ?? "").replace(/\/$/, "");
    this.siteBase = (opts.siteBase ?? "").replace(/\/$/, "");
  }

  private get uploadUrl(): string {
    return `${this.uploadBase}/api/upload`;
  }

  /**
   * Upload one file (or Blob) and return its BigFile hash.
   * Automatically chooses single vs chunked upload based on size.
   */
  async uploadFile(
    file: Blob,
    onProgress?: (p: UploadProgress) => void,
    signal?: AbortSignal
  ): Promise<string> {
    const total = file.size;

    // Compute the full-file hash up front. The single-shot path needs it in the
    // Hash header, and the chunked path needs it on the final chunk.
    const hash = await hashBlob(file, signal);

    if (total <= CHUNK_SIZE) {
      const range = total === 0 ? "0-0" : `0-${total - 1}`;
      const resp = await postChunk(
        this.uploadUrl,
        file,
        { "Content-Range": range, Hash: hash },
        (loaded) => onProgress?.({ uploaded: Math.min(loaded, total), total }),
        signal
      );
      onProgress?.({ uploaded: total, total });
      this.assertUri(resp);
      return hash;
    }

    // Chunked upload.
    let next: string | undefined;
    let offset = 0;
    while (offset < total) {
      const end = Math.min(offset + CHUNK_SIZE, total);
      const chunk = file.slice(offset, end);
      const isLast = end >= total;
      const range = `${offset}-${end - 1}`;

      const headers: Record<string, string> = { "Content-Range": range };
      if (next) headers["Next"] = next;
      if (isLast) headers["Hash"] = hash;

      const base = offset;
      const resp = await postChunk(
        this.uploadUrl,
        chunk,
        headers,
        (loaded) =>
          onProgress?.({ uploaded: Math.min(base + loaded, total), total }),
        signal
      );

      if (isLast) {
        onProgress?.({ uploaded: total, total });
        this.assertUri(resp);
        return hash;
      }
      if (!resp.next) {
        throw new Error("分片响应缺少 next token");
      }
      next = resp.next;
      offset = end;
    }
    return hash;
  }

  /**
   * Create a share by uploading the list.json describing the given items.
   * Returns the share hash and share URL.
   */
  async createShare(
    items: ShareFileItem[],
    signal?: AbortSignal
  ): Promise<{ shareHash: string; shareUrl: string }> {
    const json = JSON.stringify(items, null, 2);
    const blob = new Blob([json], { type: "application/json" });
    const shareHash = await this.uploadFile(blob, undefined, signal);
    return {
      shareHash,
      shareUrl: `${this.siteBase || "https://www.bigfile.net"}/s/${shareHash}`,
    };
  }

  /** Build a direct download URL for a file. */
  fileUrl(hash: string, filename: string, contentType: string): string {
    const site = this.siteBase || "https://www.bigfile.net";
    const q = new URLSearchParams({ "content-type": contentType });
    return `${site}/d/${encodeURIComponent(hash)}/${encodeURIComponent(filename)}?${q.toString()}`;
  }

  private assertUri(resp: UploadResponse) {
    if (!resp.uri) {
      throw new Error("上传完成但响应缺少 uri");
    }
  }
}
