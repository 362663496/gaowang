"use client";

import { FormEvent, useEffect, useMemo, useState } from "react";
import { ArrowDownToLine, ArrowUpFromLine, SlidersHorizontal } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Dialog, DialogClose, DialogContent, DialogTrigger } from "@/components/ui/dialog";
import { Field, Input, Select, Textarea } from "@/components/ui/fields";
import { ErrorBlock } from "@/components/ui/state";
import { ProductCombobox } from "@/features/product-combobox";
import type { InventorySnapshot, Product, Shop } from "@/features/types";
import { apiPost } from "@/lib/api";
import { centsToYuanInput, formatQuantity, yuanToCents } from "@/lib/format";

type Props = {
  products: Product[];
  shops: Shop[];
  inventory: InventorySnapshot[];
  onDone: (message: string) => void;
};

export function InventoryActions({ products, shops, inventory, onDone }: Props) {
  return (
    <>
      <InboundForm products={products} shops={shops} onDone={onDone} />
      <OutboundForm inventory={inventory} products={products} shops={shops} onDone={onDone} />
      <AdjustmentForm inventory={inventory} products={products} onDone={onDone} />
    </>
  );
}

function InboundForm({ products, shops, onDone }: Pick<Props, "products" | "shops" | "onDone">) {
  const [open, setOpen] = useState(false);
  const [productID, setProductID] = useState("");
  const [unitYuan, setUnitYuan] = useState("0.00");
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);
  const product = products.find((item) => item.ID === productID);

  useEffect(() => {
    setUnitYuan(centsToYuanInput(product?.DefaultPurchaseCents ?? 0));
  }, [product]);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!productID) {
      setError("请选择有效商品");
      return;
    }
    setSaving(true);
    setError("");
    const form = new FormData(event.currentTarget);
    try {
      await apiPost("/inventory/inbound", {
        product_id: productID,
        shop_id: String(form.get("shop_id") ?? ""),
        quantity: Number(form.get("quantity")),
        unit_cents: yuanToCents(String(form.get("unit_yuan") ?? "")),
      });
      setOpen(false);
      onDone("入库已记录");
    } catch (err) {
      setError(err instanceof Error ? err.message : "入库失败");
    } finally {
      setSaving(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild><Button><ArrowDownToLine className="h-4 w-4" />入库</Button></DialogTrigger>
      <DialogContent title="新建入库">
        <form className="grid gap-4" onSubmit={submit}>
          {error ? <ErrorBlock message={error} /> : null}
          <ProductSelect products={products} value={productID} onChange={setProductID} />
          <Field label="店铺（可选）">
            <Select defaultValue="" name="shop_id">
              <option value="">不选择店铺</option>
              {shops.map((shop) => <option key={shop.ID} value={shop.ID}>{shop.Name}</option>)}
            </Select>
          </Field>
          <div className="grid gap-3 sm:grid-cols-2">
            <Field label="数量"><Input min="1" name="quantity" required type="number" /></Field>
            <Field label="进货单价（元）"><Input min="0" name="unit_yuan" required step="0.01" type="number" value={unitYuan} onChange={(event) => setUnitYuan(event.target.value)} /></Field>
          </div>
          <Actions saving={saving} label="保存入库" />
        </form>
      </DialogContent>
    </Dialog>
  );
}

function OutboundForm({ products, shops, inventory, onDone }: Props) {
  const [open, setOpen] = useState(false);
  const [productID, setProductID] = useState("");
  const [saleYuan, setSaleYuan] = useState("0.00");
  const [quantity, setQuantity] = useState(1);
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);
  const product = products.find((item) => item.ID === productID);
  const stock = inventory.find((item) => item.ProductID === productID)?.Quantity ?? 0;
  const shortage = quantity > stock;

  useEffect(() => {
    setSaleYuan(centsToYuanInput(product?.DefaultSaleCents ?? 0));
  }, [product]);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!productID) {
      setError("请选择有效商品");
      return;
    }
    setSaving(true);
    setError("");
    const form = new FormData(event.currentTarget);
    try {
      await apiPost("/inventory/sales-outbound", {
        product_id: productID,
        shop_id: String(form.get("shop_id") ?? ""),
        quantity,
        sale_unit_cents: yuanToCents(String(form.get("sale_yuan") ?? "")),
      });
      setOpen(false);
      onDone("销售出库已记录");
    } catch (err) {
      setError(err instanceof Error ? err.message : "出库失败");
    } finally {
      setSaving(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild><Button variant="secondary"><ArrowUpFromLine className="h-4 w-4" />销售出库</Button></DialogTrigger>
      <DialogContent title="销售出库">
        <form className="grid gap-4" onSubmit={submit}>
          {error ? <ErrorBlock message={error} /> : null}
          <ProductSelect products={products} value={productID} onChange={setProductID} />
          <Field label="店铺">
            <Select name="shop_id" required>
              {shops.map((shop) => <option key={shop.ID} value={shop.ID}>{shop.Name}</option>)}
            </Select>
          </Field>
          <div className="grid gap-3 sm:grid-cols-2">
            <Field label={`数量（当前 ${formatQuantity(stock)}）`}>
              <Input min="1" name="quantity" required type="number" value={quantity} onChange={(event) => setQuantity(Number(event.target.value))} />
            </Field>
            <Field label="销售单价（元）"><Input min="0" name="sale_yuan" required step="0.01" type="number" value={saleYuan} onChange={(event) => setSaleYuan(event.target.value)} /></Field>
          </div>
          {shortage ? <div className="rounded-md bg-amber-50 px-3 py-2 text-sm text-amber-800">当前库存不足，提交会被后端拒绝。</div> : null}
          <Actions saving={saving} label="保存出库" />
        </form>
      </DialogContent>
    </Dialog>
  );
}

function AdjustmentForm({ products, onDone }: Pick<Props, "products" | "inventory" | "onDone">) {
  const [open, setOpen] = useState(false);
  const [productID, setProductID] = useState("");
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!productID) {
      setError("请选择有效商品");
      return;
    }
    setSaving(true);
    setError("");
    const form = new FormData(event.currentTarget);
    try {
      await apiPost("/inventory/adjustments", {
        product_id: productID,
        quantity_delta: Number(form.get("quantity_delta")),
        reason: String(form.get("reason") ?? ""),
      });
      setOpen(false);
      onDone("库存调整已记录");
    } catch (err) {
      setError(err instanceof Error ? err.message : "调整失败");
    } finally {
      setSaving(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild><Button variant="secondary"><SlidersHorizontal className="h-4 w-4" />调整</Button></DialogTrigger>
      <DialogContent title="库存调整">
        <form className="grid gap-4" onSubmit={submit}>
          {error ? <ErrorBlock message={error} /> : null}
          <ProductSelect products={products} value={productID} onChange={setProductID} />
          <Field label="调整数量"><Input name="quantity_delta" required type="number" placeholder="正数增加，负数减少" /></Field>
          <Field label="原因"><Textarea maxLength={500} name="reason" required /></Field>
          <Actions saving={saving} label="保存调整" />
        </form>
      </DialogContent>
    </Dialog>
  );
}

function ProductSelect({ products, value, onChange }: { products: Product[]; value: string; onChange: (id: string) => void }) {
  const enabledProducts = useMemo(() => products.filter((product) => product.Enabled && !product.ArchivedAt), [products]);
  return <ProductCombobox products={enabledProducts} required value={value} onChange={onChange} />;
}

function Actions({ saving, label }: { saving: boolean; label: string }) {
  return (
    <div className="flex justify-end gap-2">
      <DialogClose asChild><Button type="button" variant="secondary">取消</Button></DialogClose>
      <Button loading={saving} type="submit">{label}</Button>
    </div>
  );
}
