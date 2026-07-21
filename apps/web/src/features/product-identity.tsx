"use client";

import { Flex, Tag } from "antd";
import { ProductImage } from "@/features/product-image";
import type { Product } from "@/features/types";

export type ProductIdentityValue = Pick<Product, "Name" | "Code" | "ImagePath" | "ArchivedAt"> &
  Partial<Pick<Product, "Enabled">>;

export function ProductIdentity({
  product,
  preview = false,
  size = 44,
}: {
  product: ProductIdentityValue;
  preview?: boolean;
  size?: number;
}) {
  return (
    <Flex align="center" className="product-cell" gap={12}>
      <ProductImage preview={preview} product={product} size={size} />
      <div className="product-cell-copy">
        <Flex align="center" gap={6} wrap="wrap">
          <span className="product-cell-name">{product.Name}</span>
          {!product.ImagePath ? <Tag color="gold">待补图</Tag> : null}
          {product.ArchivedAt ? <Tag>已归档</Tag> : null}
          {!product.ArchivedAt && product.Enabled === false ? <Tag>已停用</Tag> : null}
        </Flex>
        <div className="product-cell-note mono">{product.Code}</div>
      </div>
    </Flex>
  );
}
