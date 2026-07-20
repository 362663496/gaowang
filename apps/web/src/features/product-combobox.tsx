"use client";

import { Select } from "antd";
import { productOptionLabel, productSearchText } from "@/features/product-options";
import type { Product } from "@/features/types";

export function ProductCombobox({
  products,
  value = "",
  onChange = () => undefined,
  placeholder = "输入名称或编码选择商品",
  allowClear = false,
  disabled = false,
}: {
  products: Product[];
  value?: string;
  onChange?: (id: string) => void;
  placeholder?: string;
  allowClear?: boolean;
  disabled?: boolean;
}) {
  return (
    <Select
      allowClear={allowClear}
      disabled={disabled}
      filterOption={(input, option) => String(option?.search ?? "").includes(input.trim().toLowerCase())}
      notFoundContent="没有匹配商品"
      optionFilterProp="search"
      options={products.map((product) => ({
        value: product.ID,
        label: productOptionLabel(product),
        search: productSearchText(product),
      }))}
      placeholder={placeholder}
      showSearch
      value={value || undefined}
      onChange={(next) => onChange(next ?? "")}
    />
  );
}
