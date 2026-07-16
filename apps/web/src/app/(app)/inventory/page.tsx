"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import Image from "next/image";
import { ImageIcon } from "lucide-react";
import { PageHeader } from "@/components/layout/page-header";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { MessageBar } from "@/components/ui/message";
import { initialPagination, Pagination } from "@/components/ui/pagination";
import { EmptyBlock, ErrorBlock, LoadingBlock } from "@/components/ui/state";
import { InventoryActions } from "@/features/inventory/action-forms";
import { StockBadge } from "@/features/labels";
import type { InventorySnapshot, Paginated, Product, Shop } from "@/features/types";
import { useMessage } from "@/features/use-message";
import { apiGet } from "@/lib/api";
import { formatDateTime, formatMoney, formatQuantity } from "@/lib/format";

export default function InventoryPage() {
  const [inventory, setInventory] = useState<InventorySnapshot[]>([]);
  const [visibleInventory, setVisibleInventory] = useState<InventorySnapshot[]>([]);
  const [products, setProducts] = useState<Product[]>([]);
  const [shops, setShops] = useState<Shop[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [showLowStock, setShowLowStock] = useState(false);
  const [page, setPage] = useState(1);
  const [pagination, setPagination] = useState(initialPagination);
  const { message, show } = useMessage();

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

  function done(value: string) {
    show(value);
    void load();
  }

  return (
    <div className="space-y-5">
      <PageHeader
        title="当前库存"
        description="库存快照由入库、销售出库和调整流水自动更新。"
        actions={<InventoryActions inventory={inventory} products={products} shops={shops} onDone={done} />}
      />

      <div className="grid gap-3 sm:grid-cols-3">
        <Summary label="库存品类" value={formatQuantity(inventory.length)} />
        <Summary label="库存金额" value={formatMoney(inventoryValue)} />
        <Summary active={showLowStock} label="低库存" value={formatQuantity(lowStock.length)} onClick={() => { setPage(1); setShowLowStock(true); }} />
      </div>

      {showLowStock ? (
        <div className="flex items-center justify-between gap-3 rounded-lg border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800">
          <span>当前仅显示低库存商品</span>
          <Button type="button" variant="secondary" onClick={() => { setPage(1); setShowLowStock(false); }}>显示全部</Button>
        </div>
      ) : null}
      {loading ? <LoadingBlock label="加载库存" /> : error ? <ErrorBlock message={error} onRetry={load} /> : <InventoryTable inventory={visibleInventory} />}
      <Pagination meta={pagination} onPageChange={setPage} />
      <MessageBar message={message} />
    </div>
  );
}

function InventoryTable({ inventory }: { inventory: InventorySnapshot[] }) {
  if (inventory.length === 0) {
    return <EmptyBlock title="还没有库存记录" />;
  }
  return (
    <div className="overflow-x-auto rounded-lg border border-[var(--border-subtle)] bg-white">
      <table className="w-full min-w-[900px] text-left text-sm">
        <thead className="border-b border-[var(--border-subtle)] text-xs text-[var(--text-secondary)]">
          <tr>
            <th className="px-4 py-3 font-medium">商品</th>
            <th className="px-4 py-3 font-medium">数量</th>
            <th className="px-4 py-3 font-medium">移动平均成本</th>
            <th className="px-4 py-3 font-medium">库存金额</th>
            <th className="px-4 py-3 font-medium">状态</th>
            <th className="px-4 py-3 font-medium">更新时间</th>
          </tr>
        </thead>
        <tbody>
          {inventory.map((item) => (
            <tr className="border-b border-[var(--border-subtle)] last:border-0 hover:bg-black/[0.02]" key={item.ProductID}>
              <td className="px-4 py-3">
                <div className="flex items-center gap-3">
                  {item.Product.ImagePath ? (
                    <Image alt={`${item.Product.Name} 图片`} className="h-10 w-10 shrink-0 rounded-md border border-[var(--border-subtle)] object-cover" height={40} src={item.Product.ImagePath} unoptimized width={40} />
                  ) : (
                    <div aria-label="无商品图片" className="grid h-10 w-10 shrink-0 place-items-center rounded-md border border-[var(--border-subtle)] bg-black/[0.03] text-[var(--text-muted)]">
                      <ImageIcon className="h-4 w-4" />
                    </div>
                  )}
                  <div className="min-w-0">
                    <div className="truncate font-medium">{item.Product.Name}</div>
                    <div className="truncate font-mono text-xs text-[var(--text-secondary)]">{item.Product.Code}</div>
                  </div>
                </div>
              </td>
              <td className="px-4 py-3"><Badge>{formatQuantity(item.Quantity)}</Badge></td>
              <td className="px-4 py-3">{formatMoney(item.MovingAverageCostCents)}</td>
              <td className="px-4 py-3">{formatMoney(item.InventoryValueCents)}</td>
              <td className="px-4 py-3"><StockBadge quantity={item.Quantity} threshold={item.Product.LowStockThreshold} /></td>
              <td className="px-4 py-3 text-[var(--text-secondary)]">{formatDateTime(item.UpdatedAt)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function Summary({ label, value, active = false, onClick }: { label: string; value: string; active?: boolean; onClick?: () => void }) {
  const className = `w-full rounded-lg border bg-white p-4 text-left ${active ? "border-amber-400 ring-2 ring-amber-100" : "border-[var(--border-subtle)]"}`;
  const content = (
    <>
      <div className="text-xs text-[var(--text-secondary)]">{label}</div>
      <div className="mt-2 text-xl font-semibold">{value}</div>
    </>
  );
  if (onClick) {
    return <button aria-pressed={active} className={`${className} transition hover:border-amber-400 focus:outline-none focus:ring-2 focus:ring-[var(--accent-primary)]`} type="button" onClick={onClick}>{content}</button>;
  }
  return <div className={className}>{content}</div>;
}
