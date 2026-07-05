"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { PageHeader } from "@/components/layout/page-header";
import { EmptyBlock, ErrorBlock, LoadingBlock } from "@/components/ui/state";
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
        apiGet<{ items: InventorySnapshot[] }>("/inventory"),
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
      <div className="grid gap-4 xl:grid-cols-[minmax(0,1.1fr)_minmax(360px,0.9fr)]">
        <TrendPanel rows={trend} />
        <RankingPanel title="商品销售排行" rows={products.map((item) => ({ id: item.product_id, label: item.product_name, sublabel: item.product_code, revenue: item.revenue_cents, quantity: item.quantity_sold, gross: item.gross_profit_cents }))} />
      </div>
      <RankingPanel title="店铺销售排行" rows={shops.map((item) => ({ id: item.shop_id, label: item.shop_name, revenue: item.revenue_cents, quantity: item.quantity_sold, gross: item.gross_profit_cents }))} />
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

function TrendPanel({ rows }: { rows: SalesTrendRow[] }) {
  const maxRevenue = Math.max(...rows.map((row) => row.revenue_cents), 0);
  return (
    <section className="rounded-lg border border-[var(--border-subtle)] bg-white">
      <div className="border-b border-[var(--border-subtle)] px-4 py-3 font-medium">近 30 天销售趋势</div>
      {rows.length === 0 ? (
        <div className="p-4"><EmptyBlock title="暂无销售趋势" /></div>
      ) : (
        <div className="grid gap-3 p-4">
          {rows.map((row) => (
            <div className="grid gap-2" key={row.day}>
              <div className="flex items-center justify-between gap-3 text-sm">
                <span className="font-mono text-xs text-[var(--text-secondary)]">{row.day}</span>
                <span className="font-medium">{formatMoney(row.revenue_cents)}</span>
              </div>
              <div className="h-2 overflow-hidden rounded-full bg-black/[0.05]">
                <div className="h-full rounded-full bg-[var(--accent-primary)]" style={{ width: percent(row.revenue_cents, maxRevenue) }} />
              </div>
              <div className="flex items-center justify-between text-xs text-[var(--text-secondary)]">
                <span>销量 {formatQuantity(row.quantity_sold)}</span>
                <span>毛利 {formatMoney(row.gross_profit_cents)}</span>
              </div>
            </div>
          ))}
        </div>
      )}
    </section>
  );
}

type RankingItem = {
  id: string;
  label: string;
  sublabel?: string;
  revenue: number;
  quantity: number;
  gross: number;
};

function RankingPanel({ title, rows }: { title: string; rows: RankingItem[] }) {
  const maxRevenue = Math.max(...rows.map((row) => row.revenue), 0);
  return (
    <section className="rounded-lg border border-[var(--border-subtle)] bg-white">
      <div className="border-b border-[var(--border-subtle)] px-4 py-3 font-medium">{title}</div>
      {rows.length === 0 ? (
        <div className="p-4"><EmptyBlock title="暂无销售排行" /></div>
      ) : (
        <div className="divide-y divide-[var(--border-subtle)]">
          {rows.map((row, index) => (
            <div className="grid gap-2 px-4 py-3" key={row.id}>
              <div className="flex items-center justify-between gap-3">
                <div className="min-w-0">
                  <div className="truncate font-medium">{index + 1}. {row.label}</div>
                  {row.sublabel ? <div className="text-xs text-[var(--text-secondary)]">{row.sublabel}</div> : null}
                </div>
                <div className="shrink-0 text-right">
                  <div className="font-medium">{formatMoney(row.revenue)}</div>
                  <div className="text-xs text-[var(--text-secondary)]">销量 {formatQuantity(row.quantity)}</div>
                </div>
              </div>
              <div className="h-2 overflow-hidden rounded-full bg-black/[0.05]">
                <div className="h-full rounded-full bg-[var(--accent-primary)]" style={{ width: percent(row.revenue, maxRevenue) }} />
              </div>
              <div className="text-xs text-[var(--text-secondary)]">毛利 {formatMoney(row.gross)}</div>
            </div>
          ))}
        </div>
      )}
    </section>
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

function percent(value: number, max: number): string {
  if (max <= 0) {
    return "0%";
  }
  return `${Math.max(4, Math.round((value / max) * 100))}%`;
}
