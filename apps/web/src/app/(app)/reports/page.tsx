"use client";

import { Card, Col, Flex, List, Progress, Result, Row, Statistic, Table, Tag, type TableProps } from "antd";
import { useCallback, useEffect, useMemo, useState } from "react";
import { PageEmpty, PageError, PageLoading } from "@/components/layout/page-feedback";
import { PageHeader } from "@/components/layout/page-header";
import { useSession } from "@/components/layout/session-context";
import { ProductIdentity } from "@/features/product-identity";
import type { InventorySnapshot, ProductRankingRow, SalesSummary, SalesTrendRow, ShopRankingRow } from "@/features/types";
import { apiGet } from "@/lib/api";
import { formatMoney, formatQuantity } from "@/lib/format";

export default function ReportsPage() {
  const { hasPermission } = useSession();
  const canSummary = hasPermission("report.sales_summary");
  const canTrend = hasPermission("report.sales_trend");
  const canProductRank = hasPermission("report.product_ranking");
  const canShopRank = hasPermission("report.shop_ranking");
  const canInventory = hasPermission("inventory.read");
  const [summary, setSummary] = useState<SalesSummary | null>(null);
  const [trend, setTrend] = useState<SalesTrendRow[]>([]);
  const [products, setProducts] = useState<ProductRankingRow[]>([]);
  const [shops, setShops] = useState<ShopRankingRow[]>([]);
  const [inventory, setInventory] = useState<InventorySnapshot[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const [report, salesTrend, productRanking, shopRanking, stock] = await Promise.all([
        canSummary ? apiGet<{ summary: SalesSummary }>("/reports/sales-summary") : Promise.resolve({ summary: null as SalesSummary | null }),
        canTrend ? apiGet<{ items: SalesTrendRow[] }>("/reports/sales-trend") : Promise.resolve({ items: [] as SalesTrendRow[] }),
        canProductRank ? apiGet<{ items: ProductRankingRow[] }>("/reports/product-ranking") : Promise.resolve({ items: [] as ProductRankingRow[] }),
        canShopRank ? apiGet<{ items: ShopRankingRow[] }>("/reports/shop-ranking") : Promise.resolve({ items: [] as ShopRankingRow[] }),
        canInventory ? apiGet<{ items: InventorySnapshot[] }>("/inventory?all=true") : Promise.resolve({ items: [] as InventorySnapshot[] }),
      ]);
      setSummary(report.summary);
      setTrend(salesTrend.items);
      setProducts(productRanking.items);
      setShops(shopRanking.items);
      setInventory(stock.items);
    } catch (err) {
      setError(err instanceof Error ? err.message : "加载失败");
    } finally {
      setLoading(false);
    }
  }, [canInventory, canProductRank, canShopRank, canSummary, canTrend]);

  useEffect(() => {
    void load();
  }, [load]);

  const inventoryValue = useMemo(() => inventory.reduce((sum, item) => sum + item.InventoryValueCents, 0), [inventory]);
  const lowStock = useMemo(
    () => inventory.filter((item) => item.Product.LowStockThreshold > 0 && item.Quantity <= item.Product.LowStockThreshold),
    [inventory],
  );

  if (loading) return <PageLoading label="加载报表" />;
  if (error) return <PageError message={error} onRetry={() => void load()} />;
  if (!canSummary && !canTrend && !canProductRank && !canShopRank && !canInventory) {
    return <Result status="403" subTitle="当前账号没有可查看的报表权限。" title="无权限" />;
  }

  const lowStockColumns: TableProps<InventorySnapshot>["columns"] = [
    { title: "商品", dataIndex: "Product", width: 280, render: (_, item) => <ProductIdentity product={item.Product} /> },
    { title: "当前库存", dataIndex: "Quantity", render: formatQuantity },
    { title: "阈值", dataIndex: ["Product", "LowStockThreshold"], render: formatQuantity },
  ];

  return (
    <Flex gap={20} vertical>
      <PageHeader description="销售额、成本、毛利、库存金额和低库存概览。" title="报表" />
      {canSummary && summary ? (
        <Row gutter={[12, 12]}>
          <Col lg={5} sm={12} xs={24}><Metric label="销售额" value={formatMoney(summary.revenue_cents)} /></Col>
          <Col lg={5} sm={12} xs={24}><Metric label="销售成本" value={formatMoney(summary.cost_cents)} /></Col>
          <Col lg={5} sm={12} xs={24}><Metric label="毛利" value={formatMoney(summary.gross_profit_cents)} /></Col>
          {canInventory ? <Col lg={5} sm={12} xs={24}><Metric label="库存金额" value={formatMoney(inventoryValue)} /></Col> : null}
          {canInventory ? <Col lg={4} sm={12} xs={24}><Metric label="低库存品类" value={formatQuantity(lowStock.length)} /></Col> : null}
        </Row>
      ) : null}
      <Row gutter={[16, 16]}>
        {canTrend ? (
          <Col xl={14} xs={24}><TrendPanel rows={trend} /></Col>
        ) : null}
        {canProductRank ? (
          <Col xl={canTrend ? 10 : 24} xs={24}>
            <RankingPanel
              rows={products.map((item) => ({
                id: item.product_id,
                label: item.product_name,
                sublabel: item.product_code,
                imagePath: item.product_image_path,
                archived: item.archived,
                revenue: item.revenue_cents,
                quantity: item.quantity_sold,
                gross: item.gross_profit_cents,
              }))}
              title="商品销售排行"
            />
          </Col>
        ) : null}
      </Row>
      {canShopRank ? (
        <RankingPanel
          rows={shops.map((item) => ({
            id: item.shop_id,
            label: item.shop_name,
            revenue: item.revenue_cents,
            quantity: item.quantity_sold,
            gross: item.gross_profit_cents,
          }))}
          title="店铺销售排行"
        />
      ) : null}
      {canInventory ? (
        <Card className="table-card" title="低库存商品">
          <Table<InventorySnapshot>
            columns={lowStockColumns}
            dataSource={lowStock}
            locale={{ emptyText: <PageEmpty title="暂无低库存" /> }}
            pagination={false}
            rowKey="ProductID"
            scroll={{ x: 700 }}
          />
        </Card>
      ) : null}
    </Flex>
  );
}

function Metric({ label, value }: { label: string; value: string }) {
  return (
    <Card className="metric-card">
      <Statistic title={label} value={value} />
    </Card>
  );
}

function TrendPanel({ rows }: { rows: SalesTrendRow[] }) {
  const max = Math.max(...rows.map((row) => row.revenue_cents), 1);
  return (
    <Card title="销售趋势">
      {rows.length === 0 ? (
        <PageEmpty title="暂无趋势数据" />
      ) : (
        <List
          dataSource={rows}
          renderItem={(row) => (
            <List.Item>
              <List.Item.Meta description={formatMoney(row.revenue_cents)} title={row.day} />
              <Progress percent={Math.round((row.revenue_cents / max) * 100)} showInfo={false} style={{ width: 160 }} />
            </List.Item>
          )}
        />
      )}
    </Card>
  );
}

function RankingPanel({
  rows,
  title,
}: {
  title: string;
  rows: Array<{ id: string; label: string; sublabel?: string; imagePath?: string; archived?: boolean; revenue: number; quantity: number; gross: number }>;
}) {
  return (
    <Card title={title}>
      {rows.length === 0 ? (
        <PageEmpty title="暂无排行" />
      ) : (
        <List
          dataSource={rows}
          renderItem={(row, index) => (
            <List.Item>
              <List.Item.Meta
                avatar={<Tag color="blue">#{index + 1}</Tag>}
                description={
                  <span>
                    销量 {formatQuantity(row.quantity)} · 毛利 {formatMoney(row.gross)}
                  </span>
                }
                title={row.sublabel !== undefined ? (
                  <ProductIdentity
                    product={{
                      Name: row.label,
                      Code: row.sublabel,
                      ImagePath: row.imagePath ?? "",
                      ArchivedAt: row.archived ? "archived" : null,
                    }}
                  />
                ) : row.label}
              />
              <strong>{formatMoney(row.revenue)}</strong>
            </List.Item>
          )}
        />
      )}
    </Card>
  );
}
