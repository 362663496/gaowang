"use client";

import { useEffect, useId, useMemo, useState } from "react";
import { Field, Input } from "@/components/ui/fields";
import { findProductByInput, productOptionLabel } from "@/features/product-options";
import type { Product } from "@/features/types";

export function ProductCombobox({
  products,
  value,
  onChange,
  label = "商品",
  placeholder = "输入名称或编码选择商品",
  required = false,
}: {
  products: Product[];
  value: string;
  onChange: (id: string) => void;
  label?: string;
  placeholder?: string;
  required?: boolean;
}) {
  const listID = useId();
  const selected = useMemo(() => products.find((product) => product.ID === value), [products, value]);
  const [input, setInput] = useState("");

  useEffect(() => {
    setInput(selected ? productOptionLabel(selected) : "");
  }, [selected]);

  function update(next: string) {
    setInput(next);
    onChange(findProductByInput(products, next)?.ID ?? "");
  }

  function normalize() {
    const match = findProductByInput(products, input);
    setInput(match ? productOptionLabel(match) : selected ? productOptionLabel(selected) : "");
    if (match) onChange(match.ID);
  }

  return (
    <Field label={label}>
      <Input aria-autocomplete="list" list={listID} placeholder={placeholder} required={required} type="search" value={input} onBlur={normalize} onChange={(event) => update(event.target.value)} />
      <datalist id={listID}>
        {products.map((product) => <option key={product.ID} value={productOptionLabel(product)} />)}
      </datalist>
    </Field>
  );
}
