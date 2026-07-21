"use client";

import { CheckOutlined, DownOutlined, SearchOutlined } from "@ant-design/icons";
import { Button, Empty, Flex, Input, Popover, Tag, Typography } from "antd";
import { useMemo, useState } from "react";
import { ProductIdentity } from "@/features/product-identity";
import { ProductImage } from "@/features/product-image";
import { matchesProductOption } from "@/features/product-options";
import type { Product } from "@/features/types";

export function ProductCombobox({
  products,
  value = "",
  onChange = () => undefined,
  placeholder = "按图片选择商品",
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
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const selected = products.find((product) => product.ID === value);
  const visible = useMemo(
    () => products.filter((product) => matchesProductOption(product, query)),
    [products, query],
  );

  const content = (
    <Flex className="product-picker" gap={12} vertical>
      <Input
        allowClear
        aria-label="搜索商品名称或编码"
        autoFocus
        placeholder="搜索名称或编码"
        prefix={<SearchOutlined />}
        value={query}
        onChange={(event) => setQuery(event.target.value)}
      />
      {visible.length === 0 ? (
        <Empty description="没有匹配商品" image={Empty.PRESENTED_IMAGE_SIMPLE} />
      ) : (
        <div aria-label="商品图片选择" className="product-picker-grid" role="grid">
          {visible.map((product) => {
            const active = product.ID === value;
            return (
              <div key={product.ID} role="gridcell">
                <Button
                  aria-label={`选择商品 ${product.Name} ${product.Code}`}
                  aria-pressed={active}
                  className={`product-picker-option${active ? " product-picker-option-selected" : ""}`}
                  onClick={() => {
                    onChange(product.ID);
                    setOpen(false);
                    setQuery("");
                  }}
                >
                  <ProductImage product={product} size={88} />
                  <Typography.Text ellipsis strong>{product.Name}</Typography.Text>
                  <Typography.Text className="mono" ellipsis type="secondary">{product.Code}</Typography.Text>
                  {product.ArchivedAt ? <Tag>已归档</Tag> : !product.Enabled ? <Tag>已停用</Tag> : null}
                  {active ? <span className="product-picker-selected"><CheckOutlined /> 已选择</span> : null}
                </Button>
              </div>
            );
          })}
        </div>
      )}
      {allowClear && value ? (
        <Button
          onClick={() => {
            onChange("");
            setOpen(false);
            setQuery("");
          }}
        >
          清空选择
        </Button>
      ) : null}
    </Flex>
  );

  return (
    <Popover
      content={content}
      open={open}
      placement="bottomLeft"
      trigger="click"
      onOpenChange={(next) => {
        if (!disabled) setOpen(next);
        if (!next) setQuery("");
      }}
    >
      <Button
        aria-expanded={open}
        aria-haspopup="grid"
        block
        className="product-picker-trigger"
        disabled={disabled}
      >
        {selected ? <ProductIdentity product={selected} size={40} /> : <span className="muted">{placeholder}</span>}
        <DownOutlined />
      </Button>
    </Popover>
  );
}
