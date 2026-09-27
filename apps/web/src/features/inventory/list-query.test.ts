import { describe, expect, it } from "vitest";
import { inventoryListQuery, inventoryQuantityRangeError } from "./list-query";

const filters = {
  query: " 茶 ",
  lowStock: false,
  minQuantity: null,
  maxQuantity: null,
  sort: "name" as const,
  order: "asc" as const,
};

describe("inventoryListQuery", () => {
  it("omits empty bounds and trims the keyword", () => {
    expect(inventoryListQuery({ ...filters, page: 2 })).toBe("page=2&q=%E8%8C%B6&sort=name&order=asc");
  });

  it("includes quantity bounds, low stock, and the selected sort", () => {
    expect(inventoryListQuery({
      ...filters,
      query: "",
      lowStock: true,
      minQuantity: 0,
      maxQuantity: 8,
      sort: "quantity",
      order: "desc",
    })).toBe("low_stock=true&min_quantity=0&max_quantity=8&sort=quantity&order=desc");
  });
});

describe("inventoryQuantityRangeError", () => {
  it("rejects an inverted range and allows open bounds", () => {
    expect(inventoryQuantityRangeError(5, 1)).toBe("数量下限不能大于上限");
    expect(inventoryQuantityRangeError(0, null)).toBe("");
    expect(inventoryQuantityRangeError(null, 3)).toBe("");
  });
});
