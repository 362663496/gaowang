"use client";

import { PlayCircleOutlined } from "@ant-design/icons";
import { Alert, App, Button, Card, Descriptions, Empty, Flex } from "antd";
import { useCallback, useEffect, useState } from "react";
import { PageHeader } from "@/components/layout/page-header";
import { BackupStatusBadge } from "@/features/labels";
import type { BackupJob } from "@/features/types";
import { apiGet, apiPost } from "@/lib/api";
import { formatDateTime, formatFileSize } from "@/lib/format";

export default function BackupsPage() {
  const { message } = App.useApp();
  const [job, setJob] = useState<BackupJob | null>(null);
  const [loading, setLoading] = useState(true);
  const [running, setRunning] = useState(false);
  const [error, setError] = useState("");

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
      message.success("备份任务已完成");
    } catch (err) {
      setError(err instanceof Error ? err.message : "备份失败");
      await load();
    } finally {
      setRunning(false);
    }
  }

  return (
    <Flex gap={20} vertical>
      <PageHeader
        actions={<Button icon={<PlayCircleOutlined />} loading={running} type="primary" onClick={() => void runBackup()}>立即备份</Button>}
        description="查看数据库备份和邮件发送状态。"
        title="备份"
      />
      {error ? <Alert action={<Button size="small" onClick={() => void load()}>重试</Button>} message={error} showIcon type="error" /> : null}
      <Card loading={loading} title="最近一次备份" extra={job ? <BackupStatusBadge status={job.Status} /> : null}>
        {job ? <BackupDetail job={job} /> : loading ? null : <Empty description="还没有备份记录" image={Empty.PRESENTED_IMAGE_SIMPLE} />}
      </Card>
    </Flex>
  );
}

function BackupDetail({ job }: { job: BackupJob }) {
  return (
    <Descriptions
      bordered
      column={{ xs: 1, sm: 2 }}
      items={[
        { key: "started", label: "开始时间", children: formatDateTime(job.StartedAt) },
        { key: "finished", label: "结束时间", children: formatDateTime(job.FinishedAt) },
        { key: "size", label: "文件大小", children: formatFileSize(job.FileSize) },
        { key: "mail", label: "邮件状态", children: job.EmailStatus || "-" },
        { key: "recipient", label: "收件人", children: job.Recipient || "-" },
        { key: "path", label: "文件路径", children: <span className="mono">{job.FilePath || "-"}</span> },
        { key: "error", label: "错误信息", children: job.ErrorMessage || "-", span: 2 },
      ]}
      size="small"
    />
  );
}
