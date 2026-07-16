import type { Product } from "@/features/types";

export type ProductOption = Pick<Product, "ID" | "Name" | "Code" | "ArchivedAt">;

export function productOptionLabel(product: ProductOption): string {
  return `${product.Name} · ${product.Code}${product.ArchivedAt ? "（已归档）" : ""}`;
}

export function findProductByInput(products: ProductOption[], input: string): ProductOption | undefined {
  const value = input.trim().toLowerCase();
  if (!value) return undefined;
  return products.find((product) =>
    productOptionLabel(product).toLowerCase() === value || product.Name.toLowerCase() === value || product.Code.toLowerCase() === value,
  );
}
