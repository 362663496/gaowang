"use client";

import { PictureOutlined } from "@ant-design/icons";
import { Avatar, Image } from "antd";
import { useEffect, useState } from "react";
import type { Product } from "@/features/types";

export function ProductImage({ product, preview = false, size = 40 }: {
  product: Pick<Product, "Name" | "ImagePath">;
  preview?: boolean;
  size?: number;
}) {
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    setFailed(false);
  }, [product.ImagePath]);

  if (!product.ImagePath || failed) {
    return <Avatar aria-label="无商品图片" icon={<PictureOutlined />} shape="square" size={size} />;
  }

  return (
    <Image
      alt={`${product.Name} 图片`}
      height={size}
      preview={preview ? { mask: "预览" } : false}
      src={product.ImagePath}
      style={{ borderRadius: 6, objectFit: "cover" }}
      width={size}
      onError={() => setFailed(true)}
    />
  );
}
