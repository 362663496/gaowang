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

// Four product-selection entry points share this searchable Select.
ProductCombobox(props: {
  products: Product[];
  value?: string;
  onChange?: (id: string) => void;
  placeholder?: string;
  allowClear?: boolean;
  disabled?: boolean;
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
- Product selection uses one `Select` with `showSearch`; each option searches a lower-case `"Name Code"` string. Inventory actions receive only enabled, non-archived products; history filtering may include archived products with a visible suffix.
- Multipart product forms construct `FormData` explicitly and omit `image` when no replacement file exists.
- Server errors surface the original `Error.message` in a persistent `Alert`. Success uses contextual `message.success`.
- Product `ImagePath` may reference a missing historical file; `ProductImage` must switch to an Ant Design `Avatar` placeholder on `onError` so broken alt text cannot expand a table cell.

## 4. Validation & Error Matrix

| Condition | Required UI behavior |
| --- | --- |
| Required form value missing | Ant Design `Form.Item.rules` blocks submit with a field message |
| API returns structured 4xx/5xx | Keep current data/form; show the original server message in `Alert` |
| Delete needs confirmation | Use contextual `modal.confirm` with a danger primary button |
| Product selector has no match | Show `没有匹配商品`; never add a side search input |
| Image URL empty or load fails | Render square `Avatar` plus `PictureOutlined` |
| Remote list loading | Keep `Table` mounted with `loading=true` |
| Remote list fails | Show retryable `Alert`; do not silently empty the table |
| Viewport below 992px | Hide `Sider`, expose named menu button, open navigation in `Drawer`, keep tables horizontally scrollable |

## 5. Good / Base / Bad Cases

- Good: `Table<Product>` receives typed columns, `rowKey="ID"`, `scroll.x`, and `tablePagination(meta, setPage)`.
- Base: a bounded dashboard/report table can set `pagination={false}` because it is not a management collection.
- Bad: raw `<table>`, custom next/previous buttons, `window.confirm`, native `datalist`, or a home-grown toast reintroduces a second UI system.

## 6. Tests Required

- Unit: product option label/search matches name and code case-insensitively.
- Unit: API client continues to expose the server error message and redirect auth failures.
- Static gate: lint, strict TypeScript, Vitest, production build, and `npm audit --omit=dev` pass.
- Browser: verify desktop shell, 500px drawer, all routes, server-paged table navigation, product search `Select`, form error persistence, delete conflict persistence, and image-load fallback.

## 7. Wrong vs Correct

### Wrong

```tsx
if (window.confirm("删除？")) await request(`/products/${id}`, { method: "DELETE" });
return <table>{rows.map(/* custom cells and custom paging */)}</table>;
```

### Correct

```tsx
const { modal } = App.useApp();
modal.confirm({
  title: "确认删除？",
  okButtonProps: { danger: true },
  onOk: () => request(`/products/${id}`, { method: "DELETE" }),
});

return <Table dataSource={rows} pagination={tablePagination(meta, setPage)} scroll={{ x: 960 }} />;
```
