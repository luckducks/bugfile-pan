import { useCallback, useReducer, useRef } from "react";
import { Trash2, Upload, RotateCcw } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Progress } from "@/components/ui/progress";
import { Dropzone } from "@/components/Dropzone";
import { FileList } from "@/components/FileList";
import { ShareResultPanel } from "@/components/ShareResultPanel";
import { BigFileClient } from "@/lib/bigfile";
import { formatBytes } from "@/lib/utils";
import type {
  AppPhase,
  FileEntry,
  ShareFileItem,
  ShareResult,
  UploadEntry,
} from "@/types";

// ---------------------------------------------------------------------------
// State
// ---------------------------------------------------------------------------

interface State {
  phase: AppPhase;
  entries: UploadEntry[];
  shareResult: ShareResult | null;
  errorMsg: string | null;
}

type Action =
  | { type: "ADD_FILES"; entries: FileEntry[] }
  | { type: "REMOVE_FILE"; id: string }
  | { type: "START_UPLOAD" }
  | { type: "FILE_HASHING"; id: string }
  | { type: "FILE_PROGRESS"; id: string; uploaded: number; total: number }
  | { type: "FILE_DONE"; id: string; hash: string }
  | { type: "FILE_ERROR"; id: string; message: string }
  | { type: "CREATING_SHARE" }
  | { type: "DONE"; result: ShareResult }
  | { type: "ERROR"; message: string }
  | { type: "RESET" };

function reducer(state: State, action: Action): State {
  switch (action.type) {
    case "ADD_FILES": {
      const existingPaths = new Set(
        state.entries.map((e) => `${e.entry.relativePath}\0${e.entry.file.name}`)
      );
      const additions = action.entries.filter((entry) => {
        const key = `${entry.relativePath}\0${entry.file.name}`;
        if (existingPaths.has(key)) return false;
        existingPaths.add(key);
        return true;
      });
      if (additions.length === 0) return state;
      return {
        ...state,
        phase: "ready",
        entries: [
          ...state.entries,
          ...additions.map(
            (e): UploadEntry => ({ entry: e, status: { phase: "pending" } })
          ),
        ],
      };
    }
    case "REMOVE_FILE":
      return {
        ...state,
        entries: state.entries.filter((e) => e.entry.id !== action.id),
        phase:
          state.entries.length <= 1
            ? "idle"
            : state.phase,
      };
    case "START_UPLOAD":
      return { ...state, phase: "uploading" };
    case "FILE_HASHING":
      return {
        ...state,
        entries: state.entries.map((e) =>
          e.entry.id === action.id
            ? { ...e, status: { phase: "hashing" } }
            : e
        ),
      };
    case "FILE_PROGRESS":
      return {
        ...state,
        entries: state.entries.map((e) =>
          e.entry.id === action.id
            ? {
                ...e,
                status: {
                  phase: "uploading",
                  uploaded: action.uploaded,
                  total: action.total,
                },
              }
            : e
        ),
      };
    case "FILE_DONE":
      return {
        ...state,
        entries: state.entries.map((e) =>
          e.entry.id === action.id
            ? { ...e, status: { phase: "done", hash: action.hash } }
            : e
        ),
      };
    case "FILE_ERROR":
      return {
        ...state,
        entries: state.entries.map((e) =>
          e.entry.id === action.id
            ? { ...e, status: { phase: "error", message: action.message } }
            : e
        ),
      };
    case "CREATING_SHARE":
      return { ...state, phase: "creating_share" };
    case "DONE":
      return { ...state, phase: "done", shareResult: action.result };
    case "ERROR":
      return { ...state, phase: "error", errorMsg: action.message };
    case "RESET":
      return { phase: "idle", entries: [], shareResult: null, errorMsg: null };
    default:
      return state;
  }
}

const INITIAL_STATE: State = {
  phase: "idle",
  entries: [],
  shareResult: null,
  errorMsg: null,
};

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// Vite and the Go service expose the same /api/upload route. Set
// VITE_UPLOAD_BASE only when the API is hosted on a different origin.
const client = new BigFileClient({
  uploadBase: import.meta.env.VITE_UPLOAD_BASE ?? "",
  siteBase: import.meta.env.VITE_SITE_BASE ?? "https://www.bigfile.net",
});

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export default function App() {
  const [state, dispatch] = useReducer(reducer, INITIAL_STATE);
  const abortRef = useRef<AbortController | null>(null);

  const isActive =
    state.phase === "uploading" || state.phase === "creating_share";
  const canUpload = state.phase === "ready" && state.entries.length > 0;

  // Overall progress percentage
  const progressStats = (() => {
    const done = state.entries.filter((e) => e.status.phase === "done").length;
    return {
      done,
      total: state.entries.length,
      pct: state.entries.length === 0 ? 0 : Math.round((done / state.entries.length) * 100),
    };
  })();

  const handleAdd = useCallback((entries: FileEntry[]) => {
    dispatch({ type: "ADD_FILES", entries });
  }, []);

  const handleRemove = useCallback((id: string) => {
    dispatch({ type: "REMOVE_FILE", id });
  }, []);

  const handleUpload = async () => {
    const entries = state.entries;
    if (!entries.length) return;

    const abort = new AbortController();
    abortRef.current = abort;
    dispatch({ type: "START_UPLOAD" });

    // Upload files concurrently (up to 3 at a time).
    const shareItems: ShareFileItem[] = [];
    const failed: string[] = [];
    const queue = [...entries];
    const CONCURRENCY = 3;

    async function worker() {
      let entry: UploadEntry | undefined;
      while ((entry = queue.shift())) {
        const { entry: fe } = entry;
        dispatch({ type: "FILE_HASHING", id: fe.id });
        try {
          const hash = await client.uploadFile(
            fe.file,
            (p) =>
              dispatch({
                type: "FILE_PROGRESS",
                id: fe.id,
                uploaded: p.uploaded,
                total: p.total,
              }),
            abort.signal
          );
          dispatch({ type: "FILE_DONE", id: fe.id, hash });
          shareItems.push({
            name: fe.file.name,
            size: fe.file.size,
            type: fe.file.type || "application/octet-stream",
            hash,
            path: fe.relativePath,
          });
        } catch (err) {
          if (abort.signal.aborted) return;
          const msg = err instanceof Error ? err.message : String(err);
          dispatch({ type: "FILE_ERROR", id: fe.id, message: msg });
          failed.push(fe.file.name);
        }
      }
    }

    await Promise.all(Array.from({ length: CONCURRENCY }, () => worker()));

    if (abort.signal.aborted) return;
    if (failed.length > 0) {
      dispatch({
        type: "ERROR",
        message: `${failed.length} 个文件上传失败，未创建不完整的分享`,
      });
      return;
    }

    // Only create a share after every selected file has uploaded successfully.
    dispatch({ type: "CREATING_SHARE" });
    try {
      const { shareHash, shareUrl } = await client.createShare(
        shareItems,
        abort.signal
      );
      dispatch({
        type: "DONE",
        result: { shareHash, shareUrl, items: shareItems },
      });
    } catch (err) {
      if (!abort.signal.aborted) {
        const msg = err instanceof Error ? err.message : String(err);
        dispatch({ type: "ERROR", message: `创建分享失败: ${msg}` });
      }
    }
  };

  const handleReset = () => {
    abortRef.current?.abort();
    dispatch({ type: "RESET" });
  };

  const totalBytes = state.entries.reduce(
    (acc, e) => acc + e.entry.file.size,
    0
  );

  return (
    <div className="mx-auto max-w-2xl px-4 py-10 space-y-6">
      {/* Header */}
      <div className="space-y-1">
        <h1 className="text-3xl font-bold tracking-tight">BigFile 上传</h1>
        <p className="text-muted-foreground text-sm">
          上传文件或文件夹，自动生成分享链接与 WebDAV 网关配置。
        </p>
      </div>

      {/* Error banner */}
      {state.phase === "error" && (
        <div className="rounded-md bg-destructive/10 border border-destructive/30 px-4 py-3 text-sm text-destructive">
          {state.errorMsg}
        </div>
      )}

      {/* Share result */}
      {state.shareResult && (
        <ShareResultPanel result={state.shareResult} />
      )}

      {/* Drop zone — hide after upload done */}
      {state.phase !== "done" && (
        <Card>
          <CardHeader className="pb-4">
            <CardTitle className="text-lg">选择文件</CardTitle>
            <CardDescription>
              支持拖放单个文件、多个文件或整个文件夹。
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <Dropzone onAdd={handleAdd} disabled={isActive} />

            {/* File list */}
            {state.entries.length > 0 && (
              <>
                <div className="flex items-center justify-between text-sm">
                  <span className="text-muted-foreground">
                    {state.entries.length} 个文件 · {formatBytes(totalBytes)}
                  </span>
                  {!isActive && (
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      className="h-8 gap-1 text-muted-foreground hover:text-destructive"
                      onClick={() => dispatch({ type: "RESET" })}
                    >
                      <Trash2 className="h-3.5 w-3.5" />
                      清空
                    </Button>
                  )}
                </div>
                <FileList
                  entries={state.entries}
                  onRemove={handleRemove}
                  removable={!isActive}
                />
              </>
            )}

            {/* Upload progress bar */}
            {isActive && (
              <div className="space-y-1.5">
                <div className="flex items-center justify-between text-xs text-muted-foreground">
                  <span>
                    {state.phase === "creating_share"
                      ? "正在创建分享…"
                      : `已上传 ${progressStats.done} / ${progressStats.total} 个文件`}
                  </span>
                  <span className="tabular-nums">{progressStats.pct}%</span>
                </div>
                <Progress
                  value={
                    state.phase === "creating_share" ? 99 : progressStats.pct
                  }
                />
              </div>
            )}

            {/* Action buttons */}
            <div className="flex gap-3 pt-1">
              {state.phase !== "error" && (
                <Button
                  type="button"
                  disabled={!canUpload || isActive}
                  onClick={handleUpload}
                  className="gap-2"
                >
                  <Upload className="h-4 w-4" />
                  {isActive ? "上传中…" : "开始上传"}
                </Button>
              )}
              {(isActive ||
                state.phase === "error" ||
                state.phase === "ready") && (
                <Button
                  type="button"
                  variant="outline"
                  onClick={handleReset}
                >
                  <RotateCcw className="h-4 w-4" />
                  {isActive ? "取消" : "重置"}
                </Button>
              )}
            </div>
          </CardContent>
        </Card>
      )}

      {state.phase === "done" && (
        <Button
          type="button"
          variant="outline"
          onClick={handleReset}
          className="gap-2"
        >
          <RotateCcw className="h-4 w-4" />
          重新上传
        </Button>
      )}

      {/* Footer hint */}
      <p className="text-center text-xs text-muted-foreground">
        文件由本地服务流式转发至 BigFile.net，不会临时落盘。
      </p>
    </div>
  );
}
