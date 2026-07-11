import { useState } from "react";
import {
  CheckCircle2,
  Copy,
  ExternalLink,
  Server,
  ChevronDown,
  ChevronUp,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import type { ShareResult } from "@/types";

interface ShareResultPanelProps {
  result: ShareResult;
}

export function ShareResultPanel({ result }: ShareResultPanelProps) {
  const [copied, setCopied] = useState(false);
  const [showFiles, setShowFiles] = useState(false);

  const copy = async () => {
    await navigator.clipboard.writeText(result.shareUrl);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <Card className="border-emerald-200 bg-emerald-50">
      <CardHeader className="pb-3">
        <CardTitle className="flex items-center gap-2 text-emerald-700">
          <CheckCircle2 className="h-5 w-5" />
          上传完成！分享链接已生成
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        {/* Share URL row */}
        <div className="flex items-center gap-2 rounded-md border bg-white px-3 py-2 text-sm font-mono break-all">
          <span className="flex-1 select-all text-muted-foreground">
            {result.shareUrl}
          </span>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            className="h-8 shrink-0"
            onClick={copy}
            aria-label="复制链接"
          >
            {copied ? (
              <CheckCircle2 className="h-4 w-4 text-emerald-600" />
            ) : (
              <Copy className="h-4 w-4" />
            )}
          </Button>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            className="h-8 shrink-0"
            asChild
          >
            <a href={result.shareUrl} target="_blank" rel="noopener noreferrer">
              <ExternalLink className="h-4 w-4" />
            </a>
          </Button>
        </div>

        {/* WebDAV hint */}
        <div className="flex items-start gap-2 rounded-md bg-white/70 px-3 py-2 text-sm text-muted-foreground">
          <Server className="mt-0.5 h-4 w-4 shrink-0 text-sky-500" />
          <div>
            <p className="font-medium text-foreground">
              配置到 WebDAV 网关
            </p>
            <p className="mt-0.5 font-mono text-xs">
              BIGFILE_SHARE_HASH=
              <span className="select-all text-sky-700">{result.shareHash}</span>
            </p>
          </div>
        </div>

        {/* File list toggle */}
        <button
          type="button"
          className="flex w-full items-center justify-between text-sm text-muted-foreground hover:text-foreground"
          onClick={() => setShowFiles((v) => !v)}
        >
          <span>共 {result.items.length} 个文件</span>
          {showFiles ? (
            <ChevronUp className="h-4 w-4" />
          ) : (
            <ChevronDown className="h-4 w-4" />
          )}
        </button>
        {showFiles && (
          <ul className="max-h-56 overflow-y-auto divide-y rounded-md border bg-white text-xs">
            {result.items.map((item) => (
              <li key={`${item.path}${item.name}`} className="flex items-center gap-2 px-3 py-2">
                <span className="min-w-0 flex-1 truncate font-medium">
                  {item.path !== "./" && (
                    <span className="text-muted-foreground">{item.path}</span>
                  )}
                  {item.name}
                </span>
                <span className="shrink-0 tabular-nums text-muted-foreground">
                  {(item.size / 1024).toFixed(1)} KB
                </span>
              </li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  );
}
