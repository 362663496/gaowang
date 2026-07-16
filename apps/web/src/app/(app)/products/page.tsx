"use client";

import { FormEvent, useCallback, useEffect, useState } from "react";
import Image from "next/image";
import { ImageIcon, Pencil, Plus, Power, Search, Trash2 } from "lucide-react";
import { PageHeader } from "@/components/layout/page-header";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Dialog, DialogClose, DialogContent, DialogTrigger } from "@/components/ui/dialog";
import { Field, Input, Textarea } from "@/components/ui/fields";
import { MessageBar } from "@/components/ui/message";
import { initialPagination, Pagination } from "@/components/ui/pagination";
import { EmptyBlock, ErrorBlock, LoadingBlock } from "@/components/ui/state";
import type { Paginated, Product } from "@/features/types";
import { useMessage } from "@/features/use-message";
import { apiGet, apiPost, request } from "@/lib/api";
import { centsToYuanInput, formatMoney, formatQuantity, yuanToCents } from "@/lib/format";

type ProductAction = { productID: string; type: "status" | "delete" } | null;
type ProductListResponse = Paginated<Product> & {
  summary: { total: number; enabled: number; default_sale_cents: number };
};

export default function ProductsPage() {
  const [products, setProducts] = useState<Product[]>([]);
  const [query, setQuery] = useState("");
  const [error, setError] = useState("");
  const [actionError, setActionError] = useState("");
  const [loading, setLoading] = useState(true);
  const [open, setOpen] = useState(false);
  const [editing, setEditing] = useState<Product | null>(null);
  const [busyAction, setBusyAction] = useState<ProductAction>(null);
  const [page, setPage] = useState(1);
  const [pagination, setPagination] = useState(initialPagination);
  const [summary, setSummary] = useState({ total: 0, enabled: 0, default_sale_cents: 0 });
  const { message, show } = useMessage();

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    setActionError("");
    try {
      const params = new URLSearchParams({ page: String(page) });
      if (query) params.set("q", query);
      const data = await apiGet<ProductListResponse>(`/products?${params}`);
      setProducts(data.items);
      setPagination(data.pagination);
      setSummary(data.summary);
      if (data.pagination.page !== page) setPage(data.pagination.page);
    } catch (err) {
      setError(err instanceof Error ? err.message : "加载失败");
    } finally {
      setLoading(false);
    }
  }, [page, query]);

  useEffect(() => {
    void load();
  }, [load]);

  async function setProductEnabled(product: Product) {
    setBusyAction({ productID: product.ID, type: "status" });
    setActionError("");
    try {
      const data = await request<{ item: Product }>(`/products/${product.ID}/enabled`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ enabled: !product.Enabled }),
      });
      setProducts((current) => current.map((item) => item.ID === product.ID ? data.item : item));
      setSummary((current) => ({ ...current, enabled: current.enabled + (data.item.Enabled ? 1 : -1) }));
      show(data.item.Enabled ? "商品已启用" : "商品已禁用");
    } catch (err) {
      setActionError(err instanceof Error ? err.message : "更新商品状态失败");
    } finally {
      setBusyAction(null);
    }
  }

  async function deleteProduct(product: Product) {
    if (!window.confirm(`确定删除商品“${product.Name}”吗？未使用商品会彻底删除，有历史的零库存商品会归档。`)) {
      return;
    }
    setBusyAction({ productID: product.ID, type: "delete" });
    setActionError("");
    try {
      await request<void>(`/products/${product.ID}`, { method: "DELETE" });
      show("商品已删除");
      void load();
    } catch (err) {
      setActionError(err instanceof Error ? err.message : "删除商品失败");
    } finally {
      setBusyAction(null);
    }
  }

  return (
    <div className="space-y-5">
      <PageHeader
        title="商品"
        description="管理商品图片、编码、默认价格和低库存阈值。"
        actions={
          <Dialog open={open} onOpenChange={setOpen}>
            <DialogTrigger asChild>
              <Button><Plus className="h-4 w-4" />新增商品</Button>
            </DialogTrigger>
            <DialogContent title="新增商品">
              <ProductForm
                onSaved={() => {
                  setOpen(false);
                  show("商品已创建");
                  if (page === 1) void load();
                  else setPage(1);
                }}
              />
            </DialogContent>
          </Dialog>
        }
      />

      <div className="grid gap-3 sm:grid-cols-3">
        <Summary label="商品数" value={formatQuantity(summary.total)} />
        <Summary label="默认售价合计" value={formatMoney(summary.default_sale_cents)} />
        <Summary label="已启用" value={formatQuantity(summary.enabled)} />
      </div>

      <div className="flex items-center gap-2 rounded-lg border border-[var(--border-subtle)] bg-white p-2">
        <Search className="ml-2 h-4 w-4 text-[var(--text-muted)]" />
        <Input className="border-0 shadow-none focus:ring-0" placeholder="搜索商品名称或编码" value={query} onChange={(event) => { setQuery(event.target.value); setPage(1); }} />
      </div>

      {loading ? <LoadingBlock label="加载商品" /> : error ? <ErrorBlock message={error} onRetry={load} /> : (
        <ProductsTable busyAction={busyAction} onDelete={deleteProduct} onEdit={setEditing} onSetEnabled={setProductEnabled} products={products} />
      )}
      <Pagination meta={pagination} onPageChange={setPage} />
      <Dialog open={editing !== null} onOpenChange={(value) => !value && setEditing(null)}>
        <DialogContent title="修改商品">
          {editing ? (
            <ProductForm
              key={editing.ID}
              product={editing}
              onSaved={() => {
                setEditing(null);
                show("商品已修改");
                void load();
              }}
            />
          ) : null}
        </DialogContent>
      </Dialog>
      <MessageBar message={actionError || message} tone={actionError ? "error" : "success"} />
    </div>
  );
}

function ProductForm({ product, onSaved }: { product?: Product; onSaved: (product: Product) => void }) {
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSaving(true);
    setError("");
    const form = new FormData(event.currentTarget);
    form.set("default_purchase_cents", String(yuanToCents(String(form.get("purchase_yuan") ?? ""))));
    form.set("default_sale_cents", String(yuanToCents(String(form.get("sale_yuan") ?? ""))));
    form.delete("purchase_yuan");
    form.delete("sale_yuan");
    if ((form.get("image") as File)?.size === 0) form.delete("image");
    try {
      const data = product
        ? await request<{ item: Product }>(`/products/${product.ID}`, { method: "PATCH", body: form })
        : await apiPost<{ item: Product }>("/products", form);
      onSaved(data.item);
    } catch (err) {
      setError(err instanceof Error ? err.message : "保存失败");
    } finally {
      setSaving(false);
    }
  }

  return (
    <form className="grid gap-4" onSubmit={submit}>
      {error ? <ErrorBlock message={error} /> : null}
      <div className="grid gap-3 sm:grid-cols-2">
        <Field label="商品名称"><Input defaultValue={product?.Name} name="name" required /></Field>
        <Field label="商品编码"><Input defaultValue={product?.Code} name="code" required /></Field>
        <Field label="默认进货价（元）"><Input defaultValue={centsToYuanInput(product?.DefaultPurchaseCents ?? 0)} min="0" name="purchase_yuan" step="0.01" type="number" /></Field>
        <Field label="默认销售价（元）"><Input defaultValue={centsToYuanInput(product?.DefaultSaleCents ?? 0)} min="0" name="sale_yuan" step="0.01" type="number" /></Field>
        <Field label="低库存阈值"><Input defaultValue={product?.LowStockThreshold} min="0" name="low_stock_threshold" type="number" /></Field>
        <Field label={product ? "替换商品图片（可选）" : "商品图片"}><Input accept=".jpg,.jpeg,.png,.webp" name="image" type="file" /></Field>
      </div>
      <Field label="备注"><Textarea defaultValue={product?.Note} name="note" /></Field>
      <div className="flex justify-end gap-2">
        <DialogClose asChild><Button type="button" variant="secondary">取消</Button></DialogClose>
        <Button loading={saving} type="submit">保存商品</Button>
      </div>
    </form>
  );
}

function ProductsTable({ busyAction, onDelete, onEdit, onSetEnabled, products }: {
  busyAction: ProductAction;
  onDelete: (product: Product) => void;
  onEdit: (product: Product) => void;
  onSetEnabled: (product: Product) => void;
  products: Product[];
}) {
  const [preview, setPreview] = useState<Product | null>(null);
  if (products.length === 0) {
    return <EmptyBlock title="还没有商品" />;
  }
  return (
    <>
      <div className="overflow-x-auto rounded-lg border border-[var(--border-subtle)] bg-white">
        <table className="w-full min-w-[1120px] text-left text-sm">
          <thead className="border-b border-[var(--border-subtle)] text-xs text-[var(--text-secondary)]">
            <tr>
              <th className="px-4 py-3 font-medium">商品</th>
              <th className="px-4 py-3 font-medium">编码</th>
              <th className="px-4 py-3 font-medium">进货价</th>
              <th className="px-4 py-3 font-medium">销售价</th>
              <th className="px-4 py-3 font-medium">低库存</th>
              <th className="px-4 py-3 font-medium">状态</th>
              <th className="px-4 py-3 font-medium">操作</th>
            </tr>
          </thead>
          <tbody>
            {products.map((product) => (
              <tr className="border-b border-[var(--border-subtle)] last:border-0 hover:bg-black/[0.02]" key={product.ID}>
                <td className="px-4 py-3">
                  <div className="flex items-center gap-3">
                    <ProductThumb product={product} onPreview={() => setPreview(product)} />
                    <div className="min-w-0">
                      <div className="truncate font-medium">{product.Name}</div>
                      <div className="truncate text-xs text-[var(--text-secondary)]">{product.Note || "无备注"}</div>
                    </div>
                  </div>
                </td>
                <td className="px-4 py-3 font-mono text-xs">{product.Code}</td>
                <td className="px-4 py-3">{formatMoney(product.DefaultPurchaseCents)}</td>
                <td className="px-4 py-3">{formatMoney(product.DefaultSaleCents)}</td>
                <td className="px-4 py-3">{formatQuantity(product.LowStockThreshold)}</td>
                <td className="px-4 py-3"><Badge tone={product.Enabled ? "success" : "error"}>{product.Enabled ? "启用" : "禁用"}</Badge></td>
                <td className="px-4 py-3">
                  <div className="flex items-center gap-1">
                    <Button disabled={busyAction !== null} size="sm" type="button" variant="secondary" onClick={() => onEdit(product)}>
                      <Pencil className="h-3.5 w-3.5" />修改
                    </Button>
                    <Button
                      disabled={busyAction !== null}
                      loading={busyAction?.productID === product.ID && busyAction.type === "status"}
                      size="sm"
                      type="button"
                      variant="secondary"
                      onClick={() => onSetEnabled(product)}
                    >
                      <Power className="h-3.5 w-3.5" />{product.Enabled ? "禁用" : "启用"}
                    </Button>
                    <Button
                      className="text-[var(--status-error)] hover:text-[var(--status-error)]"
                      disabled={busyAction !== null}
                      loading={busyAction?.productID === product.ID && busyAction.type === "delete"}
                      size="sm"
                      type="button"
                      variant="ghost"
                      onClick={() => onDelete(product)}
                    >
                      <Trash2 className="h-3.5 w-3.5" />删除
                    </Button>
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <Dialog open={preview !== null} onOpenChange={(openValue) => !openValue && setPreview(null)}>
        <DialogContent title={preview ? preview.Name : "商品图片"}>
          {preview?.ImagePath ? (
            <div className="overflow-hidden rounded-lg border border-[var(--border-subtle)] bg-black/[0.03]">
              <Image alt={`${preview.Name} 图片预览`} className="max-h-[70dvh] w-full object-contain" height={900} src={preview.ImagePath} unoptimized width={1200} />
            </div>
          ) : null}
        </DialogContent>
      </Dialog>
    </>
  );
}

function ProductThumb({ product, onPreview }: { product: Product; onPreview: () => void }) {
  const baseClass = "grid h-10 w-10 place-items-center overflow-hidden rounded-md border border-[var(--border-subtle)] bg-black/[0.03] text-xs text-[var(--text-muted)]";
  if (!product.ImagePath) {
    return (
      <div className={baseClass} aria-label="无商品图片">
        <ImageIcon className="h-4 w-4" />
      </div>
    );
  }
  return (
    <button className={`${baseClass} transition hover:border-[var(--accent-primary)] focus:outline-none focus:ring-2 focus:ring-[var(--accent-primary)]`} type="button" aria-label={`预览 ${product.Name} 图片`} onClick={onPreview}>
      <Image alt={`${product.Name} 图片`} className="h-full w-full object-cover" height={40} src={product.ImagePath} unoptimized width={40} />
    </button>
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
