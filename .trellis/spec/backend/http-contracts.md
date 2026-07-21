# HTTP Contracts

## Scenario: Browser Sessions And Explicit Business Permissions

### 1. Scope / Trigger

Use this contract for authentication, permission changes, or any new protected route under `/api/v1`. The repository web app is the only authenticated client; scripts and third-party clients do not receive a separate token protocol.

### 2. Signatures

- Session APIs: `POST /auth/login`, `GET /auth/me`, `POST /auth/logout`, `POST /auth/password`.
- Permission APIs: `GET /permissions`, `PUT /permissions` with `{"permissions":["product.read"]}`.
- Middleware: `RequireSameOrigin()`, `RequireAuth(db, cfg)`, `RequirePermission(permission)`.
- Database: `sessions(token_hash, user_id, expires_at, created_at)` and `staff_permissions(permission, created_at)`.
- Read-only exports reuse their domain read permission; for example, `GET /inventory/export` requires `inventory.read`.

### 3. Contracts

- Login creates a 32-byte random token and returns `{"user":{"id","name","email","role"},"permissions":[]}`. Only the `HMAC-SHA256(AUTH_SECRET, token)` hex hash is stored.
- The browser receives `gaowang_session` with `Path=/api/v1`, `HttpOnly`, `SameSite=Strict`, and a fixed seven-day lifetime. Set `Secure` from `SESSION_COOKIE_SECURE`, TLS, or trusted `X-Forwarded-Proto: https`.
- Requests use `credentials: "include"`; identity, role, and token are never stored in browser-readable storage or supplied through development headers.
- `admin` receives the full code catalog. `staff` receives only known assignable rows from `staff_permissions`; permission writes validate keys and expand dependencies before replacing rows and recording `permission.updated` in the same transaction.
- Account routes require only a valid session. Every business route declares one `RequirePermission(...)` at registration; handlers do not branch on roles.
- `POST`, `PUT`, `PATCH`, and `DELETE` require an `Origin` matching the direct or forwarded scheme and host.
- The web client treats `401` as session expiry and redirects to `/login`; `403` emits `gaowang:permissions-refresh`, preserves the session, and surfaces the original error.

### 4. Validation & Error Matrix

| Condition | Result |
| --- | --- |
| Missing, unknown, expired session or disabled user | `401 UNAUTHORIZED`, clear the Cookie |
| Invalid login credentials | `401 INVALID_CREDENTIALS` |
| Wrong current password on a valid session | `400 INVALID_CREDENTIALS`, keep the session |
| Missing business permission | `403 FORBIDDEN`, keep the session |
| Missing, malformed, or cross-origin mutation `Origin` | `403 FORBIDDEN` before the handler |
| Unknown or admin-only key in a staff permission update | `400 VALIDATION` |
| Session or permission persistence failure | `500 INTERNAL` without token, password, or secret details |

### 5. Good / Base / Bad Cases

- Good: a staff user with `inventory.read` can list and export inventory; a user without it gets `403` from both routes.
- Base: a zero-permission staff user can still call `/auth/me`, change a password, and log out.
- Bad: a new business route is mounted behind authentication only, or a client handles `403` by logging the user out.

### 6. Tests Required

- Assert login sets an `HttpOnly` Cookie and the database contains only its HMAC hash.
- Assert forged development headers, expired/deleted sessions, and disabled users do not authenticate.
- Assert same-origin mutation handling, current-session logout, and all-session password revocation.
- Enumerate registered routes with a valid zero-permission staff session; every non-public, non-account business route must return `403`.
- Assert permission dependency closure, unknown/admin-only rejection, atomic audit metadata, and independent destructive permissions.
- Assert the web API client sends Cookie credentials, redirects only on `401`, refreshes permissions on `403`, and applies the same behavior to downloads.

### 7. Wrong vs Correct

```go
// Wrong: authenticated staff can reach a new business handler without policy.
group.GET("/inventory/export", inventoryHandler.ExportCurrent)

// Correct: route registration is the explicit policy map.
group.GET("/inventory/export", RequirePermission(services.PermInventoryRead), inventoryHandler.ExportCurrent)
```

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

## Scenario: Multipart Product Create And Update Images

### 1. Scope / Trigger

Use this contract when creating a product with its required main image or editing an active product and optionally replacing the image.

### 2. Signatures

- Request: `PATCH /api/v1/products/:id`, content type `multipart/form-data`
- Create: `POST /api/v1/products`, content type `multipart/form-data`, required `image`
- Fields: `name`, `code`, `default_purchase_cents`, `default_sale_cents`, `low_stock_threshold`, `note`; `image` is required on create and optional on update
- Response: create uses `201 {"item": Product}`; update uses `200 {"item": Product}`

### 3. Contracts

- Only an unarchived product can be updated; `Enabled` and `ArchivedAt` are not editable here.
- Creating a product requires one `image`; existing image-less products remain valid and may be edited without uploading a replacement.
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
| Create without `image` | `400 VALIDATION` |
| Invalid image | `400 UPLOAD_INVALID` |
| Duplicate code or invalid update | `400 PRODUCT_UPDATE_FAILED` |
| Success | create: `201` and `product.create` audit; update: `200` and `product.update` audit |

### 5. Good / Base / Bad Cases

- Good: replace the image, persist all fields, then remove only the previous file.
- Good: create with one JPG/JPEG/PNG/WebP image no larger than 5 MB.
- Base: edit text and prices without an image; the prior image remains readable.
- Bad: delete the old image before the database accepts the replacement.

### 6. Tests Required

- Assert every editable field changes and uneditable status fields remain unchanged.
- Assert create without an image fails, create with an image persists its path, and a failed create removes the newly saved file.
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

## Scenario: Latest Stock Movement Correction

### 1. Scope / Trigger

Use this contract when previewing or correcting an existing inbound, sales-outbound, or adjustment movement. Corrections are accounting mutations, not ordinary CRUD.

### 2. Signatures

- `POST /api/v1/stock-movements/:id/preview` requires `movement.update` and performs no writes.
- `PATCH /api/v1/stock-movements/:id` requires `movement.update` and commits one correction.
- Database: `stock_movements.revision`, `updated_at`, nullable `last_edited_by_id`; response-only `IsLatest`.
- Service: `InventoryService.PreviewMovementUpdate(MovementUpdateInput)` and `UpdateMovement(MovementUpdateInput)`.

### 3. Contracts

Both endpoints accept exactly one JSON object with `expected_revision`, `note`, `change_reason`, and type-specific fields:

```json
{
  "expected_revision": 1,
  "quantity": 8,
  "unit_cents": 350,
  "shop_id": null,
  "note": "业务备注",
  "change_reason": "录入错误"
}
```

- Inbound: positive `quantity`, nonnegative `unit_cents`, optional `shop_id`; no `quantity_delta`.
- Sales outbound: positive `quantity`, nonnegative `unit_cents`, required `shop_id`; no `quantity_delta`.
- Adjustment: nonzero `quantity_delta`, required `note`, no quantity/unit/shop.
- `note` and `change_reason` are at most 500 characters; `change_reason` is always required and exists only in audit metadata.
- Product, type, original operator, and `created_at` are absent from the request and immutable. Unknown JSON fields are rejected.
- Only the latest movement for the product under `created_at DESC, id DESC` is editable. Metadata-only edits are allowed for an archived product; numeric edits are not.
- Preview returns `before`, `after`, `impact`, and `expected_revision`. Save returns the revised `item` and the same impact shape.
- Numeric save reverses the saved latest effect from the current snapshot and reapplies the same pure transition used by create. It updates the original row and increments `revision`; no movement is appended or deleted.
- Snapshot, movement, last editor/time, and complete `movement.updated` audit commit in one transaction.

### 4. Validation & Error Matrix

| Condition | Response |
| --- | --- |
| Invalid ID, field combination, number, note, reason, or unknown field | `400 VALIDATION` |
| Missing movement | `404 MOVEMENT_NOT_FOUND` |
| Missing `movement.update` | `403 FORBIDDEN` |
| Non-latest target or revision mismatch | `409 MOVEMENT_STALE` |
| New outbound/adjustment would make inventory negative | `409 INSUFFICIENT_STOCK` |
| Archived product with changed numeric fields | `409 PRODUCT_ARCHIVED` |
| Snapshot/reversal/audit/persistence failure | `500 INTERNAL`, full rollback |

### 5. Good / Base / Bad Cases

- Good: revise latest sale quantity from 4 to 5; snapshot, row revenue/cost/gross, original-date reports, revision, editor, and audit all change together.
- Base: revise only note or shop; snapshot and all derived money fields remain unchanged while revision and audit advance.
- Bad: calculate impact in the browser, patch a non-latest row, or update snapshot without the row/audit; these paths bypass accounting and stale-write protection.

### 6. Tests Required

- Service: all three transitions, moving-average/rounding behavior, metadata-only edit, archived numeric rejection, insufficient stock, stale ID/version, preview no-write, and audit-failure rollback.
- Route: independent permission on preview/PATCH, strict unknown-field rejection, safe operator DTO without `PasswordHash`, `IsLatest`, stable error codes, and immutable identity/time.
- Report: an edited sale remains in its original `created_at` period with corrected revenue/cost/gross.

### 7. Wrong vs Correct

```go
// Wrong: direct snapshot update loses revision, movement, and audit guarantees.
db.Model(&models.InventorySnapshot{}).Where("product_id = ?", productID).Update("quantity", newQuantity)

// Correct: the service owns latest/version checks, shared accounting, and the transaction.
movement, impact, err := (services.InventoryService{DB: db}).UpdateMovement(input)
```
