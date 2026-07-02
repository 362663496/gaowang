"use client";

import { useCallback, useEffect, useState } from "react";
import { Play } from "lucide-react";
import { PageHeader } from "@/components/layout/page-header";
import { Button } from "@/components/ui/button";
import { MessageBar } from "@/components/ui/message";
import { EmptyBlock, ErrorBlock, LoadingBlock } from "@/components/ui/state";
import { BackupStatusBadge } from "@/features/labels";
import type { BackupJob } from "@/features/types";
import { useMessage } from "@/features/use-message";
import { apiGet, apiPost } from "@/lib/api";
import { formatDateTime, formatFileSize } from "@/lib/format";

export default function BackupsPage() {
  const [job, setJob] = useState<BackupJob | null>(null);
  const [loading, setLoading] = useState(true);
  const [running, setRunning] = useState(false);
  const [error, setError] = useState("");
  const { message, show } = useMessage();

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      setJob((await apiGet<{ job: BackupJob | null }>("/backups/latest")).job);
    } catch (err) {
      setError(err instanceof Error ? err.message : "加载失败");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  async function runBackup() {
    setRunning(true);
    setError("");
    try {
      setJob((await apiPost<{ job: BackupJob }>("/backups/run")).job);
      show("备份任务已完成");
    } catch (err) {
      setError(err instanceof Error ? err.message : "备份失败");
      await load();
    } finally {
      setRunning(false);
    }
  }

  return (
    <div className="space-y-5">
      <PageHeader
        title="备份"
        description="查看数据库备份和邮件发送状态。"
        actions={<Button loading={running} type="button" onClick={runBackup}><Play className="h-4 w-4" />立即备份</Button>}
      />
      {error ? <ErrorBlock message={error} onRetry={load} /> : null}
      {loading ? <LoadingBlock label="加载备份状态" /> : job ? <BackupDetail job={job} /> : <EmptyBlock title="还没有备份记录" />}
      <MessageBar message={message} />
    </div>
  );
}

function BackupDetail({ job }: { job: BackupJob }) {
  return (
    <section className="rounded-lg border border-[var(--border-subtle)] bg-white">
      <div className="flex items-center justify-between gap-3 border-b border-[var(--border-subtle)] px-4 py-3">
        <div className="font-medium">最近一次备份</div>
        <BackupStatusBadge status={job.Status} />
      </div>
      <dl className="grid gap-px bg-[var(--border-subtle)] sm:grid-cols-2">
        <Item label="开始时间" value={formatDateTime(job.StartedAt)} />
        <Item label="结束时间" value={formatDateTime(job.FinishedAt)} />
        <Item label="文件大小" value={formatFileSize(job.FileSize)} />
        <Item label="邮件状态" value={job.EmailStatus || "-"} />
        <Item label="收件人" value={job.Recipient || "-"} />
        <Item label="文件路径" value={job.FilePath || "-"} mono />
        <Item label="错误信息" value={job.ErrorMessage || "-"} wide />
      </dl>
    </section>
  );
}

function Item({ label, value, mono, wide }: { label: string; value: string; mono?: boolean; wide?: boolean }) {
  return (
    <div className={wide ? "bg-white p-4 sm:col-span-2" : "bg-white p-4"}>
      <dt className="text-xs text-[var(--text-secondary)]">{label}</dt>
      <dd className={mono ? "mt-1 break-all font-mono text-xs" : "mt-1 break-words text-sm"}>{value}</dd>
    </div>
  );
}
