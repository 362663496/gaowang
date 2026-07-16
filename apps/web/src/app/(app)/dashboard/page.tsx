"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { ArrowDownRight, ArrowUpRight, Boxes, Package } from "lucide-react";
import { PageHeader } from "@/components/layout/page-header";
import { Badge } from "@/components/ui/badge";
import { EmptyBlock, ErrorBlock, LoadingBlock } from "@/components/ui/state";
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

  if (loading) {
    return <LoadingBlock label="加载仪表盘" />;
  }

  if (error) {
    return <ErrorBlock message={error} onRetry={load} />;
  }

  if (!data) {
    return <EmptyBlock title="暂无数据" />;
  }

  return (
    <div className="space-y-5">
      <PageHeader title="仪表盘" description="销售、毛利、库存风险和最近库存流水。" />

      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        <Metric label="累计销售额" value={formatMoney(data.summary.revenue_cents)} icon={<ArrowUpRight className="h-4 w-4" />} />
        <Metric label="累计毛利" value={formatMoney(data.summary.gross_profit_cents)} icon={<ArrowDownRight className="h-4 w-4" />} />
        <Metric label="库存品类" value={formatQuantity(data.inventory.length)} icon={<Boxes className="h-4 w-4" />} />
        <Metric label="低库存" value={formatQuantity(lowStock.length)} icon={<Package className="h-4 w-4" />} warning={lowStock.length > 0} />
      </div>

      <div className="grid gap-4 xl:grid-cols-[minmax(0,1.2fr)_minmax(360px,0.8fr)]">
        <section className="rounded-lg border border-[var(--border-subtle)] bg-white">
          <div className="border-b border-[var(--border-subtle)] px-4 py-3 font-medium">最近流水</div>
          {data.movements.length === 0 ? (
            <div className="p-4">
              <EmptyBlock title="暂无库存流水" />
            </div>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full min-w-[760px] text-left text-sm">
                <thead className="border-b border-[var(--border-subtle)] text-xs text-[var(--text-secondary)]">
                  <tr>
                    <th className="px-4 py-3 font-medium">类型</th>
                    <th className="px-4 py-3 font-medium">商品</th>
                    <th className="px-4 py-3 font-medium">数量</th>
                    <th className="px-4 py-3 font-medium">金额</th>
                    <th className="px-4 py-3 font-medium">时间</th>
                  </tr>
                </thead>
                <tbody>
                  {data.movements.slice(0, 8).map((movement) => (
                    <tr className="border-b border-[var(--border-subtle)] last:border-0 hover:bg-black/[0.02]" key={movement.ID}>
                      <td className="px-4 py-3"><MovementBadge type={movement.Type} /></td>
                      <td className="px-4 py-3 font-medium">{movement.Product?.Name ?? "-"}</td>
                      <td className="px-4 py-3">{formatQuantity(movement.QuantityDelta)}</td>
                      <td className="px-4 py-3">{formatMoney(movement.RevenueCents || movement.PurchaseAmountCents || movement.CostAmountCents)}</td>
                      <td className="px-4 py-3 text-[var(--text-secondary)]">{formatDateTime(movement.CreatedAt)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </section>

        <section className="rounded-lg border border-[var(--border-subtle)] bg-white">
          <div className="border-b border-[var(--border-subtle)] px-4 py-3 font-medium">库存风险</div>
          {lowStock.length === 0 ? (
            <div className="p-4">
              <EmptyBlock title="没有低库存商品" />
            </div>
          ) : (
            <div className="divide-y divide-[var(--border-subtle)]">
              {lowStock.slice(0, 8).map((item) => (
                <div className="flex items-center justify-between gap-3 px-4 py-3" key={item.ProductID}>
                  <div className="min-w-0">
                    <div className="truncate font-medium">{item.Product.Name}</div>
                    <div className="text-xs text-[var(--text-secondary)]">{item.Product.Code}</div>
                  </div>
                  <div className="flex shrink-0 items-center gap-2">
                    <StockBadge quantity={item.Quantity} threshold={item.Product.LowStockThreshold} />
                    <Badge>{formatQuantity(item.Quantity)}</Badge>
                  </div>
                </div>
              ))}
            </div>
          )}
        </section>
      </div>
    </div>
  );
}

function Metric({ label, value, icon, warning }: { label: string; value: string; icon: React.ReactNode; warning?: boolean }) {
  return (
    <section className="rounded-lg border border-[var(--border-subtle)] bg-white p-4">
      <div className="flex items-center justify-between gap-3 text-sm text-[var(--text-secondary)]">
        <span>{label}</span>
        <span className={warning ? "text-[var(--status-warning)]" : "text-[var(--text-muted)]"}>{icon}</span>
      </div>
      <div className="mt-3 text-2xl font-semibold">{value}</div>
    </section>
  );
}
