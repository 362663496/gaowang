import { describe, expect, it } from "vitest";
import { centsToYuanInput, formatDateTime, formatMoney, formatQuantity, yuanToCents } from "./format";

describe("format helpers", () => {
  it("formats integer cents as CNY", () => {
    expect(formatMoney(123456)).toBe("¥1,234.56");
  });

  it("formats quantities without decimals", () => {
    expect(formatQuantity(12345)).toBe("12,345");
  });

  it("formats date time in Chinese locale", () => {
    expect(formatDateTime("2026-07-01T08:30:00+08:00")).toContain("2026");
  });

  it("converts yuan inputs to integer cents", () => {
    expect(yuanToCents("12.35")).toBe(1235);
    expect(yuanToCents("")).toBe(0);
  });

  it("converts cents to yuan form inputs", () => {
    expect(centsToYuanInput(1235)).toBe("12.35");
  });
});
