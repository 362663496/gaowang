# HTTP Collection And Mutation Contracts

## Scenario: Paginated Collection Endpoints

### 1. Scope / Trigger

Use this contract for an API collection that backs a management list and can grow beyond one screen.

### 2. Signatures

- Request: `GET /api/v1/<collection>?page=<int>&page_size=<int>`
- Internal full-data request: `GET /api/v1/<collection>?all=true`
- Shared implementation: `paginate(c *gin.Context, query *gorm.DB) (*gorm.DB, paginationMeta, error)`

### 3. Contracts

Responses keep `items` and add:

```json
{"pagination":{"page":1,"page_size":20,"total":42,"total_pages":3}}
```

- Defaults are page `1` and page size `20`; maximum page size is `100`.
- Apply every filter before both `Count` and the paginated data query.
- Clamp a page beyond the result set to the last page. Empty results use `total_pages: 0`.
- `all=true` bypasses offset/limit but preserves the same response shape. Use it only for bounded option, summary, dashboard, or report consumers—not real list pages.
- Frontend collection types use `Paginated<T>` and reset `page` to `1` when filters change.

### 4. Validation & Error Matrix

| Condition | Result |
| --- | --- |
| Missing, non-numeric, or non-positive `page` | Use `1` |
| Missing, non-numeric, or non-positive `page_size` | Use `20` |
| `page_size > 100` | Clamp to `100` |
| Page exceeds non-empty result set | Return last page |
| Count fails | `500 INTERNAL` |
| Data query fails | `500 INTERNAL` |

### 5. Good / Base / Bad Cases

- Good: `/products?page=2&page_size=20&q=tea` counts and returns only products matching `q=tea`.
- Base: `/shops` returns the first 20 rows and pagination metadata.
- Bad: a form loads `/products` without `all=true` and silently exposes only the first page as choices.

### 6. Tests Required

- Assert default/custom page sizes, total, total pages, and last-page clamping.
- Assert `all=true` returns every filtered row without a limit.
- Assert at least one filtered handler produces matching count and items.
- Assert frontend list pages render total/page controls and full-data consumers explicitly request `all=true`.

### 7. Wrong vs Correct

GORM statements retain clauses selected by aggregate operations. Clone the session before both count and list queries.

```go
// Wrong: Count/Select state can leak into Find.
query.Count(&total)
query.Find(&items)

// Correct: preserve the filtered base while isolating statements.
query.Session(&gorm.Session{}).Count(&total)
query.Session(&gorm.Session{}).Offset(offset).Limit(size).Find(&items)
```

## Scenario: Multipart Product Update With Optional Image

### 1. Scope / Trigger

Use this contract when editing an active product and optionally replacing its uploaded image.

### 2. Signatures

- Request: `PATCH /api/v1/products/:id`, content type `multipart/form-data`
- Fields: `name`, `code`, `default_purchase_cents`, `default_sale_cents`, `low_stock_threshold`, `note`, optional `image`
- Response: `200 {"item": Product}`

### 3. Contracts

- Only an unarchived product can be updated; `Enabled` and `ArchivedAt` are not editable here.
- With no `image`, preserve `ImagePath`.
- With a new image, save it first; if the database update fails, remove the new file. After a successful update, remove the old file best-effort.
- Record `product.update` with the product resource ID.
- The frontend reuses the create form, sends `FormData`, and displays the server error envelope message inside the dialog.

### 4. Validation & Error Matrix

| Condition | Result |
| --- | --- |
| Invalid ID | `400 VALIDATION` |
| Missing or archived product | `404 PRODUCT_NOT_FOUND` |
| Missing required fields / invalid integer fields | `400 VALIDATION` |
| Invalid image | `400 UPLOAD_INVALID` |
| Duplicate code or invalid update | `400 PRODUCT_UPDATE_FAILED` |
| Success | `200`, updated item, `product.update` audit |

### 5. Good / Base / Bad Cases

- Good: replace the image, persist all fields, then remove only the previous file.
- Base: edit text and prices without an image; the prior image remains readable.
- Bad: delete the old image before the database accepts the replacement.

### 6. Tests Required

- Assert every editable field changes and uneditable status fields remain unchanged.
- Assert no-file update preserves the old path and file.
- Assert replacement returns the new path, removes the old file, and records the audit.
- Assert failed updates return the stable error envelope and do not leak a newly uploaded file.

### 7. Wrong vs Correct

```go
// Wrong: destroys the current image before persistence succeeds.
removeProductImage(uploadDir, current.ImagePath)
db.Model(&current).Updates(updates)

// Correct: persist the new path, clean it on any unsuccessful write, then retire the old file.
result := db.Model(&current).Updates(updates)
if result.Error != nil || result.RowsAffected == 0 {
    removeProductImage(uploadDir, newPath)
    return
}
removeProductImage(uploadDir, current.ImagePath)
```
