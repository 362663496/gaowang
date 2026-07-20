"use client";

import { Alert, Button, Card, Col, Flex, Form, Row, Select, Table, Tag, type TableProps } from "antd";
import { useCallback, useEffect, useMemo, useState } from "react";
import { PageHeader } from "@/components/layout/page-header";
import { MovementBadge } from "@/features/labels";
import { initialPagination, tablePagination } from "@/features/pagination";
import { ProductCombobox } from "@/features/product-combobox";
import { ProductImage } from "@/features/product-image";
import type { MovementType, Paginated, Product, Shop, StockMovement } from "@/features/types";
import { apiGet } from "@/lib/api";
import { formatDateTime, formatMoney, formatQuantity } from "@/lib/format";

export default function StockMovementsPage() {
  const [movements, setMovements] = useState<StockMovement[]>([]);
  const [products, setProducts] = useState<Product[]>([]);
  const [shops, setShops] = useState<Shop[]>([]);
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
    {
      title: "商品",
      dataIndex: "Product",
      width: 280,
      render: (product: Product | null) => <MovementProduct product={product} />,
    },
    { title: "店铺", dataIndex: ["Shop", "Name"], width: 130, render: (value?: string) => value ?? "-" },
    { title: "数量", dataIndex: "QuantityDelta", width: 100, render: formatQuantity },
    { title: "收入", dataIndex: "RevenueCents", width: 120, render: formatMoney },
    {
      title: "成本",
      width: 120,
      render: (_, movement) => formatMoney(movement.CostAmountCents || movement.PurchaseAmountCents),
    },
    { title: "毛利", dataIndex: "GrossProfitCents", width: 120, render: formatMoney },
    { title: "备注", dataIndex: "Reason", width: 200, ellipsis: true, render: (value: string) => <span className="muted">{value || "-"}</span> },
    { title: "时间", dataIndex: "CreatedAt", width: 180, render: (value: string) => <span className="muted">{formatDateTime(value)}</span> },
  ];

  return (
    <Flex gap={20} vertical>
      <PageHeader description="按类型、商品和店铺查看不可变库存流水。" title="流水记录" />
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
                  placeholder="全部商品（输入名称或编码）"
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
          scroll={{ x: 1380 }}
        />
      </Card>
    </Flex>
  );
}

function MovementProduct({ product }: { product: Product | null }) {
  if (!product) return <span>-</span>;
  return (
    <Flex align="center" className="product-cell" gap={12}>
      <ProductImage product={product} />
      <div className="product-cell-copy">
        <Flex align="center" gap={6}>
          <span className="product-cell-name">{product.Name}</span>
          {product.ArchivedAt ? <Tag>已归档</Tag> : null}
        </Flex>
        <div className="product-cell-note mono">{product.Code}</div>
      </div>
    </Flex>
  );
}
