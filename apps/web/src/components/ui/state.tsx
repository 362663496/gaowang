import type * as React from "react";
import { AlertCircle, Inbox, Loader2 } from "lucide-react";
import { Button } from "@/components/ui/button";

export function LoadingBlock({ label = "加载中" }: { label?: string }) {
  return (
    <div className="flex min-h-40 items-center justify-center rounded-lg border border-[var(--border-subtle)] bg-white text-sm text-[var(--text-secondary)]">
      <Loader2 className="mr-2 h-4 w-4 animate-spin" />
      {label}
    </div>
  );
}

export function EmptyBlock({ title, action }: { title: string; action?: React.ReactNode }) {
  return (
    <div className="grid min-h-40 place-items-center rounded-lg border border-dashed border-[var(--border-subtle)] bg-white px-6 py-10 text-center">
      <div>
        <Inbox className="mx-auto h-7 w-7 text-[var(--text-muted)]" />
        <div className="mt-3 font-medium">{title}</div>
        {action ? <div className="mt-4">{action}</div> : null}
      </div>
    </div>
  );
}

export function ErrorBlock({ message, onRetry }: { message: string; onRetry?: () => void }) {
  return (
    <div className="rounded-lg border border-red-200 bg-red-50 p-4 text-sm text-red-800">
      <div className="flex items-start gap-2">
        <AlertCircle className="mt-0.5 h-4 w-4 shrink-0" />
        <div className="min-w-0 flex-1">{message}</div>
        {onRetry ? (
          <Button size="sm" type="button" variant="secondary" onClick={onRetry}>
            重试
          </Button>
        ) : null}
      </div>
    </div>
  );
}
