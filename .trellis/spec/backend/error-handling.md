# Error Handling

## API Contract

All API failures use the envelope produced by `internal/http/handlers/writeError`:

```json
{"error":{"code":"VALIDATION","message":"..."}}
```

Use a stable machine-readable uppercase code and a concise user-facing message. Success collections use `{"items": [...]}`; named results use keys such as `item`, `summary`, `settings`, or `job`.

## Boundary Validation

- JSON handlers define request structs with Gin `binding` tags and call `bindJSON`.
- Parse identifiers at the boundary with `parseUUID` before calling a service.
- Multipart endpoints validate required text and numeric form fields in a focused parser such as `productFromForm`/`formInt`.
- Use pointer fields when zero is valid but omission is not. `inboundRequest.UnitCents` and `outboundRequest.SaleUnitCents` preserve that distinction.
- Keep compatibility decoding localized to the request type, as `outboundRequest.UnmarshalJSON` does for the legacy camel-case price field.

## Service Errors

- Return errors; do not panic for expected failures.
- Wrap operational failures with an action and `%w`, following `db.Open`, `db.Migrate`, password hashing, backup, and upload services.
- Use a sentinel only when callers need category-based handling. `services.ErrInsufficientStock` is checked with `errors.Is` and mapped to HTTP 409.
- Validation that belongs to the business operation stays in the service so every caller gets it, as in outbound and adjustment validation.

## HTTP Mapping

- `400 VALIDATION` for malformed or missing input.
- `401 UNAUTHORIZED` or `INVALID_CREDENTIALS` for authentication failures.
- `403 FORBIDDEN` for role failures.
- `409 INSUFFICIENT_STOCK` for stock conflicts.
- `500 INTERNAL` or a feature-specific failure code for unexpected operational failures.
- Return `204` for successful operations with no response body, as in password change.

Middleware must use the same envelope and abort after writing a terminal response (`internal/http/middleware.go`).

## Sensitive Failures

Do not expose passwords, auth secrets, SMTP credentials, or the database URL in response messages or audit metadata. Existing CRUD handlers sometimes surface a raw GORM error as a 400 response; do not copy that pattern to new security-sensitive or internal failures—prefer a stable public message while retaining wrapped context for process-level logs.

## Scenario: Product archive and inventory mutations

### 1. Scope / Trigger

Use this contract when listing, changing, deleting, or posting inventory for a product. These operations cross the HTTP, GORM, audit, upload-file, reporting, and frontend boundaries.

### 2. Signatures

- `GET /api/v1/products[?include_archived=true]`
- `GET /api/v1/inventory`
- `PATCH /api/v1/products/:id/enabled`
- `DELETE /api/v1/products/:id`
- `POST /api/v1/inventory/inbound|sales-outbound|adjustment`
- Database fields: nullable `products.archived_at`; nullable `stock_movements.shop_id`; restrictive product foreign keys from snapshots and movements.

### 3. Contracts

- PATCH request: `{"enabled": <boolean>}`; use `*bool` in the request struct so explicit `false` differs from omission.
- PATCH success: `200 {"item": Product}` and audit action `product.enable` or `product.disable`.
- `GET /products` excludes rows with `archived_at`; `include_archived=true` includes them for historical filters. `GET /inventory` always excludes archived products.
- DELETE success is always `204`. A never-used product is hard-deleted, audited as `product.delete`, and its image is removed best-effort using `filepath.Base(ImagePath)`. A used zero-stock product is kept with `ArchivedAt != nil` and `Enabled=false`, audited as `product.archive`, and retains its image and history.
- Historical movement and sales-report queries continue to include archived products; product-ranking rows expose `archived: boolean`.
- Inbound accepts optional `shop_id`. Omission or `""` stores `NULL`; a valid UUID is copied only to the movement and does not create per-shop inventory.

### 4. Validation & Error Matrix

| Condition | Response |
| --- | --- |
| Invalid UUID, invalid non-empty `shop_id`, or missing/non-boolean `enabled` | `400 VALIDATION` |
| Product does not exist or is already archived for lifecycle mutation | `404 PRODUCT_NOT_FOUND` |
| Product current quantity is not zero during DELETE | `409 PRODUCT_HAS_STOCK` |
| Inventory write targets an archived product | `409 PRODUCT_ARCHIVED` |
| Omitted or empty inbound `shop_id` | accepted; movement `shop_id` is `NULL` |
| Lookup/update/archive/delete query fails | `500` with a stable product-specific code |

### 5. Good / Base / Bad Cases

- Good: `{"enabled": false}` persists `false`, returns the updated product, and records `product.disable`.
- Good: a used zero-stock product archives with `204`; operational lists hide it while history, image, and financial aggregates remain.
- Base: a never-used product hard-deletes with `204`; its database row and private upload file disappear. Inbound without `shop_id` remains valid.
- Bad: a nonzero-stock product returns `409 PRODUCT_HAS_STOCK`, and an archived product inventory write returns `409 PRODUCT_ARCHIVED`; neither operation changes stock or movements.

### 6. Tests Required

Use route-level `httptest` tests with SQLite plus inventory-service tests. Assert explicit enable states; hard-delete row/image/audit behavior; zero-stock archive visibility, retained image/history, and audit behavior; nonzero-stock rejection; archived-write rejection with no new snapshot/movement; optional inbound shop persistence; and unchanged historical sales summary/ranking with `archived=true`.

### 7. Wrong vs Correct

Wrong: use `gorm.DeletedAt`, filter archived products from historical reports, or check stock without sharing the same lock order as inventory writes. Preloads lose product details, financial history disappears, or a concurrent write can land after archive.

Correct: use explicit nullable `ArchivedAt`; filter only operational queries; lock product row then snapshot in both delete and inventory transactions; return stable conflict codes; and retain database `RESTRICT` constraints as the hard-delete safety net.

## Tests

Test both status and observable state. Handler tests use `httptest` through `NewRouter`; service tests use `errors.Is` for sentinel errors and verify persisted snapshot/movement values.
