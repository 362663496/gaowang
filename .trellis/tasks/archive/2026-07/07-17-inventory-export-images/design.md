# Design: 库存页面导出含图片

## Overview

Add one protected API endpoint that builds an `.xlsx` workbook from the authoritative inventory query, then add one inventory-page button that downloads the returned blob.

```text
库存页导出按钮
  → GET /api/v1/inventory/export[?low_stock=true]
  → 查询当前筛选下全部库存（无分页）
  → 读取并缩放商品图片
  → 生成含嵌入图片的 XLSX
  → 浏览器下载 inventory-YYYY-MM-DD.xlsx
```

## Backend

### Query ownership

Extract the existing active-inventory base query in `inventory.go` so both list and export paths use the same archived-product exclusion and low-stock predicate. The existing paginated JSON response remains unchanged; export skips pagination and preserves the same updated-time ordering.

### Endpoint contract

- Route: `GET /api/v1/inventory/export`
- Authorization: existing protected inventory access; both admin and staff may export.
- Query: optional `low_stock=true` with the same meaning as the page filter.
- Success: `200`, XLSX content type, attachment response.
- Database/workbook failure: existing JSON error contract with a `500` response.
- Empty result: valid workbook containing the header row only.

### Workbook

- Worksheet: `当前库存`.
- Columns: 图片、商品名称、商品编码、数量、移动平均成本、库存金额、库存状态、更新时间.
- Quantity and money cells remain numeric; money uses a yuan number format.
- Status uses the page semantics: `无库存`, `低库存`, `正常`.
- Header styling, practical column widths, and taller image rows make the file immediately usable without adding a broader reporting system.

### Images

- Resolve only `filepath.Base(Product.ImagePath)` under `UploadDir`; never read arbitrary paths from the database value.
- Decode JPG, PNG, or WebP, resize proportionally to a bounded thumbnail, encode as PNG, and embed it in the image cell.
- Missing, unreadable, or corrupt images produce `无图片` in that row and do not abort the export.
- Workbook generation stays in memory for this lightweight inventory app.

## Frontend

- Add `apiDownload(path)` beside the existing API helpers, sharing the same credentials, development headers, authentication redirect, and structured error handling.
- Add a secondary `导出 Excel` button with loading and disabled states to the existing page-header actions.
- Pass `low_stock=true` only while the low-stock filter is active; page number is never sent.
- Convert the returned blob to an object URL, trigger a native download, and always revoke the URL.
- Show export failures as an error message without replacing the loaded inventory table.

## Compatibility and Rollback

- No database or JSON response changes.
- Existing inventory list callers are protected by a shared query and regression tests.
- Removing the route, button, helper, and Excel dependencies fully rolls back the feature.

