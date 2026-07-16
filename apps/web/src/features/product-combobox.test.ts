import { describe, expect, it } from "vitest";
import { findProductByInput, productOptionLabel } from "./product-options";

const product = { ID: "1", Name: "Green Tea", Code: "TEA-001", ArchivedAt: null };

describe("product combobox", () => {
  it("resolves a selected product by label, name, or code", () => {
    const products = [product];
    expect(findProductByInput(products, productOptionLabel(product))?.ID).toBe("1");
    expect(findProductByInput(products, "green tea")?.ID).toBe("1");
    expect(findProductByInput(products, "tea-001")?.ID).toBe("1");
    expect(findProductByInput(products, "tea")?.ID).toBeUndefined();
  });
});
