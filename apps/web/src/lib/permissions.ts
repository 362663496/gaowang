export type PermissionCatalogItem = {
  key: string;
  module: string;
  module_label: string;
  action_label: string;
  staff_assignable: boolean;
  requires: string[];
};

/** Grant a key and recursively add its dependencies. */
export function grantWithDependencies(selected: Iterable<string>, key: string, catalog: PermissionCatalogItem[]): string[] {
  const byKey = indexCatalog(catalog);
  const next = new Set(selected);
  addClosure(next, key, byKey);
  return sorted(next);
}

/** Revoke a key and every permission that transitively depends on it. */
export function revokeWithDependents(selected: Iterable<string>, key: string, catalog: PermissionCatalogItem[]): string[] {
  const byKey = indexCatalog(catalog);
  const next = new Set(selected);
  const dependents = reverseDependents(byKey);
  removeCascade(next, key, dependents);
  return sorted(next);
}

export function hasPermission(permissions: Iterable<string> | undefined, key: string): boolean {
  if (!permissions) return false;
  if (permissions instanceof Set) return permissions.has(key);
  return Array.from(permissions).includes(key);
}

export function anyPermission(permissions: Iterable<string> | undefined, keys: string[]): boolean {
  return keys.some((key) => hasPermission(permissions, key));
}

function indexCatalog(catalog: PermissionCatalogItem[]): Map<string, PermissionCatalogItem> {
  return new Map(catalog.map((item) => [item.key, item]));
}

function addClosure(set: Set<string>, key: string, byKey: Map<string, PermissionCatalogItem>): void {
  if (set.has(key)) return;
  const item = byKey.get(key);
  if (!item) return;
  set.add(key);
  for (const req of item.requires ?? []) {
    addClosure(set, req, byKey);
  }
}

function reverseDependents(byKey: Map<string, PermissionCatalogItem>): Map<string, string[]> {
  const dependents = new Map<string, string[]>();
  for (const item of byKey.values()) {
    for (const req of item.requires ?? []) {
      const list = dependents.get(req) ?? [];
      list.push(item.key);
      dependents.set(req, list);
    }
  }
  return dependents;
}

function removeCascade(set: Set<string>, key: string, dependents: Map<string, string[]>): void {
  if (!set.has(key)) return;
  set.delete(key);
  for (const child of dependents.get(key) ?? []) {
    removeCascade(set, child, dependents);
  }
}

function sorted(set: Set<string>): string[] {
  return Array.from(set).sort();
}
