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

## Scenario: Product lifecycle mutations

### 1. Scope / Trigger

Use this contract when changing a product's enabled state or deleting a product. These mutations cross the HTTP, GORM, audit, upload-file, and frontend boundaries.

### 2. Signatures

- `PATCH /api/v1/products/:id/enabled`
- `DELETE /api/v1/products/:id`
- Database references: `inventory_snapshots.product_id` and `stock_movements.product_id` both restrict product deletion.

### 3. Contracts

- PATCH request: `{"enabled": <boolean>}`; use `*bool` in the request struct so explicit `false` differs from omission.
- PATCH success: `200 {"item": Product}` and audit action `product.enable` or `product.disable`.
- DELETE success: `204`, audit action `product.delete`, then best-effort removal of the product image using `filepath.Base(ImagePath)` inside the configured upload directory.
- Both routes stay in the authenticated product route group; disabling does not change inventory-operation eligibility.

### 4. Validation & Error Matrix

| Condition | Response |
| --- | --- |
| Invalid UUID or missing/non-boolean `enabled` | `400 VALIDATION` |
| Product does not exist | `404 PRODUCT_NOT_FOUND` |
| Inventory snapshot or stock movement references the product | `409 PRODUCT_IN_USE` |
| Lookup/reference/update/delete query fails | `500` with a stable product-specific code |

### 5. Good / Base / Bad Cases

- Good: `{"enabled": false}` persists `false`, returns the updated product, and records `product.disable`.
- Base: a never-used product deletes with `204`; its database row and private upload file disappear.
- Bad: a referenced product returns `409`; the product, inventory snapshot, and movement history remain unchanged.

### 6. Tests Required

Use a route-level `httptest` test with SQLite. Assert explicit `false` and `true` persist, all lifecycle audits exist, an unused product and its image are removed, and a referenced product returns `409 PRODUCT_IN_USE` while remaining stored.

### 7. Wrong vs Correct

Wrong: bind `enabled` to a plain `bool` with `binding:"required"`, or delete before checking references; explicit `false` is rejected and history safety depends on an opaque database error.

Correct: bind to `*bool`, check both reference tables, return the stable `PRODUCT_IN_USE` conflict, and leave the database `RESTRICT` constraints as the final safety net.

## Tests

Test both status and observable state. Handler tests use `httptest` through `NewRouter`; service tests use `errors.Is` for sentinel errors and verify persisted snapshot/movement values.
