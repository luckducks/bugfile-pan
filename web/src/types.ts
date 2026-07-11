// ---------------------------------------------------------------------------
// Domain types shared across the upload UI
// ---------------------------------------------------------------------------

/** One file collected from a picker or drag-drop. */
export interface FileEntry {
  /** Unique ID within this session. */
  id: string;
  /** Raw browser File object. */
  file: File;
  /**
   * Relative path from the share root, e.g. `"./"` for top-level files or
   * `"./subfolder/"` for files inside a folder. Derived from
   * `File.webkitRelativePath` when the user picks a whole folder.
   */
  relativePath: string;
}

/** Per-file upload state. */
export type FileUploadStatus =
  | { phase: "pending" }
  | { phase: "hashing" }
  | { phase: "uploading"; uploaded: number; total: number }
  | { phase: "done"; hash: string }
  | { phase: "error"; message: string };

/** Entry tracked in the upload queue. */
export interface UploadEntry {
  entry: FileEntry;
  status: FileUploadStatus;
}

/** The share JSON item matching BigFile's list.json schema. */
export interface ShareFileItem {
  name: string;
  size: number;
  type: string;
  hash: string;
  path: string;
}

/** Final share result after all uploads complete. */
export interface ShareResult {
  shareHash: string;
  shareUrl: string;
  items: ShareFileItem[];
}

/** Overall page state machine. */
export type AppPhase =
  | "idle"
  | "ready"
  | "uploading"
  | "creating_share"
  | "done"
  | "error";
