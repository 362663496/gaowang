"use client";

import { ArrowDownOutlined, ArrowUpOutlined, AppstoreOutlined, WarningOutlined } from "@ant-design/icons";
import { Card, Col, Flex, List, Result, Row, Statistic, Table, Tag, type TableProps } from "antd";
import { useCallback, useEffect, useMemo, useState } from "react";
import { PageEmpty, PageError, PageLoading } from "@/components/layout/page-feedback";
import { PageHeader } from "@/components/layout/page-header";
import { useSession } from "@/components/layout/session-context";
import { MovementBadge, StockBadge } from "@/features/labels";
import { ProductIdentity } from "@/features/product-identity";
import type { InventorySnapshot, SalesSummary, StockMovement } from "@/features/types";
import { apiGet } from "@/lib/api";
import { formatDateTime, formatMoney, formatQuantity } from "@/lib/format";

type DashboardData = {
  inventory: InventorySnapshot[];
  movements: StockMovement[];
  summary: SalesSummary | null;
};

export default function DashboardPage() {
  const { hasPermission, permissions } = useSession();
  const canInventory = hasPermission("inventory.read");
  const canMovements = hasPermission("movement.read");
  const canSales = hasPermission("report.sales_summary");
  const [data, setData] = useState<DashboardData | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);

  const load = useCallback(async () => {
    if (!canInventory && !canMovements && !canSales) {
      setData({ inventory: [], movements: [], summary: null });
      setLoading(false);
      return;
    }
    setLoading(true);
    setError("");
    try {
      const [inventory, movements, report] = await Promise.all([
        canInventory ? apiGet<{ items: InventorySnapshot[] }>("/inventory?all=true") : Promise.resolve({ items: [] as InventorySnapshot[] }),
        canMovements ? apiGet<{ items: StockMovement[] }>("/stock-movements?page_size=8") : Promise.resolve({ items: [] as StockMovement[] }),
        canSales ? apiGet<{ summary: SalesSummary }>("/reports/sales-summary") : Promise.resolve({ summary: null as SalesSummary | null }),
      ]);
      setData({ inventory: inventory.items, movements: movements.items, summary: report.summary });
    } catch (err) {
      setError(err instanceof Error ? err.message : "加载失败");
    } finally {
      setLoading(false);
    }
  }, [canInventory, canMovements, canSales]);

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
  if (!canInventory && !canMovements && !canSales) {
    return (
      <Result
        status="info"
        subTitle={permissions.length === 0 ? "请联系管理员分配业务权限。" : "当前权限无法展示仪表盘数据块。"}
        title="暂无可展示内容"
      />
    );
  }

  const columns: TableProps<StockMovement>["columns"] = [
    { title: "类型", dataIndex: "Type", width: 110, render: (value) => <MovementBadge type={value} /> },
    { title: "商品", dataIndex: "Product", width: 240, render: (_, movement) => <ProductIdentity product={movement.Product} /> },
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
        {canSales && data.summary ? (
          <>
            <Col lg={6} sm={12} xs={24}><Metric icon={<ArrowUpOutlined />} label="累计销售额" value={formatMoney(data.summary.revenue_cents)} /></Col>
            <Col lg={6} sm={12} xs={24}><Metric icon={<ArrowDownOutlined />} label="累计毛利" value={formatMoney(data.summary.gross_profit_cents)} /></Col>
          </>
        ) : null}
        {canInventory ? (
          <>
            <Col lg={6} sm={12} xs={24}><Metric icon={<AppstoreOutlined />} label="库存品类" value={formatQuantity(data.inventory.length)} /></Col>
            <Col lg={6} sm={12} xs={24}><Metric icon={<WarningOutlined />} label="低库存" value={formatQuantity(lowStock.length)} warning={lowStock.length > 0} /></Col>
          </>
        ) : null}
      </Row>

      <Row gutter={[16, 16]}>
        {canMovements ? (
          <Col xl={15} xs={24}>
            <Card className="table-card" title="最近流水">
              <Table<StockMovement>
                columns={columns}
                dataSource={data.movements}
                pagination={false}
                rowKey="ID"
                scroll={{ x: 700 }}
                size="middle"
              />
            </Card>
          </Col>
        ) : null}
        {canInventory ? (
          <Col xl={canMovements ? 9 : 24} xs={24}>
            <Card title="低库存预警">
              {lowStock.length === 0 ? (
                <PageEmpty title="暂无低库存商品" />
              ) : (
                <List
                  dataSource={lowStock.slice(0, 8)}
                  renderItem={(item) => (
                    <List.Item>
                      <List.Item.Meta
                        title={<ProductIdentity product={item.Product} />}
                      />
                      <StockBadge quantity={item.Quantity} threshold={item.Product.LowStockThreshold} />
                    </List.Item>
                  )}
                />
              )}
            </Card>
          </Col>
        ) : null}
      </Row>
    </Flex>
  );
}

function Metric({ icon, label, value, warning = false }: { icon: React.ReactNode; label: string; value: string; warning?: boolean }) {
  return (
    <Card className="metric-card">
      <Statistic
        prefix={icon}
        title={label}
        value={value}
        valueStyle={warning ? { color: "#cf1322" } : undefined}
      />
      {warning ? <Tag color="error" style={{ marginTop: 8 }}>需关注</Tag> : null}
    </Card>
  );
}
