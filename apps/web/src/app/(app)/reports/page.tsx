"use client";

import { Card, Col, Flex, List, Progress, Row, Statistic, Table, Tag, type TableProps } from "antd";
import { useCallback, useEffect, useMemo, useState } from "react";
import { PageEmpty, PageError, PageLoading } from "@/components/layout/page-feedback";
import { PageHeader } from "@/components/layout/page-header";
import type { InventorySnapshot, ProductRankingRow, SalesSummary, SalesTrendRow, ShopRankingRow } from "@/features/types";
import { apiGet } from "@/lib/api";
import { formatMoney, formatQuantity } from "@/lib/format";

export default function ReportsPage() {
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
        apiGet<{ summary: SalesSummary }>("/reports/sales-summary"),
        apiGet<{ items: SalesTrendRow[] }>("/reports/sales-trend"),
        apiGet<{ items: ProductRankingRow[] }>("/reports/product-ranking"),
        apiGet<{ items: ShopRankingRow[] }>("/reports/shop-ranking"),
        apiGet<{ items: InventorySnapshot[] }>("/inventory?all=true"),
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
  }, []);

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
  if (!summary) return <PageEmpty title="暂无报表" />;

  const lowStockColumns: TableProps<InventorySnapshot>["columns"] = [
    { title: "商品", dataIndex: ["Product", "Name"], render: (value: string) => <strong>{value}</strong> },
    { title: "编码", dataIndex: ["Product", "Code"], render: (value: string) => <span className="mono">{value}</span> },
    { title: "当前库存", dataIndex: "Quantity", render: formatQuantity },
    { title: "阈值", dataIndex: ["Product", "LowStockThreshold"], render: formatQuantity },
  ];

  return (
    <Flex gap={20} vertical>
      <PageHeader description="销售额、成本、毛利、库存金额和低库存概览。" title="报表" />
      <Row gutter={[12, 12]}>
        <Col lg={5} sm={12} xs={24}><Metric label="销售额" value={formatMoney(summary.revenue_cents)} /></Col>
        <Col lg={5} sm={12} xs={24}><Metric label="销售成本" value={formatMoney(summary.cost_cents)} /></Col>
        <Col lg={5} sm={12} xs={24}><Metric label="毛利" value={formatMoney(summary.gross_profit_cents)} /></Col>
        <Col lg={5} sm={12} xs={24}><Metric label="库存金额" value={formatMoney(inventoryValue)} /></Col>
        <Col lg={4} sm={12} xs={24}><Metric label="低库存品类" value={formatQuantity(lowStock.length)} /></Col>
      </Row>
      <Row gutter={[16, 16]}>
        <Col xl={14} xs={24}><TrendPanel rows={trend} /></Col>
        <Col xl={10} xs={24}>
          <RankingPanel
            rows={products.map((item) => ({
              id: item.product_id,
              label: item.product_name,
              sublabel: item.product_code,
              archived: item.archived,
              revenue: item.revenue_cents,
              quantity: item.quantity_sold,
              gross: item.gross_profit_cents,
            }))}
            title="商品销售排行"
          />
        </Col>
      </Row>
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
      <Card className="table-card" title="低库存列表">
        <Table<InventorySnapshot>
          columns={lowStockColumns}
          dataSource={lowStock}
          pagination={false}
          rowKey="ProductID"
          scroll={{ x: 620 }}
          size="small"
        />
      </Card>
    </Flex>
  );
}

function TrendPanel({ rows }: { rows: SalesTrendRow[] }) {
  const maxRevenue = Math.max(...rows.map((row) => row.revenue_cents), 0);
  return (
    <Card title="近 30 天销售趋势">
      <List<SalesTrendRow>
        dataSource={rows}
        locale={{ emptyText: "暂无销售趋势" }}
        renderItem={(row) => (
          <List.Item>
            <Flex gap={7} style={{ width: "100%" }} vertical>
              <Flex justify="space-between">
                <span className="mono muted">{row.day}</span>
                <strong>{formatMoney(row.revenue_cents)}</strong>
              </Flex>
              <Progress percent={percent(row.revenue_cents, maxRevenue)} showInfo={false} size="small" />
              <Flex className="muted" justify="space-between">
                <span>销量 {formatQuantity(row.quantity_sold)}</span>
                <span>毛利 {formatMoney(row.gross_profit_cents)}</span>
              </Flex>
            </Flex>
          </List.Item>
        )}
      />
    </Card>
  );
}

type RankingItem = {
  id: string;
  label: string;
  sublabel?: string;
  archived?: boolean;
  revenue: number;
  quantity: number;
  gross: number;
};

function RankingPanel({ title, rows }: { title: string; rows: RankingItem[] }) {
  const maxRevenue = Math.max(...rows.map((row) => row.revenue), 0);
  return (
    <Card title={title}>
      <List<RankingItem>
        dataSource={rows}
        locale={{ emptyText: "暂无销售排行" }}
        renderItem={(row, index) => (
          <List.Item>
            <Flex gap={7} style={{ width: "100%" }} vertical>
              <Flex align="flex-start" gap={12} justify="space-between">
                <div>
                  <Flex align="center" gap={6}>
                    <strong>{index + 1}. {row.label}</strong>
                    {row.archived ? <Tag>已归档</Tag> : null}
                  </Flex>
                  {row.sublabel ? <span className="mono muted">{row.sublabel}</span> : null}
                </div>
                <div style={{ textAlign: "right" }}>
                  <strong>{formatMoney(row.revenue)}</strong>
                  <div className="muted">销量 {formatQuantity(row.quantity)}</div>
                </div>
              </Flex>
              <Progress percent={percent(row.revenue, maxRevenue)} showInfo={false} size="small" />
              <span className="muted">毛利 {formatMoney(row.gross)}</span>
            </Flex>
          </List.Item>
        )}
      />
    </Card>
  );
}

function Metric({ label, value }: { label: string; value: string }) {
  return <Card className="metric-card"><Statistic title={label} value={value} /></Card>;
}

function percent(value: number, max: number): number {
  if (max <= 0) return 0;
  return Math.max(4, Math.round((value / max) * 100));
}
