# Ant Design Frontend Platform

## 1. Scope / Trigger

- Trigger: any user-facing page, shell, form, table, selector, upload, modal, notification, status, loading, empty, or error change in `apps/web`.
- Ant Design 6 is the single generic UI framework. Project components add inventory-domain behavior or page framing; they do not recreate generic controls.

## 2. Signatures

```tsx
// Root: App Router style extraction plus configured client context.
<AntdRegistry>
  <AppProvider>{children}</AppProvider>
</AntdRegistry>

// Server paging adapter used by six management tables.
tablePagination(meta: PaginationMeta, onChange: (page: number) => void)

// Product-selection entry points share this searchable image-grid Popover.
ProductCombobox(props: {
  products: Product[];
  value?: string;
  onChange?: (id: string) => void;
  placeholder?: string;
  allowClear?: boolean;
  disabled?: boolean;
})

// Direct product displays share current-image-first identity.
ProductIdentity(props: {
  product: Pick<Product, "Name" | "Code" | "ImagePath" | "ArchivedAt"> & Partial<Pick<Product, "Enabled">>;
  preview?: boolean;
  size?: number;
})

// Product thumbnails use the same image/failure fallback.
ProductImage(props: {
  product: Pick<Product, "Name" | "ImagePath">;
  preview?: boolean;
  size?: number;
})
```

## 3. Contracts

- Dependencies are `antd@6.5.1`, `@ant-design/icons@6.3.2`, and `@ant-design/nextjs-registry@1.3.0`; React 19 needs no v5 patch.
- `AppProvider` owns `zh_CN`, the `#5e6ad2` primary token, component tokens, and the contextual `App` API. Do not call static message/modal APIs outside that context.
- Management `Table` pagination maps `current <- pagination.page`, `pageSize <- page_size`, and `total <- total`; `showSizeChanger` is false and page changes refetch the server.
- Product selection uses one `Popover` image grid with 88px images and a name/code search input. Inventory actions receive only enabled, non-archived products; history filtering may include archived products with a visible status.
- `ProductIdentity` is the shared direct-product display for tables, dashboard lists, reports, and confirmations: 40–48px current image first, then name/code and archived/missing-image status.
- Multipart product forms construct `FormData` explicitly. Create requires one `image`; edit omits `image` when no replacement file exists.
- Server errors surface the original `Error.message` in a persistent `Alert`. Success uses contextual `message.success`.
- Product `ImagePath` may reference a missing historical file; `ProductImage` switches to a square Ant Design `Avatar` with visible `待补图` text on empty path or `onError`.

## 4. Validation & Error Matrix

| Condition | Required UI behavior |
| --- | --- |
| Required form value missing | Ant Design `Form.Item.rules` blocks submit with a field message |
| API returns structured 4xx/5xx | Keep current data/form; show the original server message in `Alert` |
| Delete needs confirmation | Use contextual `modal.confirm` with a danger primary button |
| Product selector has no match | Show `没有匹配商品` inside the same Popover |
| Image URL empty or load fails | Render square `Avatar` with visible `待补图` |
| Remote list loading | Keep `Table` mounted with `loading=true` |
| Remote list fails | Show retryable `Alert`; do not silently empty the table |
| Viewport below 992px | Hide `Sider`, expose named menu button, open navigation in `Drawer`, keep tables horizontally scrollable |

## 5. Good / Base / Bad Cases

- Good: `Table<Product>` receives typed columns, `ProductIdentity`, `rowKey="ID"`, `scroll.x`, and `tablePagination(meta, setPage)`.
- Base: a bounded dashboard/report table can set `pagination={false}` because it is not a management collection.
- Bad: a text-only product selector/list item, raw `<table>`, custom next/previous buttons, `window.confirm`, native `datalist`, or a home-grown toast reintroduces inconsistent operation surfaces.

## 6. Tests Required

- Unit: product search matches name and code case-insensitively.
- Unit: API client continues to expose the server error message and redirect auth failures.
- Static gate: lint, strict TypeScript, Vitest, production build, and `npm audit --omit=dev` pass.
- Browser: verify desktop shell, 500px drawer, all routes, server-paged table navigation, image-grid search/selection, form error persistence, delete conflict persistence, and image-load fallback.

## 7. Wrong vs Correct

### Wrong

```tsx
<Select options={products.map((product) => ({ value: product.ID, label: product.Name }))} />
return <table>{rows.map(/* text-only product cells */)}</table>;
```

### Correct

```tsx
<ProductCombobox products={products} value={productID} onChange={setProductID} />
return <Table columns={[{ title: "商品", render: (_, row) => <ProductIdentity product={row.Product} /> }]} />;
```
