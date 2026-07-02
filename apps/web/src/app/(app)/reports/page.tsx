"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { PageHeader } from "@/components/layout/page-header";
import { EmptyBlock, ErrorBlock, LoadingBlock } from "@/components/ui/state";
import type { InventorySnapshot, SalesSummary } from "@/features/types";
import { apiGet } from "@/lib/api";
import { formatMoney, formatQuantity } from "@/lib/format";

export default function ReportsPage() {
  const [summary, setSummary] = useState<SalesSummary | null>(null);
  const [inventory, setInventory] = useState<InventorySnapshot[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const [report, stock] = await Promise.all([
        apiGet<{ summary: SalesSummary }>("/reports/sales-summary"),
        apiGet<{ items: InventorySnapshot[] }>("/inventory"),
      ]);
      setSummary(report.summary);
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
  const lowStock = inventory.filter((item) => item.Product.LowStockThreshold > 0 && item.Quantity <= item.Product.LowStockThreshold);

  if (loading) return <LoadingBlock label="加载报表" />;
  if (error) return <ErrorBlock message={error} onRetry={load} />;
  if (!summary) return <EmptyBlock title="暂无报表" />;

  return (
    <div className="space-y-5">
      <PageHeader title="报表" description="销售额、成本、毛利、库存金额和低库存概览。" />
      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-5">
        <Metric label="销售额" value={formatMoney(summary.revenue_cents)} />
        <Metric label="销售成本" value={formatMoney(summary.cost_cents)} />
        <Metric label="毛利" value={formatMoney(summary.gross_profit_cents)} />
        <Metric label="库存金额" value={formatMoney(inventoryValue)} />
        <Metric label="低库存品类" value={formatQuantity(lowStock.length)} />
      </div>
      <section className="rounded-lg border border-[var(--border-subtle)] bg-white">
        <div className="border-b border-[var(--border-subtle)] px-4 py-3 font-medium">低库存列表</div>
        {lowStock.length === 0 ? (
          <div className="p-4"><EmptyBlock title="没有低库存商品" /></div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full min-w-[720px] text-left text-sm">
              <thead className="border-b border-[var(--border-subtle)] text-xs text-[var(--text-secondary)]">
                <tr><th className="px-4 py-3 font-medium">商品</th><th className="px-4 py-3 font-medium">编码</th><th className="px-4 py-3 font-medium">当前库存</th><th className="px-4 py-3 font-medium">阈值</th></tr>
              </thead>
              <tbody>
                {lowStock.map((item) => (
                  <tr className="border-b border-[var(--border-subtle)] last:border-0" key={item.ProductID}>
                    <td className="px-4 py-3 font-medium">{item.Product.Name}</td>
                    <td className="px-4 py-3 font-mono text-xs">{item.Product.Code}</td>
                    <td className="px-4 py-3">{formatQuantity(item.Quantity)}</td>
                    <td className="px-4 py-3">{formatQuantity(item.Product.LowStockThreshold)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>
    </div>
  );
}

function Metric({ label, value }: { label: string; value: string }) {
  return (
    <section className="rounded-lg border border-[var(--border-subtle)] bg-white p-4">
      <div className="text-xs text-[var(--text-secondary)]">{label}</div>
      <div className="mt-2 text-xl font-semibold">{value}</div>
    </section>
  );
}
