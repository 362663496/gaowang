import { describe, expect, it } from "vitest";
import { matchesProductOption, productOptionLabel, productSearchText } from "./product-options";

const product = { ID: "1", Name: "Green Tea", Code: "TEA-001", ArchivedAt: null };

describe("product select options", () => {
  it("builds labels and searches by name or code", () => {
    expect(productOptionLabel(product)).toBe("Green Tea · TEA-001");
    expect(productSearchText(product)).toBe("green tea tea-001");
    expect(matchesProductOption(product, "green")).toBe(true);
    expect(matchesProductOption(product, "TEA-001")).toBe(true);
    expect(matchesProductOption(product, "coffee")).toBe(false);
  });
});
