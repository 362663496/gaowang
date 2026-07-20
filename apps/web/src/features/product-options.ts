import type { Product } from "@/features/types";

export type ProductOption = Pick<Product, "ID" | "Name" | "Code" | "ArchivedAt">;

export function productOptionLabel(product: ProductOption): string {
  return `${product.Name} · ${product.Code}${product.ArchivedAt ? "（已归档）" : ""}`;
}

export function productSearchText(product: ProductOption): string {
  return `${product.Name} ${product.Code}`.toLowerCase();
}

export function matchesProductOption(product: ProductOption, input: string): boolean {
  return productSearchText(product).includes(input.trim().toLowerCase());
}
