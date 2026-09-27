"use client";

import { PlayCircleOutlined } from "@ant-design/icons";
import { Alert, App, Button, Card, Descriptions, Empty, Flex, Form, Input, Switch } from "antd";
import { useCallback, useEffect, useState } from "react";
import { PageHeader } from "@/components/layout/page-header";
import { BackupStatusBadge } from "@/features/labels";
import type { BackupJob, BackupSchedule } from "@/features/types";
import { useSession } from "@/components/layout/session-context";
import { apiGet, apiPost, apiPut } from "@/lib/api";
import { formatDateTime, formatFileSize } from "@/lib/format";

const backupTimePattern = /^(?:[01]\d|2[0-3]):[0-5]\d$/;

export default function BackupsPage() {
  const { message } = App.useApp();
  const { hasPermission } = useSession();
  const canRun = hasPermission("backup.run");
  const [job, setJob] = useState<BackupJob | null>(null);
  const [schedule, setSchedule] = useState<BackupSchedule | null>(null);
  const [enabled, setEnabled] = useState(false);
  const [time, setTime] = useState("02:00");
  const [loading, setLoading] = useState(true);
  const [running, setRunning] = useState(false);
  const [savingSchedule, setSavingSchedule] = useState(false);
  const [error, setError] = useState("");

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const [latest, current] = await Promise.all([
        apiGet<{ job: BackupJob | null }>("/backups/latest"),
        apiGet<{ schedule: BackupSchedule }>("/backups/schedule"),
      ]);
      setJob(latest.job);
      setSchedule(current.schedule);
      setEnabled(current.schedule.enabled);
      setTime(current.schedule.time);
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

  async function saveSchedule() {
    if (!backupTimePattern.test(time.trim())) {
      setError("备份时间格式应为 HH:MM");
      return;
    }
    setSavingSchedule(true);
    setError("");
    try {
      const saved = await apiPut<{ schedule: BackupSchedule }>("/backups/schedule", { enabled, time: time.trim() });
      setSchedule(saved.schedule);
      setEnabled(saved.schedule.enabled);
      setTime(saved.schedule.time);
      message.success("定时备份已保存");
    } catch (err) {
      setError(err instanceof Error ? err.message : "保存定时备份失败");
    } finally {
      setSavingSchedule(false);
    }
  }

  return (
    <Flex gap={20} vertical>
      <PageHeader
        actions={canRun ? <Button icon={<PlayCircleOutlined />} loading={running} type="primary" onClick={() => void runBackup()}>立即备份</Button> : null}
        description="查看最近一次备份，并设置每天自动备份。"
        title="备份"
      />
      {error ? <Alert action={<Button size="small" onClick={() => void load()}>重试</Button>} message={error} showIcon type="error" /> : null}
      <Card loading={loading} title="定时备份">
        <Flex gap={16} vertical>
          <Flex align="center" gap={12}>
            <Switch aria-label="启用定时备份" checked={enabled} disabled={!canRun || loading} onChange={setEnabled} />
            <span>{enabled ? "已启用" : "未启用"}</span>
          </Flex>
          <Form layout="vertical" requiredMark={false}>
            <Flex align="flex-end" gap={12} wrap="wrap">
              <Form.Item label="每天备份时间" style={{ marginBottom: 0 }}>
                <Input
                  disabled={!canRun || loading}
                  maxLength={5}
                  placeholder="02:00"
                  style={{ width: 140 }}
                  value={time}
                  onChange={(event) => setTime(event.target.value)}
                />
              </Form.Item>
              {canRun ? <Button loading={savingSchedule} type="primary" onClick={() => void saveSchedule()}>保存定时设置</Button> : null}
            </Flex>
          </Form>
          <span className="muted">每天北京时间执行一次。服务若在设定时间之后启动，当天会补跑一次。</span>
          {schedule?.due ? <Alert message="已到备份时间，将在一分钟内执行" showIcon type="info" /> : null}
          {schedule?.enabled && !schedule.due && schedule.next_run_at ? <span>下次备份：{formatShanghaiDateTime(schedule.next_run_at)}</span> : null}
        </Flex>
      </Card>
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
        { key: "trigger", label: "触发方式", children: job.Trigger === "scheduled" ? "定时" : "手动" },
        { key: "started", label: "开始时间", children: formatDateTime(job.StartedAt) },
        { key: "finished", label: "结束时间", children: formatDateTime(job.FinishedAt) },
        { key: "size", label: "文件大小", children: formatFileSize(job.FileSize) },
        { key: "mail", label: "邮件状态", children: job.EmailStatus || "-" },
        { key: "recipient", label: "收件人", children: job.Recipient || "-" },
        { key: "path", label: "文件路径", children: <span className="mono">{job.FilePath || "-"}</span>, span: 2 },
        { key: "error", label: "错误信息", children: job.ErrorMessage || "-", span: 2 },
      ]}
      size="small"
    />
  );
}

function formatShanghaiDateTime(value: string): string {
  return new Intl.DateTimeFormat("zh-CN", {
    dateStyle: "medium",
    timeStyle: "short",
    hour12: false,
    timeZone: "Asia/Shanghai",
  }).format(new Date(value));
}
