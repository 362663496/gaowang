export type InventorySort = "name" | "quantity" | "code";
export type InventoryOrder = "asc" | "desc";

export type InventoryListFilters = {
  page?: number;
  query: string;
  lowStock: boolean;
  minQuantity: number | null;
  maxQuantity: number | null;
  sort: InventorySort;
  order: InventoryOrder;
};

export function inventoryQuantityRangeError(minQuantity: number | null, maxQuantity: number | null): string {
  if (minQuantity != null && maxQuantity != null && minQuantity > maxQuantity) {
    return "数量下限不能大于上限";
  }
  return "";
}

export function inventoryListQuery(filters: InventoryListFilters): string {
  const params = new URLSearchParams();
  if (filters.page != null) params.set("page", String(filters.page));
  if (filters.lowStock) params.set("low_stock", "true");
  const query = filters.query.trim();
  if (query) params.set("q", query);
  if (filters.minQuantity != null) params.set("min_quantity", String(filters.minQuantity));
  if (filters.maxQuantity != null) params.set("max_quantity", String(filters.maxQuantity));
  params.set("sort", filters.sort);
  params.set("order", filters.order);
  return params.toString();
}
