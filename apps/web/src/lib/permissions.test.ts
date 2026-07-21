import { describe, expect, it } from "vitest";
import {
  grantWithDependencies,
  hasPermission,
  revokeWithDependents,
  type PermissionCatalogItem,
} from "./permissions";

const catalog: PermissionCatalogItem[] = [
  { key: "product.read", module: "product", module_label: "商品", action_label: "查看", staff_assignable: true, requires: [] },
  { key: "product.create", module: "product", module_label: "商品", action_label: "新增", staff_assignable: true, requires: ["product.read"] },
  { key: "product.delete", module: "product", module_label: "商品", action_label: "删除", staff_assignable: true, requires: ["product.read"] },
  { key: "shop.read", module: "shop", module_label: "店铺", action_label: "查看", staff_assignable: true, requires: [] },
  {
    key: "inventory.inbound",
    module: "inventory",
    module_label: "库存",
    action_label: "入库",
    staff_assignable: true,
    requires: ["inventory.read", "shop.read"],
  },
  {
    key: "inventory.read",
    module: "inventory",
    module_label: "库存",
    action_label: "查看",
    staff_assignable: true,
    requires: ["product.read"],
  },
];

describe("permission helpers", () => {
  it("grants transitive dependencies", () => {
    expect(grantWithDependencies([], "inventory.inbound", catalog)).toEqual([
      "inventory.inbound",
      "inventory.read",
      "product.read",
      "shop.read",
    ]);
  });

  it("revokes dependents when a base permission is removed", () => {
    const selected = grantWithDependencies([], "inventory.inbound", catalog);
    expect(revokeWithDependents(selected, "product.read", catalog)).toEqual(["shop.read"]);
  });

  it("checks membership", () => {
    expect(hasPermission(["product.read"], "product.read")).toBe(true);
    expect(hasPermission(["product.read"], "product.delete")).toBe(false);
  });
});
