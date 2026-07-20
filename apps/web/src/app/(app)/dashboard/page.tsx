"use client";

import { ArrowDownOutlined, ArrowUpOutlined, AppstoreOutlined, WarningOutlined } from "@ant-design/icons";
import { Card, Col, Flex, List, Row, Statistic, Table, Tag, type TableProps } from "antd";
import { useCallback, useEffect, useMemo, useState } from "react";
import { PageEmpty, PageError, PageLoading } from "@/components/layout/page-feedback";
import { PageHeader } from "@/components/layout/page-header";
import { MovementBadge, StockBadge } from "@/features/labels";
import type { InventorySnapshot, SalesSummary, StockMovement } from "@/features/types";
import { apiGet } from "@/lib/api";
import { formatDateTime, formatMoney, formatQuantity } from "@/lib/format";

type DashboardData = {
  inventory: InventorySnapshot[];
  movements: StockMovement[];
  summary: SalesSummary;
};

export default function DashboardPage() {
  const [data, setData] = useState<DashboardData | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const [inventory, movements, report] = await Promise.all([
        apiGet<{ items: InventorySnapshot[] }>("/inventory?all=true"),
        apiGet<{ items: StockMovement[] }>("/stock-movements?page_size=8"),
        apiGet<{ summary: SalesSummary }>("/reports/sales-summary"),
      ]);
      setData({ inventory: inventory.items, movements: movements.items, summary: report.summary });
    } catch (err) {
      setError(err instanceof Error ? err.message : "加载失败");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const lowStock = useMemo(
    () => data?.inventory.filter((item) => item.Product.LowStockThreshold > 0 && item.Quantity <= item.Product.LowStockThreshold) ?? [],
    [data],
  );

  if (loading) return <PageLoading label="加载仪表盘" />;
  if (error) return <PageError message={error} onRetry={() => void load()} />;
  if (!data) return <PageEmpty title="暂无数据" />;

  const columns: TableProps<StockMovement>["columns"] = [
    { title: "类型", dataIndex: "Type", width: 110, render: (value) => <MovementBadge type={value} /> },
    { title: "商品", dataIndex: ["Product", "Name"], ellipsis: true, render: (value?: string) => <strong>{value ?? "-"}</strong> },
    { title: "数量", dataIndex: "QuantityDelta", width: 100, render: formatQuantity },
    {
      title: "金额",
      width: 130,
      render: (_, movement) => formatMoney(movement.RevenueCents || movement.PurchaseAmountCents || movement.CostAmountCents),
    },
    { title: "时间", dataIndex: "CreatedAt", width: 180, render: (value: string) => <span className="muted">{formatDateTime(value)}</span> },
  ];

  return (
    <Flex gap={20} vertical>
      <PageHeader description="销售、毛利、库存风险和最近库存流水。" title="仪表盘" />
      <Row gutter={[12, 12]}>
        <Col lg={6} sm={12} xs={24}><Metric icon={<ArrowUpOutlined />} label="累计销售额" value={formatMoney(data.summary.revenue_cents)} /></Col>
        <Col lg={6} sm={12} xs={24}><Metric icon={<ArrowDownOutlined />} label="累计毛利" value={formatMoney(data.summary.gross_profit_cents)} /></Col>
        <Col lg={6} sm={12} xs={24}><Metric icon={<AppstoreOutlined />} label="库存品类" value={formatQuantity(data.inventory.length)} /></Col>
        <Col lg={6} sm={12} xs={24}><Metric icon={<WarningOutlined />} label="低库存" value={formatQuantity(lowStock.length)} warning={lowStock.length > 0} /></Col>
      </Row>

      <Row gutter={[16, 16]}>
        <Col xl={15} xs={24}>
          <Card className="table-card" title="最近流水">
            <Table<StockMovement>
              columns={columns}
              dataSource={data.movements.slice(0, 8)}
              pagination={false}
              rowKey="ID"
              scroll={{ x: 720 }}
              size="small"
            />
          </Card>
        </Col>
        <Col xl={9} xs={24}>
          <Card title="库存风险">
            <List<InventorySnapshot>
              dataSource={lowStock.slice(0, 8)}
              locale={{ emptyText: "没有低库存商品" }}
              renderItem={(item) => (
                <List.Item
                  extra={
                    <Flex align="center" gap={6}>
                      <StockBadge quantity={item.Quantity} threshold={item.Product.LowStockThreshold} />
                      <Tag>{formatQuantity(item.Quantity)}</Tag>
                    </Flex>
                  }
                >
                  <List.Item.Meta description={<span className="mono">{item.Product.Code}</span>} title={item.Product.Name} />
                </List.Item>
              )}
            />
          </Card>
        </Col>
      </Row>
    </Flex>
  );
}

function Metric({ label, value, icon, warning = false }: { label: string; value: string; icon: React.ReactNode; warning?: boolean }) {
  return (
    <Card className="metric-card">
      <Statistic prefix={<span style={{ color: warning ? "var(--status-warning)" : "var(--text-muted)" }}>{icon}</span>} title={label} value={value} />
    </Card>
  );
}
