"use client";

import { DeleteOutlined, EditOutlined, PlusOutlined, PoweroffOutlined, SearchOutlined, UploadOutlined } from "@ant-design/icons";
import {
  Alert,
  App,
  Button,
  Card,
  Col,
  Flex,
  Form,
  Input,
  InputNumber,
  Modal,
  Row,
  Space,
  Statistic,
  Table,
  Tag,
  Upload,
  type TableProps,
  type UploadFile,
} from "antd";
import { useCallback, useEffect, useState } from "react";
import { PageHeader } from "@/components/layout/page-header";
import { tablePagination, initialPagination } from "@/features/pagination";
import { ProductImage } from "@/features/product-image";
import type { Paginated, Product } from "@/features/types";
import { useSession } from "@/components/layout/session-context";
import { apiGet, apiPost, request } from "@/lib/api";
import { centsToYuanInput, formatMoney, formatQuantity, yuanToCents } from "@/lib/format";

type ProductAction = { productID: string; type: "status" | "delete" } | null;
type ProductListResponse = Paginated<Product> & {
  summary: { total: number; enabled: number; default_sale_cents: number };
};
type ProductFormValues = {
  name: string;
  code: string;
  purchase_yuan?: number;
  sale_yuan?: number;
  low_stock_threshold?: number;
  note?: string;
};

export default function ProductsPage() {
  const { message, modal } = App.useApp();
  const { hasPermission } = useSession();
  const canCreate = hasPermission("product.create");
  const canUpdate = hasPermission("product.update");
  const canToggle = hasPermission("product.toggle");
  const canDelete = hasPermission("product.delete");
  const [products, setProducts] = useState<Product[]>([]);
  const [query, setQuery] = useState("");
  const [error, setError] = useState("");
  const [actionError, setActionError] = useState("");
  const [loading, setLoading] = useState(true);
  const [creating, setCreating] = useState(false);
  const [editing, setEditing] = useState<Product | null>(null);
  const [busyAction, setBusyAction] = useState<ProductAction>(null);
  const [page, setPage] = useState(1);
  const [pagination, setPagination] = useState(initialPagination);
  const [summary, setSummary] = useState({ total: 0, enabled: 0, default_sale_cents: 0 });

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const params = new URLSearchParams({ page: String(page) });
      if (query.trim()) params.set("q", query.trim());
      const data = await apiGet<ProductListResponse>(`/products?${params}`);
      setProducts(data.items);
      setPagination(data.pagination);
      setSummary(data.summary);
      if (data.pagination.page !== page) setPage(data.pagination.page);
    } catch (err) {
      setError(err instanceof Error ? err.message : "加载失败");
    } finally {
      setLoading(false);
    }
  }, [page, query]);

  useEffect(() => {
    void load();
  }, [load]);

  async function setProductEnabled(product: Product) {
    setBusyAction({ productID: product.ID, type: "status" });
    setActionError("");
    try {
      const data = await request<{ item: Product }>(`/products/${product.ID}/enabled`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ enabled: !product.Enabled }),
      });
      setProducts((current) => current.map((item) => item.ID === product.ID ? data.item : item));
      setSummary((current) => ({ ...current, enabled: current.enabled + (data.item.Enabled ? 1 : -1) }));
      message.success(data.item.Enabled ? "商品已启用" : "商品已禁用");
    } catch (err) {
      setActionError(err instanceof Error ? err.message : "更新商品状态失败");
    } finally {
      setBusyAction(null);
    }
  }

  function confirmDelete(product: Product) {
    modal.confirm({
      title: `删除商品“${product.Name}”？`,
      content: "未使用商品会彻底删除；有历史的零库存商品会归档。此操作不可撤销。",
      okText: "确认删除",
      cancelText: "取消",
      okButtonProps: { danger: true },
      async onOk() {
        setBusyAction({ productID: product.ID, type: "delete" });
        setActionError("");
        try {
          await request<void>(`/products/${product.ID}`, { method: "DELETE" });
          message.success("商品已删除");
          await load();
        } catch (err) {
          setActionError(err instanceof Error ? err.message : "删除商品失败");
        } finally {
          setBusyAction(null);
        }
      },
    });
  }

  const columns: TableProps<Product>["columns"] = [
    {
      title: "商品",
      dataIndex: "Name",
      width: 260,
      render: (_, product) => (
        <Flex align="center" className="product-cell" gap={12}>
          <ProductImage preview product={product} />
          <div className="product-cell-copy">
            <div className="product-cell-name">{product.Name}</div>
            <div className="product-cell-note">{product.Note || "无备注"}</div>
          </div>
        </Flex>
      ),
    },
    { title: "编码", dataIndex: "Code", width: 150, render: (value: string) => <span className="mono">{value}</span> },
    { title: "进货价", dataIndex: "DefaultPurchaseCents", width: 120, render: formatMoney },
    { title: "销售价", dataIndex: "DefaultSaleCents", width: 120, render: formatMoney },
    { title: "低库存", dataIndex: "LowStockThreshold", width: 100, render: formatQuantity },
    {
      title: "状态",
      dataIndex: "Enabled",
      width: 90,
      render: (enabled: boolean) => <Tag color={enabled ? "green" : "red"}>{enabled ? "启用" : "禁用"}</Tag>,
    },
    {
      title: "操作",
      key: "actions",
      fixed: "right",
      width: 260,
      render: (_, product) => (
        <Space size={4}>
          {canUpdate ? <Button disabled={busyAction !== null} icon={<EditOutlined />} size="small" onClick={() => setEditing(product)}>修改</Button> : null}
          {canToggle ? (
            <Button
              disabled={busyAction !== null}
              icon={<PoweroffOutlined />}
              loading={busyAction?.productID === product.ID && busyAction.type === "status"}
              size="small"
              onClick={() => void setProductEnabled(product)}
            >
              {product.Enabled ? "禁用" : "启用"}
            </Button>
          ) : null}
          {canDelete ? (
            <Button
              danger
              disabled={busyAction !== null}
              icon={<DeleteOutlined />}
              loading={busyAction?.productID === product.ID && busyAction.type === "delete"}
              size="small"
              type="text"
              onClick={() => confirmDelete(product)}
            >删除</Button>
          ) : null}
        </Space>
      ),
    },
  ];

  return (
    <Flex gap={20} vertical>
      <PageHeader
        actions={canCreate ? <Button icon={<PlusOutlined />} type="primary" onClick={() => setCreating(true)}>新增商品</Button> : null}
        description="管理商品图片、编码、默认价格和低库存阈值。"
        title="商品"
      />

      <Row gutter={[12, 12]}>
        <Col md={8} xs={24}><Card className="metric-card"><Statistic title="商品数" value={formatQuantity(summary.total)} /></Card></Col>
        <Col md={8} xs={24}><Card className="metric-card"><Statistic title="默认售价合计" value={formatMoney(summary.default_sale_cents)} /></Card></Col>
        <Col md={8} xs={24}><Card className="metric-card"><Statistic title="已启用" value={formatQuantity(summary.enabled)} /></Card></Col>
      </Row>

      <Card className="filter-card">
        <Input
          allowClear
          placeholder="搜索商品名称或编码"
          prefix={<SearchOutlined />}
          value={query}
          onChange={(event) => { setQuery(event.target.value); setPage(1); }}
        />
      </Card>

      {actionError ? <Alert closable message={actionError} showIcon type="error" onClose={() => setActionError("")} /> : null}
      {error ? <Alert action={<Button size="small" onClick={() => void load()}>重试</Button>} message={error} showIcon type="error" /> : null}

      <Card className="table-card">
        <Table<Product>
          columns={columns}
          dataSource={products}
          loading={loading}
          pagination={tablePagination(pagination, setPage)}
          rowKey="ID"
          scroll={{ x: 1100 }}
          size="middle"
        />
      </Card>

      <Modal destroyOnHidden footer={null} open={creating} title="新增商品" width={720} onCancel={() => setCreating(false)}>
        <ProductForm
          onCancel={() => setCreating(false)}
          onSaved={() => {
            setCreating(false);
            message.success("商品已创建");
            if (page === 1) void load();
            else setPage(1);
          }}
        />
      </Modal>

      <Modal destroyOnHidden footer={null} open={editing !== null} title="修改商品" width={720} onCancel={() => setEditing(null)}>
        {editing ? (
          <ProductForm
            key={editing.ID}
            product={editing}
            onCancel={() => setEditing(null)}
            onSaved={() => {
              setEditing(null);
              message.success("商品已修改");
              void load();
            }}
          />
        ) : null}
      </Modal>
    </Flex>
  );
}

function ProductForm({ product, onCancel, onSaved }: {
  product?: Product;
  onCancel: () => void;
  onSaved: (product: Product) => void;
}) {
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);
  const [files, setFiles] = useState<UploadFile[]>([]);

  async function submit(values: ProductFormValues) {
    setSaving(true);
    setError("");
    const form = new FormData();
    form.set("name", values.name);
    form.set("code", values.code);
    form.set("default_purchase_cents", String(yuanToCents(String(values.purchase_yuan ?? 0))));
    form.set("default_sale_cents", String(yuanToCents(String(values.sale_yuan ?? 0))));
    form.set("low_stock_threshold", String(values.low_stock_threshold ?? 0));
    form.set("note", values.note ?? "");
    const image = files[0]?.originFileObj;
    if (image) form.set("image", image);
    try {
      const data = product
        ? await request<{ item: Product }>(`/products/${product.ID}`, { method: "PATCH", body: form })
        : await apiPost<{ item: Product }>("/products", form);
      onSaved(data.item);
    } catch (err) {
      setError(err instanceof Error ? err.message : "保存失败");
    } finally {
      setSaving(false);
    }
  }

  return (
    <Form<ProductFormValues>
      initialValues={{
        name: product?.Name,
        code: product?.Code,
        purchase_yuan: Number(centsToYuanInput(product?.DefaultPurchaseCents ?? 0)),
        sale_yuan: Number(centsToYuanInput(product?.DefaultSaleCents ?? 0)),
        low_stock_threshold: product?.LowStockThreshold ?? 0,
        note: product?.Note,
      }}
      layout="vertical"
      requiredMark={false}
      onFinish={submit}
    >
      {error ? <Alert message={error} showIcon style={{ marginBottom: 16 }} type="error" /> : null}
      <Row gutter={14}>
        <Col sm={12} xs={24}>
          <Form.Item label="商品名称" name="name" rules={[{ required: true, message: "请输入商品名称" }]}><Input /></Form.Item>
        </Col>
        <Col sm={12} xs={24}>
          <Form.Item label="商品编码" name="code" rules={[{ required: true, message: "请输入商品编码" }]}><Input /></Form.Item>
        </Col>
        <Col sm={12} xs={24}>
          <Form.Item label="默认进货价（元）" name="purchase_yuan"><InputNumber min={0} precision={2} style={{ width: "100%" }} /></Form.Item>
        </Col>
        <Col sm={12} xs={24}>
          <Form.Item label="默认销售价（元）" name="sale_yuan"><InputNumber min={0} precision={2} style={{ width: "100%" }} /></Form.Item>
        </Col>
        <Col sm={12} xs={24}>
          <Form.Item label="低库存阈值" name="low_stock_threshold"><InputNumber min={0} precision={0} style={{ width: "100%" }} /></Form.Item>
        </Col>
        <Col sm={12} xs={24}>
          <Form.Item label={product ? "替换商品图片（可选）" : "商品图片（可选）"}>
            <Upload
              accept=".jpg,.jpeg,.png,.webp"
              beforeUpload={() => false}
              fileList={files}
              maxCount={1}
              onChange={({ fileList }) => setFiles(fileList)}
            >
              <Button icon={<UploadOutlined />}>选择图片</Button>
            </Upload>
          </Form.Item>
        </Col>
      </Row>
      <Form.Item label="备注" name="note"><Input.TextArea maxLength={500} rows={3} showCount /></Form.Item>
      <Flex gap={8} justify="flex-end">
        <Button onClick={onCancel}>取消</Button>
        <Button htmlType="submit" loading={saving} type="primary">保存商品</Button>
      </Flex>
    </Form>
  );
}
