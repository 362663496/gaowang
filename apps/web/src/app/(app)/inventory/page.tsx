"use client";

import { Alert, App, Button, Card, Col, Flex, Row, Statistic, Table, Tag, type TableProps } from "antd";
import { useCallback, useEffect, useMemo, useState } from "react";
import { PageHeader } from "@/components/layout/page-header";
import { InventoryActions } from "@/features/inventory/action-forms";
import { StockBadge } from "@/features/labels";
import { initialPagination, tablePagination } from "@/features/pagination";
import { ProductImage } from "@/features/product-image";
import type { InventorySnapshot, Paginated, Product, Shop } from "@/features/types";
import { apiGet } from "@/lib/api";
import { formatDateTime, formatMoney, formatQuantity } from "@/lib/format";

export default function InventoryPage() {
  const { message } = App.useApp();
  const [inventory, setInventory] = useState<InventorySnapshot[]>([]);
  const [visibleInventory, setVisibleInventory] = useState<InventorySnapshot[]>([]);
  const [products, setProducts] = useState<Product[]>([]);
  const [shops, setShops] = useState<Shop[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [showLowStock, setShowLowStock] = useState(false);
  const [page, setPage] = useState(1);
  const [pagination, setPagination] = useState(initialPagination);

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const listParams = new URLSearchParams({ page: String(page) });
      if (showLowStock) listParams.set("low_stock", "true");
      const [visibleStock, stock, productList, shopList] = await Promise.all([
        apiGet<Paginated<InventorySnapshot>>(`/inventory?${listParams}`),
        apiGet<Paginated<InventorySnapshot>>("/inventory?all=true"),
        apiGet<Paginated<Product>>("/products?all=true"),
        apiGet<Paginated<Shop>>("/shops?all=true"),
      ]);
      setVisibleInventory(visibleStock.items);
      setPagination(visibleStock.pagination);
      setInventory(stock.items);
      setProducts(productList.items);
      setShops(shopList.items);
      if (visibleStock.pagination.page !== page) setPage(visibleStock.pagination.page);
    } catch (err) {
      setError(err instanceof Error ? err.message : "加载失败");
    } finally {
      setLoading(false);
    }
  }, [page, showLowStock]);

  useEffect(() => {
    void load();
  }, [load]);

  const inventoryValue = useMemo(() => inventory.reduce((sum, item) => sum + item.InventoryValueCents, 0), [inventory]);
  const lowStock = useMemo(
    () => inventory.filter((item) => item.Product.LowStockThreshold > 0 && item.Quantity <= item.Product.LowStockThreshold),
    [inventory],
  );

  const columns: TableProps<InventorySnapshot>["columns"] = [
    {
      title: "商品",
      dataIndex: "Product",
      width: 280,
      render: (_, item) => (
        <Flex align="center" className="product-cell" gap={12}>
          <ProductImage preview product={item.Product} />
          <div className="product-cell-copy">
            <div className="product-cell-name">{item.Product.Name}</div>
            <div className="product-cell-note mono">{item.Product.Code}</div>
          </div>
        </Flex>
      ),
    },
    { title: "数量", dataIndex: "Quantity", width: 110, render: (value: number) => <Tag>{formatQuantity(value)}</Tag> },
    { title: "移动平均成本", dataIndex: "MovingAverageCostCents", width: 150, render: formatMoney },
    { title: "库存金额", dataIndex: "InventoryValueCents", width: 140, render: formatMoney },
    {
      title: "状态",
      width: 100,
      render: (_, item) => <StockBadge quantity={item.Quantity} threshold={item.Product.LowStockThreshold} />,
    },
    { title: "更新时间", dataIndex: "UpdatedAt", width: 180, render: (value: string) => <span className="muted">{formatDateTime(value)}</span> },
  ];

  function done(value: string) {
    message.success(value);
    void load();
  }

  return (
    <Flex gap={20} vertical>
      <PageHeader
        actions={<InventoryActions inventory={inventory} products={products} shops={shops} onDone={done} />}
        description="库存快照由入库、销售出库和调整流水自动更新。"
        title="当前库存"
      />

      <Row gutter={[12, 12]}>
        <Col md={8} xs={24}><Card className="metric-card"><Statistic title="库存品类" value={formatQuantity(inventory.length)} /></Card></Col>
        <Col md={8} xs={24}><Card className="metric-card"><Statistic title="库存金额" value={formatMoney(inventoryValue)} /></Card></Col>
        <Col md={8} xs={24}>
          <Card className="metric-card">
            <Statistic
              title="低库存"
              value={lowStock.length}
              formatter={() => (
                <Button
                  aria-pressed={showLowStock}
                  className="metric-link"
                  danger={lowStock.length > 0}
                  type="link"
                  onClick={() => { setPage(1); setShowLowStock(true); }}
                >{formatQuantity(lowStock.length)}</Button>
              )}
            />
          </Card>
        </Col>
      </Row>

      {showLowStock ? (
        <Alert
          action={<Button size="small" onClick={() => { setPage(1); setShowLowStock(false); }}>显示全部</Button>}
          message="当前仅显示低库存商品"
          showIcon
          type="warning"
        />
      ) : null}
      {error ? <Alert action={<Button size="small" onClick={() => void load()}>重试</Button>} message={error} showIcon type="error" /> : null}

      <Card className="table-card">
        <Table<InventorySnapshot>
          columns={columns}
          dataSource={visibleInventory}
          loading={loading}
          pagination={tablePagination(pagination, setPage)}
          rowKey="ProductID"
          scroll={{ x: 960 }}
        />
      </Card>
    </Flex>
  );
}
