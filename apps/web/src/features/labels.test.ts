import { describe, expect, it } from "vitest";
import { stockStatus } from "./labels";

describe("stockStatus", () => {
  it.each([
    { quantity: 0, threshold: 5, label: "无库存", tone: "error" },
    { quantity: 3, threshold: 5, label: "低库存", tone: "warning" },
    { quantity: 6, threshold: 5, label: "正常", tone: "success" },
    { quantity: 1, threshold: 0, label: "正常", tone: "success" },
  ])("maps $quantity/$threshold to $label", ({ quantity, threshold, label, tone }) => {
    expect(stockStatus(quantity, threshold)).toMatchObject({ label, tone });
  });
});
