"use client";

import { FormEvent, useCallback, useEffect, useState } from "react";
import { Plus } from "lucide-react";
import { PageHeader } from "@/components/layout/page-header";
import { Button } from "@/components/ui/button";
import { Dialog, DialogClose, DialogContent, DialogTrigger } from "@/components/ui/dialog";
import { Field, Input, Textarea } from "@/components/ui/fields";
import { MessageBar } from "@/components/ui/message";
import { initialPagination, Pagination } from "@/components/ui/pagination";
import { EmptyBlock, ErrorBlock, LoadingBlock } from "@/components/ui/state";
import type { Paginated, Shop } from "@/features/types";
import { useMessage } from "@/features/use-message";
import { apiGet, apiPost } from "@/lib/api";
import { formatDateTime, formatQuantity } from "@/lib/format";

export default function ShopsPage() {
  const [shops, setShops] = useState<Shop[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [open, setOpen] = useState(false);
  const [page, setPage] = useState(1);
  const [pagination, setPagination] = useState(initialPagination);
  const { message, show } = useMessage();

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const data = await apiGet<Paginated<Shop>>(`/shops?page=${page}`);
      setShops(data.items);
      setPagination(data.pagination);
      if (data.pagination.page !== page) setPage(data.pagination.page);
    } catch (err) {
      setError(err instanceof Error ? err.message : "加载失败");
    } finally {
      setLoading(false);
    }
  }, [page]);

  useEffect(() => {
    void load();
  }, [load]);

  return (
    <div className="space-y-5">
      <PageHeader
        title="店铺"
        description="店铺用于销售出库归属，不单独持有库存。"
        actions={
          <Dialog open={open} onOpenChange={setOpen}>
            <DialogTrigger asChild><Button><Plus className="h-4 w-4" />新增店铺</Button></DialogTrigger>
            <DialogContent title="新增店铺">
              <ShopForm
                onCreated={() => {
                  setOpen(false);
                  show("店铺已创建");
                  if (page === 1) void load();
                  else setPage(1);
                }}
              />
            </DialogContent>
          </Dialog>
        }
      />
      <div className="rounded-lg border border-[var(--border-subtle)] bg-white p-4">
        <div className="text-xs text-[var(--text-secondary)]">店铺数量</div>
        <div className="mt-2 text-xl font-semibold">{formatQuantity(pagination.total)}</div>
      </div>
      {loading ? <LoadingBlock label="加载店铺" /> : error ? <ErrorBlock message={error} onRetry={load} /> : <ShopsTable shops={shops} />}
      <Pagination meta={pagination} onPageChange={setPage} />
      <MessageBar message={message} />
    </div>
  );
}

function ShopForm({ onCreated }: { onCreated: () => void }) {
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSaving(true);
    setError("");
    const form = new FormData(event.currentTarget);
    try {
      await apiPost<{ item: Shop }>("/shops", {
        name: String(form.get("name") ?? ""),
        note: String(form.get("note") ?? ""),
      });
      onCreated();
    } catch (err) {
      setError(err instanceof Error ? err.message : "保存失败");
    } finally {
      setSaving(false);
    }
  }

  return (
    <form className="grid gap-4" onSubmit={submit}>
      {error ? <ErrorBlock message={error} /> : null}
      <Field label="店铺名称"><Input name="name" required /></Field>
      <Field label="备注"><Textarea name="note" /></Field>
      <div className="flex justify-end gap-2">
        <DialogClose asChild><Button type="button" variant="secondary">取消</Button></DialogClose>
        <Button loading={saving} type="submit">保存店铺</Button>
      </div>
    </form>
  );
}

function ShopsTable({ shops }: { shops: Shop[] }) {
  if (shops.length === 0) {
    return <EmptyBlock title="还没有店铺" />;
  }
  return (
    <div className="overflow-x-auto rounded-lg border border-[var(--border-subtle)] bg-white">
      <table className="w-full min-w-[640px] text-left text-sm">
        <thead className="border-b border-[var(--border-subtle)] text-xs text-[var(--text-secondary)]">
          <tr>
            <th className="px-4 py-3 font-medium">名称</th>
            <th className="px-4 py-3 font-medium">备注</th>
            <th className="px-4 py-3 font-medium">状态</th>
            <th className="px-4 py-3 font-medium">创建时间</th>
          </tr>
        </thead>
        <tbody>
          {shops.map((shop) => (
            <tr className="border-b border-[var(--border-subtle)] last:border-0 hover:bg-black/[0.02]" key={shop.ID}>
              <td className="px-4 py-3 font-medium">{shop.Name}</td>
              <td className="px-4 py-3 text-[var(--text-secondary)]">{shop.Note || "-"}</td>
              <td className="px-4 py-3">{shop.Enabled ? "启用" : "禁用"}</td>
              <td className="px-4 py-3 text-[var(--text-secondary)]">{formatDateTime(shop.CreatedAt)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
