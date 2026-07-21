"use client";

import { EditOutlined } from "@ant-design/icons";
import {
  Alert,
  App,
  Button,
  Card,
  Col,
  Descriptions,
  Flex,
  Form,
  Input,
  InputNumber,
  Modal,
  Row,
  Select,
  Table,
  Tag,
  Tooltip,
  type TableProps,
} from "antd";
import { useCallback, useEffect, useMemo, useState } from "react";
import { PageHeader } from "@/components/layout/page-header";
import { useSession } from "@/components/layout/session-context";
import { MovementBadge } from "@/features/labels";
import { initialPagination, tablePagination } from "@/features/pagination";
import { ProductCombobox } from "@/features/product-combobox";
import { ProductIdentity } from "@/features/product-identity";
import type {
  MovementPreview,
  MovementType,
  Paginated,
  Product,
  Shop,
  StockMovement,
} from "@/features/types";
import { ApiError, apiGet, apiPost, request } from "@/lib/api";
import { centsToYuanInput, formatDateTime, formatMoney, formatQuantity, yuanToCents } from "@/lib/format";

type EditValues = {
  quantity?: number;
  quantity_delta?: number;
  unit_yuan?: number;
  shop_id?: string;
  note: string;
  change_reason: string;
};

export default function StockMovementsPage() {
  const { hasPermission } = useSession();
  const canUpdate = hasPermission("movement.update");
  const [movements, setMovements] = useState<StockMovement[]>([]);
  const [products, setProducts] = useState<Product[]>([]);
  const [shops, setShops] = useState<Shop[]>([]);
  const [editing, setEditing] = useState<StockMovement | null>(null);
  const [type, setType] = useState("");
  const [productID, setProductID] = useState("");
  const [shopID, setShopID] = useState("");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [page, setPage] = useState(1);
  const [pagination, setPagination] = useState(initialPagination);

  const params = useMemo(() => {
    const query = new URLSearchParams({ page: String(page) });
    if (type) query.set("type", type);
    if (productID) query.set("product_id", productID);
    if (shopID) query.set("shop_id", shopID);
    return query.toString();
  }, [page, productID, shopID, type]);

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const [movementList, productList, shopList] = await Promise.all([
        apiGet<Paginated<StockMovement>>(`/stock-movements?${params}`),
        apiGet<Paginated<Product>>("/products?include_archived=true&all=true"),
        apiGet<Paginated<Shop>>("/shops?all=true"),
      ]);
      setMovements(movementList.items);
      setPagination(movementList.pagination);
      setProducts(productList.items);
      setShops(shopList.items);
      if (movementList.pagination.page !== page) setPage(movementList.pagination.page);
    } catch (err) {
      setError(err instanceof Error ? err.message : "加载失败");
    } finally {
      setLoading(false);
    }
  }, [page, params]);

  useEffect(() => {
    void load();
  }, [load]);

  const columns: TableProps<StockMovement>["columns"] = [
    { title: "类型", dataIndex: "Type", width: 110, render: (value: MovementType) => <MovementBadge type={value} /> },
    { title: "商品", dataIndex: "Product", width: 280, render: (product: Product) => <ProductIdentity product={product} /> },
    { title: "店铺", dataIndex: ["Shop", "Name"], width: 130, render: (value?: string) => value ?? "-" },
    { title: "数量", dataIndex: "QuantityDelta", width: 100, render: formatQuantity },
    { title: "收入", dataIndex: "RevenueCents", width: 120, render: formatMoney },
    {
      title: "成本",
      width: 120,
      render: (_, movement) => formatMoney(movement.CostAmountCents || movement.PurchaseAmountCents),
    },
    { title: "毛利", dataIndex: "GrossProfitCents", width: 120, render: formatMoney },
    { title: "备注", dataIndex: "Reason", width: 180, ellipsis: true, render: (value: string) => <span className="muted">{value || "-"}</span> },
    { title: "原操作人", dataIndex: ["Operator", "name"], width: 130, render: (value?: string) => value ?? "-" },
    { title: "原时间", dataIndex: "CreatedAt", width: 180, render: (value: string) => <span className="muted">{formatDateTime(value)}</span> },
    {
      title: "修订",
      width: 190,
      render: (_, movement) => movement.Revision > 1 ? (
        <div>
          <Tag color="blue">已修改</Tag>
          <div className="product-cell-note">{movement.LastEditedBy?.name ?? "-"}</div>
          <div className="product-cell-note">{formatDateTime(movement.UpdatedAt)}</div>
        </div>
      ) : <span className="muted">-</span>,
    },
  ];

  if (canUpdate) {
    columns.push({
      title: "操作",
      fixed: "right",
      width: 100,
      render: (_, movement) => (
        <Tooltip title={movement.IsLatest ? "编辑该商品最新流水" : "仅可编辑该商品最新一条流水"}>
          <span>
            <Button
              disabled={!movement.IsLatest}
              icon={<EditOutlined />}
              size="small"
              onClick={() => setEditing(movement)}
            >
              编辑
            </Button>
          </span>
        </Tooltip>
      ),
    });
  }

  return (
    <Flex gap={20} vertical>
      <PageHeader description="查看库存流水；有权限的操作员可修正商品最新一笔流水。" title="流水记录" />
      <Card className="filter-card">
        <Form layout="vertical" requiredMark={false}>
          <Row gutter={[14, 0]}>
            <Col lg={8} sm={12} xs={24}>
              <Form.Item label="类型" style={{ marginBottom: 0 }}>
                <Select
                  allowClear
                  options={[
                    { value: "inbound", label: "入库" },
                    { value: "sales_outbound", label: "销售出库" },
                    { value: "adjustment", label: "调整" },
                  ]}
                  placeholder="全部类型"
                  value={type || undefined}
                  onChange={(value) => { setType(value ?? ""); setPage(1); }}
                />
              </Form.Item>
            </Col>
            <Col lg={8} sm={12} xs={24}>
              <Form.Item label="商品" style={{ marginBottom: 0 }}>
                <ProductCombobox
                  allowClear
                  placeholder="全部商品（按图片选择）"
                  products={products}
                  value={productID}
                  onChange={(value) => { setProductID(value); setPage(1); }}
                />
              </Form.Item>
            </Col>
            <Col lg={8} sm={12} xs={24}>
              <Form.Item label="店铺" style={{ marginBottom: 0 }}>
                <Select
                  allowClear
                  notFoundContent="没有店铺"
                  optionFilterProp="label"
                  options={shops.map((shop) => ({ value: shop.ID, label: shop.Name }))}
                  placeholder="全部店铺"
                  showSearch
                  value={shopID || undefined}
                  onChange={(value) => { setShopID(value ?? ""); setPage(1); }}
                />
              </Form.Item>
            </Col>
          </Row>
        </Form>
      </Card>
      {error ? <Alert action={<Button size="small" onClick={() => void load()}>重试</Button>} message={error} showIcon type="error" /> : null}
      <Card className="table-card">
        <Table<StockMovement>
          columns={columns}
          dataSource={movements}
          loading={loading}
          pagination={tablePagination(pagination, setPage)}
          rowKey="ID"
          scroll={{ x: canUpdate ? 1780 : 1680 }}
        />
      </Card>
      {editing ? (
        <MovementEditor
          key={`${editing.ID}-${editing.Revision}`}
          movement={editing}
          shops={shops}
          onClose={() => setEditing(null)}
          onSaved={() => {
            setEditing(null);
            void load();
          }}
          onRefresh={() => {
            setEditing(null);
            void load();
          }}
        />
      ) : null}
    </Flex>
  );
}

function MovementEditor({
  movement,
  shops,
  onClose,
  onRefresh,
  onSaved,
}: {
  movement: StockMovement;
  shops: Shop[];
  onClose: () => void;
  onRefresh: () => void;
  onSaved: () => void;
}) {
  const { message, modal } = App.useApp();
  const [error, setError] = useState("");
  const [previewing, setPreviewing] = useState(false);
  const [saving, setSaving] = useState(false);
  const [stale, setStale] = useState(false);
  const archived = Boolean(movement.Product.ArchivedAt);
  const unitCents = movement.Type === "inbound" ? movement.PurchaseUnitCents : movement.SaleUnitCents;

  async function preview(values: EditValues) {
    setPreviewing(true);
    setError("");
    setStale(false);
    const payload = movementPayload(movement, values);
    try {
      const result = await apiPost<MovementPreview>(`/stock-movements/${movement.ID}/preview`, payload);
      modal.confirm({
        title: "确认保存流水修订？",
        width: 640,
        content: <MovementImpactPreview movement={movement} preview={result} shops={shops} />,
        okText: "确认修订",
        cancelText: "返回修改",
        async onOk() {
          setSaving(true);
          setError("");
          try {
            await request(`/stock-movements/${movement.ID}`, {
              method: "PATCH",
              headers: { "Content-Type": "application/json" },
              body: JSON.stringify(payload),
            });
            message.success("流水已修订，库存与报表已同步");
            onSaved();
          } catch (err) {
            const text = err instanceof Error ? err.message : "保存失败";
            setError(text);
            setStale(err instanceof ApiError && err.code === "MOVEMENT_STALE");
            message.error(text);
          } finally {
            setSaving(false);
          }
        },
      });
    } catch (err) {
      setError(err instanceof Error ? err.message : "预览失败");
      setStale(err instanceof ApiError && err.code === "MOVEMENT_STALE");
    } finally {
      setPreviewing(false);
    }
  }

  return (
    <Modal destroyOnHidden footer={null} open title="编辑最新流水" width={720} onCancel={onClose}>
      <Flex gap={16} vertical>
        <ProductIdentity product={movement.Product} size={64} />
        <Descriptions
          column={2}
          items={[
            { key: "type", label: "流水类型", children: <MovementBadge type={movement.Type} /> },
            { key: "operator", label: "原操作人", children: movement.Operator?.name ?? "-" },
            { key: "time", label: "原始时间", children: formatDateTime(movement.CreatedAt), span: 2 },
          ]}
          size="small"
        />
        {archived ? <Alert message="商品已归档，仅可修改备注或店铺。" showIcon type="warning" /> : null}
        {error ? (
          <Alert
            action={stale ? <Button size="small" onClick={onRefresh}>刷新流水</Button> : undefined}
            description={stale ? "表单内容仍保留；刷新后请基于最新数据重新修订。" : undefined}
            message={error}
            showIcon
            type="error"
          />
        ) : null}
        <Form<EditValues>
          initialValues={{
            quantity: movement.Type === "adjustment" ? undefined : Math.abs(movement.QuantityDelta),
            quantity_delta: movement.Type === "adjustment" ? movement.QuantityDelta : undefined,
            unit_yuan: unitCents == null ? undefined : Number(centsToYuanInput(unitCents)),
            shop_id: movement.ShopID ?? undefined,
            note: movement.Reason,
            change_reason: "",
          }}
          layout="vertical"
          requiredMark={false}
          onFinish={preview}
        >
          {movement.Type === "adjustment" ? (
            <Form.Item label="调整数量" name="quantity_delta" extra="正数增加，负数减少" rules={[{ required: true, message: "请输入调整数量" }]}>
              <InputNumber disabled={archived} precision={0} style={{ width: "100%" }} />
            </Form.Item>
          ) : (
            <Flex gap={14} wrap>
              <Form.Item label="数量" name="quantity" rules={[{ required: true, message: "请输入数量" }]} style={{ flex: 1, minWidth: 180 }}>
                <InputNumber disabled={archived} min={1} precision={0} style={{ width: "100%" }} />
              </Form.Item>
              <Form.Item
                label={movement.Type === "inbound" ? "进货单价（元）" : "销售单价（元）"}
                name="unit_yuan"
                rules={[{ required: true, message: "请输入单价" }]}
                style={{ flex: 1, minWidth: 180 }}
              >
                <InputNumber disabled={archived} min={0} precision={2} style={{ width: "100%" }} />
              </Form.Item>
            </Flex>
          )}
          {movement.Type !== "adjustment" ? (
            <Form.Item
              label={movement.Type === "inbound" ? "店铺（可选）" : "店铺"}
              name="shop_id"
              rules={movement.Type === "sales_outbound" ? [{ required: true, message: "请选择店铺" }] : undefined}
            >
              <Select
                allowClear={movement.Type === "inbound"}
                notFoundContent="没有店铺"
                optionFilterProp="label"
                options={shops.map((shop) => ({ value: shop.ID, label: shop.Name }))}
                placeholder="选择店铺"
                showSearch
              />
            </Form.Item>
          ) : null}
          <Form.Item
            label="业务备注"
            name="note"
            rules={movement.Type === "adjustment" ? [{ required: true, message: "请输入调整备注" }] : undefined}
          >
            <Input.TextArea maxLength={500} rows={3} showCount />
          </Form.Item>
          <Form.Item label="修改原因" name="change_reason" rules={[{ required: true, message: "请输入修改原因" }]}>
            <Input.TextArea maxLength={500} placeholder="仅进入审计记录，不覆盖业务备注" rows={3} showCount />
          </Form.Item>
          <Flex gap={8} justify="flex-end">
            <Button disabled={previewing || saving} onClick={onClose}>取消</Button>
            <Button htmlType="submit" loading={previewing} disabled={saving} type="primary">预览影响</Button>
          </Flex>
        </Form>
      </Flex>
    </Modal>
  );
}

function MovementImpactPreview({ movement, preview, shops }: { movement: StockMovement; preview: MovementPreview; shops: Shop[] }) {
  const beforeQuantity = movement.Type === "sales_outbound" ? -preview.before.quantity_delta : preview.before.quantity_delta;
  const afterQuantity = movement.Type === "sales_outbound" ? -preview.after.quantity_delta : preview.after.quantity_delta;
  const beforeUnit = movement.Type === "inbound" ? preview.before.purchase_unit_cents : preview.before.sale_unit_cents;
  const afterUnit = movement.Type === "inbound" ? preview.after.purchase_unit_cents : preview.after.sale_unit_cents;
  const items = [
    { key: "quantity", label: "流水数量", children: `${formatQuantity(beforeQuantity)} → ${formatQuantity(afterQuantity)}` },
    ...(movement.Type === "adjustment" ? [] : [{ key: "unit", label: "单价", children: `${formatMoney(beforeUnit ?? 0)} → ${formatMoney(afterUnit ?? 0)}` }]),
    ...(movement.Type === "adjustment" ? [] : [{ key: "shop", label: "店铺", children: `${shopName(shops, preview.before.shop_id)} → ${shopName(shops, preview.after.shop_id)}` }]),
    { key: "note", label: "备注", children: `${preview.before.note || "-"} → ${preview.after.note || "-"}` },
    { key: "stock", label: "当前 / 结果库存", children: `${formatQuantity(preview.impact.current_quantity)} → ${formatQuantity(preview.impact.result_quantity)}` },
    { key: "value", label: "当前 / 结果库存金额", children: `${formatMoney(preview.impact.current_inventory_value_cents)} → ${formatMoney(preview.impact.result_inventory_value_cents)}` },
    { key: "average", label: "当前 / 结果移动平均成本", children: `${formatMoney(preview.impact.current_moving_average_cost_cents)} → ${formatMoney(preview.impact.result_moving_average_cost_cents)}` },
    ...(movement.Type === "inbound" ? [{ key: "purchase", label: "采购金额变化", children: formatMoney(preview.impact.purchase_amount_delta_cents) }] : []),
    ...(movement.Type === "sales_outbound" ? [
      { key: "revenue", label: "收入变化", children: formatMoney(preview.impact.revenue_delta_cents) },
      { key: "cost", label: "成本变化", children: formatMoney(preview.impact.cost_delta_cents) },
      { key: "gross", label: "毛利变化", children: formatMoney(preview.impact.gross_profit_delta_cents) },
    ] : []),
  ];
  return (
    <Flex gap={12} style={{ marginTop: 16 }} vertical>
      <ProductIdentity product={movement.Product} />
      <Descriptions bordered column={1} items={items} size="small" />
      <Alert message="保存时会再次校验最新流水、版本和库存，所有修改与审计在同一事务提交。" showIcon type="info" />
    </Flex>
  );
}

function movementPayload(movement: StockMovement, values: EditValues): Record<string, unknown> {
  const base: Record<string, unknown> = {
    expected_revision: movement.Revision,
    note: values.note ?? "",
    change_reason: values.change_reason,
  };
  if (movement.Type === "adjustment") {
    base.quantity_delta = values.quantity_delta;
    return base;
  }
  base.quantity = values.quantity;
  base.unit_cents = yuanToCents(String(values.unit_yuan ?? 0));
  base.shop_id = values.shop_id ?? null;
  return base;
}

function shopName(shops: Shop[], id: string | null): string {
  if (!id) return "-";
  return shops.find((shop) => shop.ID === id)?.Name ?? id;
}
