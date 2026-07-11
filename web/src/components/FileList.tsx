import { File as FileIcon, X, CheckCircle2, AlertCircle, Loader2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Progress } from "@/components/ui/progress";
import { formatBytes } from "@/lib/utils";
import type { UploadEntry } from "@/types";

function StatusBadge({ entry }: { entry: UploadEntry }) {
  const s = entry.status;
  switch (s.phase) {
    case "pending":
      return <span className="text-xs text-muted-foreground">等待中</span>;
    case "hashing":
      return (
        <span className="flex items-center gap-1 text-xs text-muted-foreground">
          <Loader2 className="h-3 w-3 animate-spin" />
          计算哈希…
        </span>
      );
    case "uploading": {
      const pct = s.total > 0 ? Math.round((s.uploaded / s.total) * 100) : 0;
      return (
        <div className="flex w-40 items-center gap-2">
          <Progress value={pct} className="h-2" />
          <span className="w-9 text-right text-xs tabular-nums text-muted-foreground">
            {pct}%
          </span>
        </div>
      );
    }
    case "done":
      return (
        <span className="flex items-center gap-1 text-xs text-emerald-600">
          <CheckCircle2 className="h-3.5 w-3.5" />
          完成
        </span>
      );
    case "error":
      return (
        <span
          className="flex items-center gap-1 text-xs text-destructive"
          title={s.message}
        >
          <AlertCircle className="h-3.5 w-3.5" />
          失败
        </span>
      );
  }
}

interface FileListProps {
  entries: UploadEntry[];
  onRemove?: (id: string) => void;
  removable: boolean;
}

export function FileList({ entries, onRemove, removable }: FileListProps) {
  if (entries.length === 0) return null;

  return (
    <ul className="divide-y rounded-lg border">
      {entries.map((e) => (
        <li
          key={e.entry.id}
          className="flex items-center gap-3 px-4 py-3"
        >
          <FileIcon className="h-5 w-5 shrink-0 text-muted-foreground" />
          <div className="min-w-0 flex-1">
            <p className="truncate text-sm font-medium">{e.entry.file.name}</p>
            <p className="truncate text-xs text-muted-foreground">
              {e.entry.relativePath}
              {e.entry.relativePath !== "./" ? "" : ""} ·{" "}
              {formatBytes(e.entry.file.size)}
              {e.status.phase === "error" ? ` · ${e.status.message}` : ""}
            </p>
          </div>
          <StatusBadge entry={e} />
          {removable && onRemove && (
            <Button
              type="button"
              variant="ghost"
              size="icon"
              className="h-8 w-8 shrink-0"
              onClick={() => onRemove(e.entry.id)}
              aria-label={`移除 ${e.entry.file.name}`}
            >
              <X className="h-4 w-4" />
            </Button>
          )}
        </li>
      ))}
    </ul>
  );
}
