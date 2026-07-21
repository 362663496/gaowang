"use client";

import { DownloadOutlined, SearchOutlined } from "@ant-design/icons";
import { Alert, App, Button, Card, Col, Flex, Input, Row, Statistic, Table, Tag, type TableProps } from "antd";
import { useCallback, useEffect, useMemo, useState } from "react";
import { PageHeader } from "@/components/layout/page-header";
import { useSession } from "@/components/layout/session-context";
import { InventoryActions } from "@/features/inventory/action-forms";
import { StockBadge } from "@/features/labels";
import { initialPagination, tablePagination } from "@/features/pagination";
import { ProductIdentity } from "@/features/product-identity";
import type { InventorySnapshot, Paginated, Product, Shop } from "@/features/types";
import { apiDownload, apiGet } from "@/lib/api";
import { formatDateTime, formatMoney, formatQuantity } from "@/lib/format";

export default function InventoryPage() {
  const { message } = App.useApp();
  const { hasPermission } = useSession();
  const canInbound = hasPermission("inventory.inbound");
  const canOutbound = hasPermission("inventory.sales_outbound");
  const canAdjust = hasPermission("inventory.adjust");
  const canLoadProducts = hasPermission("product.read");
  const canLoadShops = hasPermission("shop.read");
  const [inventory, setInventory] = useState<InventorySnapshot[]>([]);
  const [visibleInventory, setVisibleInventory] = useState<InventorySnapshot[]>([]);
  const [products, setProducts] = useState<Product[]>([]);
  const [shops, setShops] = useState<Shop[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [exporting, setExporting] = useState(false);
  const [query, setQuery] = useState("");
  const [showLowStock, setShowLowStock] = useState(false);
  const [page, setPage] = useState(1);
  const [pagination, setPagination] = useState(initialPagination);

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const listParams = new URLSearchParams({ page: String(page) });
      if (showLowStock) listParams.set("low_stock", "true");
      if (query.trim()) listParams.set("q", query.trim());
      const [visibleStock, stock, productList, shopList] = await Promise.all([
        apiGet<Paginated<InventorySnapshot>>(`/inventory?${listParams}`),
        apiGet<Paginated<InventorySnapshot>>("/inventory?all=true"),
        canLoadProducts ? apiGet<Paginated<Product>>("/products?all=true") : Promise.resolve({ items: [] as Product[] }),
        canLoadShops ? apiGet<Paginated<Shop>>("/shops?all=true") : Promise.resolve({ items: [] as Shop[] }),
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
  }, [canLoadProducts, canLoadShops, page, query, showLowStock]);

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
      render: (_, item) => <ProductIdentity preview product={item.Product} />,
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

  async function exportExcel() {
    setExporting(true);
    try {
      const params = new URLSearchParams();
      if (showLowStock) params.set("low_stock", "true");
      if (query.trim()) params.set("q", query.trim());
      const exportQuery = params.toString();
      const today = new Date().toISOString().slice(0, 10);
      await apiDownload(`/inventory/export${exportQuery ? `?${exportQuery}` : ""}`, `inventory-${today}.xlsx`);
      message.success("导出成功");
    } catch (err) {
      message.error(err instanceof Error ? err.message : "导出失败");
    } finally {
      setExporting(false);
    }
  }

  return (
    <Flex gap={20} vertical>
      <PageHeader
        actions={
          <>
            <Input
              allowClear
              aria-label="筛选商品"
              placeholder="搜索商品名称或编码"
              prefix={<SearchOutlined />}
              style={{ width: 220 }}
              value={query}
              onChange={(event) => {
                setQuery(event.target.value);
                setPage(1);
              }}
            />
            <InventoryActions
              canAdjust={canAdjust}
              canInbound={canInbound}
              canOutbound={canOutbound}
              inventory={inventory}
              products={products}
              shops={shops}
              onDone={done}
            />
            <Button icon={<DownloadOutlined />} loading={exporting} onClick={() => void exportExcel()}>
              导出 Excel
            </Button>
          </>
        }
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
