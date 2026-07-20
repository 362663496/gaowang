"use client";

import { ClearOutlined, SearchOutlined } from "@ant-design/icons";
import { Alert, Button, Card, Col, Flex, Form, Input, Row, Select, Space, Table, Tooltip, type TableProps } from "antd";
import { useCallback, useEffect, useMemo, useState } from "react";
import { PageHeader } from "@/components/layout/page-header";
import { AuditActionBadge, auditActionLabel, auditResourceLabel } from "@/features/labels";
import { initialPagination, tablePagination } from "@/features/pagination";
import type { AuditLog, Paginated, User } from "@/features/types";
import { apiGet } from "@/lib/api";
import { formatDateTime } from "@/lib/format";

const actionOptions = [
  "auth.login_succeeded",
  "auth.login_failed",
  "auth.password_changed",
  "product.create",
  "product.update",
  "product.enable",
  "product.disable",
  "product.delete",
  "product.archive",
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
  const [page, setPage] = useState(1);
  const [pagination, setPagination] = useState(initialPagination);

  const params = useMemo(() => {
    const query = new URLSearchParams({ page: String(page) });
    if (actorID) query.set("actor_id", actorID);
    if (action) query.set("action", action);
    if (resourceType) query.set("resource_type", resourceType);
    if (resourceID.trim()) query.set("resource_id", resourceID.trim());
    if (from) query.set("from", from);
    if (to) query.set("to", to);
    return query.toString();
  }, [action, actorID, from, page, resourceID, resourceType, to]);

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const [auditList, userList] = await Promise.all([
        apiGet<Paginated<AuditLog>>(`/audit-logs?${params}`),
        apiGet<Paginated<User>>("/users?all=true"),
      ]);
      setLogs(auditList.items);
      setPagination(auditList.pagination);
      setUsers(userList.items);
      if (auditList.pagination.page !== page) setPage(auditList.pagination.page);
    } catch (err) {
      setError(err instanceof Error ? err.message : "加载失败");
    } finally {
      setLoading(false);
    }
  }, [page, params]);

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
    setPage(1);
  }

  const columns: TableProps<AuditLog>["columns"] = [
    { title: "时间", dataIndex: "created_at", width: 180, render: (value: string) => <span className="muted">{formatDateTime(value)}</span> },
    {
      title: "人员",
      dataIndex: "actor",
      width: 190,
      render: (actor: User | null) => (
        <div>
          <div className="product-cell-name">{actor?.name ?? "系统"}</div>
          <div className="product-cell-note">{actor?.email ?? "-"}</div>
        </div>
      ),
    },
    { title: "动作", dataIndex: "action", width: 150, render: (value: string) => <AuditActionBadge action={value} /> },
    { title: "对象", dataIndex: "resource_type", width: 100, render: auditResourceLabel },
    { title: "对象 ID", dataIndex: "resource_id", width: 180, ellipsis: true, render: (value: string) => <span className="mono">{value || "-"}</span> },
    { title: "IP", dataIndex: "ip_address", width: 140, render: (value: string) => <span className="mono muted">{value || "-"}</span> },
    {
      title: "附加信息",
      dataIndex: "metadata",
      width: 260,
      ellipsis: true,
      render: (metadata: Record<string, string>) => {
        const text = metadataText(metadata);
        return <Tooltip title={text}><span className="muted">{text}</span></Tooltip>;
      },
    },
  ];

  return (
    <Flex gap={20} vertical>
      <PageHeader description="按人员、动作、对象和时间查询后台操作审计。" title="操作记录" />
      <Card className="filter-card">
        <Form layout="vertical" requiredMark={false}>
          <Row gutter={[14, 0]}>
            <Col lg={4} md={8} sm={12} xs={24}>
              <Form.Item label="人员">
                <Select
                  allowClear
                  optionFilterProp="label"
                  options={users.map((user) => ({ value: user.id, label: user.name || user.email }))}
                  placeholder="全部人员"
                  showSearch
                  value={actorID || undefined}
                  onChange={(value) => { setActorID(value ?? ""); setPage(1); }}
                />
              </Form.Item>
            </Col>
            <Col lg={4} md={8} sm={12} xs={24}>
              <Form.Item label="动作">
                <Select
                  allowClear
                  optionFilterProp="label"
                  options={actionOptions.map((item) => ({ value: item, label: auditActionLabel(item) }))}
                  placeholder="全部动作"
                  showSearch
                  value={action || undefined}
                  onChange={(value) => { setAction(value ?? ""); setPage(1); }}
                />
              </Form.Item>
            </Col>
            <Col lg={4} md={8} sm={12} xs={24}>
              <Form.Item label="对象">
                <Select
                  allowClear
                  optionFilterProp="label"
                  options={resourceOptions.map((item) => ({ value: item, label: auditResourceLabel(item) }))}
                  placeholder="全部对象"
                  showSearch
                  value={resourceType || undefined}
                  onChange={(value) => { setResourceType(value ?? ""); setPage(1); }}
                />
              </Form.Item>
            </Col>
            <Col lg={4} md={8} sm={12} xs={24}>
              <Form.Item label="对象 ID"><Input placeholder="精确匹配" value={resourceID} onChange={(event) => { setResourceID(event.target.value); setPage(1); }} /></Form.Item>
            </Col>
            <Col lg={4} md={8} sm={12} xs={24}>
              <Form.Item label="开始日期"><Input type="date" value={from} onChange={(event) => { setFrom(event.target.value); setPage(1); }} /></Form.Item>
            </Col>
            <Col lg={4} md={8} sm={12} xs={24}>
              <Form.Item label="结束日期"><Input type="date" value={to} onChange={(event) => { setTo(event.target.value); setPage(1); }} /></Form.Item>
            </Col>
          </Row>
          <Space>
            <Button icon={<SearchOutlined />} onClick={() => void load()}>查询</Button>
            <Button icon={<ClearOutlined />} type="text" onClick={resetFilters}>清空</Button>
          </Space>
        </Form>
      </Card>
      {error ? <Alert action={<Button size="small" onClick={() => void load()}>重试</Button>} message={error} showIcon type="error" /> : null}
      <Card className="table-card">
        <Table<AuditLog>
          columns={columns}
          dataSource={logs}
          loading={loading}
          pagination={tablePagination(pagination, setPage)}
          rowKey="id"
          scroll={{ x: 1220 }}
        />
      </Card>
    </Flex>
  );
}

function metadataText(metadata: Record<string, string>): string {
  const pairs = Object.entries(metadata);
  return pairs.length === 0 ? "-" : pairs.map(([key, value]) => `${key}: ${value}`).join(" · ");
}
