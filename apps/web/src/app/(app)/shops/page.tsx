"use client";

import { EditOutlined, PlusOutlined } from "@ant-design/icons";
import { Alert, App, Button, Card, Flex, Form, Input, Modal, Statistic, Table, Tag, type TableProps } from "antd";
import { useCallback, useEffect, useState } from "react";
import { PageHeader } from "@/components/layout/page-header";
import { initialPagination, tablePagination } from "@/features/pagination";
import type { Paginated, Shop } from "@/features/types";
import { useSession } from "@/components/layout/session-context";
import { apiGet, apiPost, apiPut } from "@/lib/api";
import { formatDateTime, formatQuantity } from "@/lib/format";

type ShopValues = { name: string; note?: string };

export default function ShopsPage() {
  const { message } = App.useApp();
  const { hasPermission } = useSession();
  const canCreate = hasPermission("shop.create");
  const canUpdate = hasPermission("shop.update");
  const [shops, setShops] = useState<Shop[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [open, setOpen] = useState(false);
  const [editing, setEditing] = useState<Shop | null>(null);
  const [page, setPage] = useState(1);
  const [pagination, setPagination] = useState(initialPagination);

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const data = await apiGet<Paginated<Shop>>(`/shops?page=${page}`);
      setShops(data.items);
      setPagination(data.pagination);
      if (data.pagination.page !== page) setPage(data.pagination.page);
    } catch (err) {
      setError(err instanceof Error ? err.message : "加载失败");
    } finally {
      setLoading(false);
    }
  }, [page]);

  useEffect(() => {
    void load();
  }, [load]);

  const columns: TableProps<Shop>["columns"] = [
    { title: "名称", dataIndex: "Name", width: 180, render: (value: string) => <strong>{value}</strong> },
    { title: "备注", dataIndex: "Note", ellipsis: true, render: (value: string) => <span className="muted">{value || "-"}</span> },
    { title: "状态", dataIndex: "Enabled", width: 100, render: (value: boolean) => <Tag color={value ? "green" : "red"}>{value ? "启用" : "禁用"}</Tag> },
    { title: "创建时间", dataIndex: "CreatedAt", width: 180, render: (value: string) => <span className="muted">{formatDateTime(value)}</span> },
  ];
  if (canUpdate) {
    columns.push({
      title: "操作",
      fixed: "right",
      width: 100,
      render: (_, shop) => <Button icon={<EditOutlined />} size="small" onClick={() => setEditing(shop)}>修改</Button>,
    });
  }

  return (
    <Flex gap={20} vertical>
      <PageHeader
        actions={canCreate ? <Button icon={<PlusOutlined />} type="primary" onClick={() => setOpen(true)}>新增店铺</Button> : null}
        description="店铺用于销售出库归属，不单独持有库存。"
        title="店铺"
      />
      <Card className="metric-card"><Statistic title="店铺数量" value={formatQuantity(pagination.total)} /></Card>
      {error ? <Alert action={<Button size="small" onClick={() => void load()}>重试</Button>} message={error} showIcon type="error" /> : null}
      <Card className="table-card">
        <Table<Shop>
          columns={columns}
          dataSource={shops}
          loading={loading}
          pagination={tablePagination(pagination, setPage)}
          rowKey="ID"
          scroll={{ x: 800 }}
        />
      </Card>
      <Modal destroyOnHidden footer={null} open={open} title="新增店铺" onCancel={() => setOpen(false)}>
        <ShopForm
          onCancel={() => setOpen(false)}
          onSaved={() => {
            setOpen(false);
            message.success("店铺已创建");
            if (page === 1) void load();
            else setPage(1);
          }}
        />
      </Modal>
      <Modal destroyOnHidden footer={null} open={editing !== null} title="修改店铺" onCancel={() => setEditing(null)}>
        {editing ? (
          <ShopForm
            key={editing.ID}
            shop={editing}
            onCancel={() => setEditing(null)}
            onSaved={() => {
              setEditing(null);
              message.success("店铺已修改");
              void load();
            }}
          />
        ) : null}
      </Modal>
    </Flex>
  );
}

function ShopForm({ shop, onCancel, onSaved }: { shop?: Shop; onCancel: () => void; onSaved: () => void }) {
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);

  async function submit(values: ShopValues) {
    setSaving(true);
    setError("");
    try {
      const payload = { name: values.name, note: values.note ?? "" };
      if (shop) await apiPut<{ item: Shop }>(`/shops/${shop.ID}`, payload);
      else await apiPost<{ item: Shop }>("/shops", payload);
      onSaved();
    } catch (err) {
      setError(err instanceof Error ? err.message : "保存失败");
    } finally {
      setSaving(false);
    }
  }

  return (
    <Form<ShopValues>
      initialValues={{ name: shop?.Name, note: shop?.Note }}
      layout="vertical"
      requiredMark={false}
      onFinish={submit}
    >
      {error ? <Alert message={error} showIcon style={{ marginBottom: 16 }} type="error" /> : null}
      <Form.Item label="店铺名称" name="name" rules={[{ required: true, message: "请输入店铺名称" }]}><Input /></Form.Item>
      <Form.Item label="备注" name="note"><Input.TextArea maxLength={500} rows={3} showCount /></Form.Item>
      <Flex gap={8} justify="flex-end">
        <Button onClick={onCancel}>取消</Button>
        <Button htmlType="submit" loading={saving} type="primary">保存店铺</Button>
      </Flex>
    </Form>
  );
}
