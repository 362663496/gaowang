"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { Search, X } from "lucide-react";
import { PageHeader } from "@/components/layout/page-header";
import { Button } from "@/components/ui/button";
import { Field, Input, Select } from "@/components/ui/fields";
import { EmptyBlock, ErrorBlock, LoadingBlock } from "@/components/ui/state";
import { AuditActionBadge, auditActionLabel, auditResourceLabel } from "@/features/labels";
import type { AuditLog, User } from "@/features/types";
import { apiGet } from "@/lib/api";
import { formatDateTime } from "@/lib/format";

const actionOptions = [
  "auth.login_succeeded",
  "auth.login_failed",
  "auth.password_changed",
  "product.create",
  "shop.create",
  "inventory.inbound",
  "inventory.sales_outbound",
  "inventory.adjustment",
  "user.create",
  "backup.run_succeeded",
  "backup.run_failed",
  "settings.update",
] as const;

const resourceOptions = ["auth", "backup", "product", "shop", "setting", "user"] as const;

export default function AuditLogsPage() {
  const [logs, setLogs] = useState<AuditLog[]>([]);
  const [users, setUsers] = useState<User[]>([]);
  const [actorID, setActorID] = useState("");
  const [action, setAction] = useState("");
  const [resourceType, setResourceType] = useState("");
  const [resourceID, setResourceID] = useState("");
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  const params = useMemo(() => {
    const query = new URLSearchParams();
    if (actorID) query.set("actor_id", actorID);
    if (action) query.set("action", action);
    if (resourceType) query.set("resource_type", resourceType);
    if (resourceID.trim()) query.set("resource_id", resourceID.trim());
    if (from) query.set("from", from);
    if (to) query.set("to", to);
    return query.toString();
  }, [action, actorID, from, resourceID, resourceType, to]);

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const [auditList, userList] = await Promise.all([
        apiGet<{ items: AuditLog[] }>(`/audit-logs${params ? `?${params}` : ""}`),
        apiGet<{ items: User[] }>("/users"),
      ]);
      setLogs(auditList.items);
      setUsers(userList.items);
    } catch (err) {
      setError(err instanceof Error ? err.message : "加载失败");
    } finally {
      setLoading(false);
    }
  }, [params]);

  useEffect(() => {
    void load();
  }, [load]);

  function resetFilters() {
    setActorID("");
    setAction("");
    setResourceType("");
    setResourceID("");
    setFrom("");
    setTo("");
  }

  return (
    <div className="space-y-5">
      <PageHeader title="操作记录" description="按人员、动作、对象和时间查询后台操作审计。" />
      <div className="grid gap-3 rounded-lg border border-[var(--border-subtle)] bg-white p-3 md:grid-cols-3 xl:grid-cols-6">
        <Field label="人员">
          <Select value={actorID} onChange={(event) => setActorID(event.target.value)}>
            <option value="">全部</option>
            {users.map((user) => <option key={user.id} value={user.id}>{user.name || user.email}</option>)}
          </Select>
        </Field>
        <Field label="动作">
          <Select value={action} onChange={(event) => setAction(event.target.value)}>
            <option value="">全部</option>
            {actionOptions.map((item) => <option key={item} value={item}>{auditActionLabel(item)}</option>)}
          </Select>
        </Field>
        <Field label="对象">
          <Select value={resourceType} onChange={(event) => setResourceType(event.target.value)}>
            <option value="">全部</option>
            {resourceOptions.map((item) => <option key={item} value={item}>{auditResourceLabel(item)}</option>)}
          </Select>
        </Field>
        <Field label="对象 ID">
          <Input value={resourceID} onChange={(event) => setResourceID(event.target.value)} placeholder="精确匹配" />
        </Field>
        <Field label="开始日期">
          <Input type="date" value={from} onChange={(event) => setFrom(event.target.value)} />
        </Field>
        <Field label="结束日期">
          <Input type="date" value={to} onChange={(event) => setTo(event.target.value)} />
        </Field>
        <div className="flex items-end gap-2 md:col-span-3 xl:col-span-6">
          <Button type="button" variant="secondary" onClick={load}><Search className="h-4 w-4" />查询</Button>
          <Button type="button" variant="ghost" onClick={resetFilters}><X className="h-4 w-4" />清空</Button>
        </div>
      </div>
      {loading ? <LoadingBlock label="加载操作记录" /> : error ? <ErrorBlock message={error} onRetry={load} /> : <AuditTable logs={logs} />}
    </div>
  );
}

function AuditTable({ logs }: { logs: AuditLog[] }) {
  if (logs.length === 0) {
    return <EmptyBlock title="没有符合条件的操作记录" />;
  }
  return (
    <div className="overflow-x-auto rounded-lg border border-[var(--border-subtle)] bg-white">
      <table className="w-full min-w-[1120px] text-left text-sm">
        <thead className="border-b border-[var(--border-subtle)] text-xs text-[var(--text-secondary)]">
          <tr>
            <th className="px-4 py-3 font-medium">时间</th>
            <th className="px-4 py-3 font-medium">人员</th>
            <th className="px-4 py-3 font-medium">动作</th>
            <th className="px-4 py-3 font-medium">对象</th>
            <th className="px-4 py-3 font-medium">对象 ID</th>
            <th className="px-4 py-3 font-medium">IP</th>
            <th className="px-4 py-3 font-medium">附加信息</th>
          </tr>
        </thead>
        <tbody>
          {logs.map((log) => (
            <tr className="border-b border-[var(--border-subtle)] last:border-0 hover:bg-black/[0.02]" key={log.id}>
              <td className="whitespace-nowrap px-4 py-3 text-[var(--text-secondary)]">{formatDateTime(log.created_at)}</td>
              <td className="px-4 py-3">
                <div className="font-medium">{log.actor?.name ?? "系统"}</div>
                <div className="text-xs text-[var(--text-secondary)]">{log.actor?.email ?? "-"}</div>
              </td>
              <td className="px-4 py-3"><AuditActionBadge action={log.action} /></td>
              <td className="px-4 py-3">{auditResourceLabel(log.resource_type)}</td>
              <td className="max-w-[180px] truncate px-4 py-3 font-mono text-xs">{log.resource_id || "-"}</td>
              <td className="px-4 py-3 font-mono text-xs text-[var(--text-secondary)]">{log.ip_address || "-"}</td>
              <td className="max-w-[260px] truncate px-4 py-3 text-xs text-[var(--text-secondary)]">{metadataText(log.metadata)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function metadataText(metadata: Record<string, string>): string {
  const pairs = Object.entries(metadata);
  if (pairs.length === 0) {
    return "-";
  }
  return pairs.map(([key, value]) => `${key}: ${value}`).join(" · ");
}
