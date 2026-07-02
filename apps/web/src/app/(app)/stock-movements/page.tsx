"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { PageHeader } from "@/components/layout/page-header";
import { Field, Select } from "@/components/ui/fields";
import { EmptyBlock, ErrorBlock, LoadingBlock } from "@/components/ui/state";
import { MovementBadge } from "@/features/labels";
import type { MovementType, Product, Shop, StockMovement } from "@/features/types";
import { apiGet } from "@/lib/api";
import { formatDateTime, formatMoney, formatQuantity } from "@/lib/format";

export default function StockMovementsPage() {
  const [movements, setMovements] = useState<StockMovement[]>([]);
  const [products, setProducts] = useState<Product[]>([]);
  const [shops, setShops] = useState<Shop[]>([]);
  const [type, setType] = useState("");
  const [productID, setProductID] = useState("");
  const [shopID, setShopID] = useState("");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  const params = useMemo(() => {
    const query = new URLSearchParams();
    if (type) query.set("type", type);
    if (productID) query.set("product_id", productID);
    if (shopID) query.set("shop_id", shopID);
    return query.toString();
  }, [productID, shopID, type]);

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const [movementList, productList, shopList] = await Promise.all([
        apiGet<{ items: StockMovement[] }>(`/stock-movements${params ? `?${params}` : ""}`),
        apiGet<{ items: Product[] }>("/products"),
        apiGet<{ items: Shop[] }>("/shops"),
      ]);
      setMovements(movementList.items);
      setProducts(productList.items);
      setShops(shopList.items);
    } catch (err) {
      setError(err instanceof Error ? err.message : "加载失败");
    } finally {
      setLoading(false);
    }
  }, [params]);

  useEffect(() => {
    void load();
  }, [load]);

  return (
    <div className="space-y-5">
      <PageHeader title="流水记录" description="按类型、商品和店铺查看不可变库存流水。" />
      <div className="grid gap-3 rounded-lg border border-[var(--border-subtle)] bg-white p-3 sm:grid-cols-3">
        <Field label="类型">
          <Select value={type} onChange={(event) => setType(event.target.value)}>
            <option value="">全部</option>
            <option value="inbound">入库</option>
            <option value="sales_outbound">销售出库</option>
            <option value="adjustment">调整</option>
          </Select>
        </Field>
        <Field label="商品">
          <Select value={productID} onChange={(event) => setProductID(event.target.value)}>
            <option value="">全部</option>
            {products.map((product) => <option key={product.ID} value={product.ID}>{product.Name}</option>)}
          </Select>
        </Field>
        <Field label="店铺">
          <Select value={shopID} onChange={(event) => setShopID(event.target.value)}>
            <option value="">全部</option>
            {shops.map((shop) => <option key={shop.ID} value={shop.ID}>{shop.Name}</option>)}
          </Select>
        </Field>
      </div>
      {loading ? <LoadingBlock label="加载流水" /> : error ? <ErrorBlock message={error} onRetry={load} /> : <MovementTable movements={movements} />}
    </div>
  );
}

function MovementTable({ movements }: { movements: StockMovement[] }) {
  if (movements.length === 0) {
    return <EmptyBlock title="没有符合条件的流水" />;
  }
  return (
    <div className="overflow-x-auto rounded-lg border border-[var(--border-subtle)] bg-white">
      <table className="w-full min-w-[1040px] text-left text-sm">
        <thead className="border-b border-[var(--border-subtle)] text-xs text-[var(--text-secondary)]">
          <tr>
            <th className="px-4 py-3 font-medium">类型</th>
            <th className="px-4 py-3 font-medium">商品</th>
            <th className="px-4 py-3 font-medium">店铺</th>
            <th className="px-4 py-3 font-medium">数量</th>
            <th className="px-4 py-3 font-medium">收入</th>
            <th className="px-4 py-3 font-medium">成本</th>
            <th className="px-4 py-3 font-medium">毛利</th>
            <th className="px-4 py-3 font-medium">备注</th>
            <th className="px-4 py-3 font-medium">时间</th>
          </tr>
        </thead>
        <tbody>
          {movements.map((movement) => (
            <tr className="border-b border-[var(--border-subtle)] last:border-0 hover:bg-black/[0.02]" key={movement.ID}>
              <td className="px-4 py-3"><MovementBadge type={movement.Type as MovementType} /></td>
              <td className="px-4 py-3 font-medium">{movement.Product?.Name ?? "-"}</td>
              <td className="px-4 py-3">{movement.Shop?.Name ?? "-"}</td>
              <td className="px-4 py-3">{formatQuantity(movement.QuantityDelta)}</td>
              <td className="px-4 py-3">{formatMoney(movement.RevenueCents)}</td>
              <td className="px-4 py-3">{formatMoney(movement.CostAmountCents || movement.PurchaseAmountCents)}</td>
              <td className="px-4 py-3">{formatMoney(movement.GrossProfitCents)}</td>
              <td className="max-w-[220px] truncate px-4 py-3 text-[var(--text-secondary)]">{movement.Reason || "-"}</td>
              <td className="px-4 py-3 text-[var(--text-secondary)]">{formatDateTime(movement.CreatedAt)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
