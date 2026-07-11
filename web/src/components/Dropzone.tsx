import * as React from "react";
import { Upload, FolderUp, FilePlus } from "lucide-react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import type { FileEntry } from "@/types";

// Normalize a browser relative path (e.g. "folder/sub/a.txt") into the
// BigFile share `path` convention: "./" for root, "./folder/sub/" otherwise.
function toSharePath(relativePath: string): string {
  const parts = relativePath.split("/").filter(Boolean);
  parts.pop(); // drop the file name
  if (parts.length === 0) return "./";
  return `./${parts.join("/")}/`;
}

let idCounter = 0;
function makeEntry(file: File, rawPath: string): FileEntry {
  return {
    id: `f${Date.now()}_${idCounter++}`,
    file,
    relativePath: toSharePath(rawPath),
  };
}

// Recursively read a dropped directory tree via the DataTransferItem API.
async function readDirectoryEntry(
  entry: FileSystemDirectoryEntry,
  basePath: string
): Promise<FileEntry[]> {
  const reader = entry.createReader();
  const out: FileEntry[] = [];

  const readBatch = (): Promise<FileSystemEntry[]> =>
    new Promise((resolve, reject) =>
      reader.readEntries(resolve, reject)
    );

  // readEntries returns results in batches; loop until empty.
  for (;;) {
    const batch = await readBatch();
    if (batch.length === 0) break;
    for (const child of batch) {
      out.push(...(await readEntry(child, basePath)));
    }
  }
  return out;
}

async function readEntry(
  entry: FileSystemEntry,
  basePath: string
): Promise<FileEntry[]> {
  const path = basePath ? `${basePath}/${entry.name}` : entry.name;
  if (entry.isFile) {
    const fileEntry = entry as FileSystemFileEntry;
    const file = await new Promise<File>((resolve, reject) =>
      fileEntry.file(resolve, reject)
    );
    return [makeEntry(file, path)];
  }
  return readDirectoryEntry(entry as FileSystemDirectoryEntry, path);
}

async function collectFromDataTransfer(
  dt: DataTransfer
): Promise<FileEntry[]> {
  const items = Array.from(dt.items).filter((i) => i.kind === "file");
  const entryGetter = items.map((i) =>
    // webkitGetAsEntry is widely supported despite the prefix.
    (i as DataTransferItem & {
      webkitGetAsEntry?: () => FileSystemEntry | null;
    }).webkitGetAsEntry?.()
  );

  // If the browser exposes the entry API, use it to preserve folder structure.
  if (entryGetter.some(Boolean)) {
    const results: FileEntry[] = [];
    for (const entry of entryGetter) {
      if (entry) results.push(...(await readEntry(entry, "")));
    }
    if (results.length) return results;
  }

  // Fallback: flat file list, no folder structure.
  return Array.from(dt.files).map((f) => makeEntry(f, f.name));
}

interface DropzoneProps {
  onAdd: (entries: FileEntry[]) => void;
  disabled?: boolean;
}

export function Dropzone({ onAdd, disabled }: DropzoneProps) {
  const [dragging, setDragging] = React.useState(false);
  const fileInputRef = React.useRef<HTMLInputElement>(null);
  const folderInputRef = React.useRef<HTMLInputElement>(null);

  const handleDrop = async (e: React.DragEvent) => {
    e.preventDefault();
    setDragging(false);
    if (disabled) return;
    const entries = await collectFromDataTransfer(e.dataTransfer);
    if (entries.length) onAdd(entries);
  };

  const handleFileInput = (e: React.ChangeEvent<HTMLInputElement>) => {
    const files = Array.from(e.target.files ?? []);
    const entries = files.map((f) =>
      // Folder picker sets webkitRelativePath; plain picker leaves it empty.
      makeEntry(f, f.webkitRelativePath || f.name)
    );
    if (entries.length) onAdd(entries);
    e.target.value = ""; // allow re-selecting the same file
  };

  return (
    <div
      onDragOver={(e) => {
        e.preventDefault();
        if (!disabled) setDragging(true);
      }}
      onDragLeave={(e) => {
        e.preventDefault();
        setDragging(false);
      }}
      onDrop={handleDrop}
      className={cn(
        "flex flex-col items-center justify-center gap-4 rounded-lg border-2 border-dashed p-10 text-center transition-colors",
        dragging
          ? "border-primary bg-accent"
          : "border-border bg-muted/30",
        disabled && "pointer-events-none opacity-60"
      )}
    >
      <div className="flex h-14 w-14 items-center justify-center rounded-full bg-primary/10">
        <Upload className="h-7 w-7 text-primary" />
      </div>
      <div className="space-y-1">
        <p className="text-base font-medium">拖放文件或文件夹到此处</p>
        <p className="text-sm text-muted-foreground">
          或使用下方按钮选择。文件夹结构会保留在分享清单中。
        </p>
      </div>
      <div className="flex flex-wrap items-center justify-center gap-3">
        <Button
          type="button"
          variant="default"
          disabled={disabled}
          onClick={() => fileInputRef.current?.click()}
        >
          <FilePlus className="h-4 w-4" />
          选择文件
        </Button>
        <Button
          type="button"
          variant="outline"
          disabled={disabled}
          onClick={() => folderInputRef.current?.click()}
        >
          <FolderUp className="h-4 w-4" />
          选择文件夹
        </Button>
      </div>

      <input
        ref={fileInputRef}
        type="file"
        multiple
        hidden
        onChange={handleFileInput}
      />
      <input
        ref={folderInputRef}
        type="file"
        hidden
        onChange={handleFileInput}
        // Non-standard attributes for directory selection.
        {...({
          webkitdirectory: "",
          directory: "",
        } as React.InputHTMLAttributes<HTMLInputElement>)}
      />
    </div>
  );
}
