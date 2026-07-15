"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import Image from "next/image";
import { ImageIcon } from "lucide-react";
import { PageHeader } from "@/components/layout/page-header";
import { Badge } from "@/components/ui/badge";
import { MessageBar } from "@/components/ui/message";
import { EmptyBlock, ErrorBlock, LoadingBlock } from "@/components/ui/state";
import { InventoryActions } from "@/features/inventory/action-forms";
import { StockBadge } from "@/features/labels";
import type { InventorySnapshot, Product, Shop } from "@/features/types";
import { useMessage } from "@/features/use-message";
import { apiGet } from "@/lib/api";
import { formatDateTime, formatMoney, formatQuantity } from "@/lib/format";

export default function InventoryPage() {
  const [inventory, setInventory] = useState<InventorySnapshot[]>([]);
  const [products, setProducts] = useState<Product[]>([]);
  const [shops, setShops] = useState<Shop[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const { message, show } = useMessage();

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const [stock, productList, shopList] = await Promise.all([
        apiGet<{ items: InventorySnapshot[] }>("/inventory"),
        apiGet<{ items: Product[] }>("/products"),
        apiGet<{ items: Shop[] }>("/shops"),
      ]);
      setInventory(stock.items);
      setProducts(productList.items);
      setShops(shopList.items);
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
        <Summary label="低库存" value={formatQuantity(inventory.filter((item) => item.Product.LowStockThreshold > 0 && item.Quantity <= item.Product.LowStockThreshold).length)} />
      </div>

      {loading ? <LoadingBlock label="加载库存" /> : error ? <ErrorBlock message={error} onRetry={load} /> : <InventoryTable inventory={inventory} />}
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

function Summary({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-lg border border-[var(--border-subtle)] bg-white p-4">
      <div className="text-xs text-[var(--text-secondary)]">{label}</div>
      <div className="mt-2 text-xl font-semibold">{value}</div>
    </div>
  );
}
