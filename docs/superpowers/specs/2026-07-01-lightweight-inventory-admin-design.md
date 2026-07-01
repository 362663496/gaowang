# Lightweight Inventory Admin Design

Date: 2026-07-01

## 1. Purpose

Build a lightweight self-hosted inventory admin system for a small retail operation. The system tracks products, one warehouse inventory pool, shop-attributed sales, stock movements, basic gross profit, user operations, backups, and backup email delivery.

This is not a full ERP. The first release focuses on keeping stock counts reliable after purchase intake and manual sales outbound entries.

## 2. Confirmed Scope

In scope:

- Single warehouse inventory pool.
- No SKU variants. One product equals one inventory item.
- Manual sales outbound entry, one record at a time.
- Product images uploaded to the server.
- Product name, code, image, default purchase price, default sale price, low-stock threshold, note, enabled status.
- Shops used as sales attribution only. Shops do not hold separate stock.
- Users with two roles: admin and staff.
- Immutable stock history. Incorrect records are corrected through adjustment records, not by editing or deleting old movements.
- Current inventory, low-stock list, stock movement list, sales amount, cost, and basic gross profit reports.
- Desktop browser first.
- Cloud-server self-hosting with Docker Compose, Nginx, HTTPS, PostgreSQL, local uploads, and scheduled backups.
- SQL backup email delivery after backup generation.
- Modern, polished, responsive admin UI based on a defined design system.

Out of scope for the first release:

- Full ERP purchasing workflows.
- Multiple warehouses.
- Product SKU variants.
- Purchase approvals.
- Platform API sync.
- Financial settlement, tax, accounts receivable, or accounting ledgers.
- Barcode scanner workflows.
- Object storage.
- Multi-tenant SaaS.
- Redis, message queues, and microservices.

## 3. Product Model

Core modules:

- Dashboard: today sales amount, today gross profit, low-stock products, recent operations.
- Product management: product list, image upload, product detail, enable or disable product.
- Shop management: shop list for sales attribution.
- Current inventory: stock quantity, moving average cost, inventory value, low-stock status.
- Inbound stock: create purchase intake records.
- Sales outbound: choose shop and product, enter quantity and sale price, then reduce stock.
- Stock adjustment: increase or decrease stock for stocktake differences or correction records.
- Stock movement log: filter all inventory changes by product, type, shop, operator, and time.
- Reports: basic sales, cost, gross profit, inventory value, low-stock report.
- System settings: users, shops, password management, backup settings, backup history.

## 4. Roles And Permissions

Admin:

- Manage products.
- Manage shops.
- Manage users.
- Create inbound, outbound, and adjustment records.
- View inventory, movements, reports, audit logs, and backup history.
- Configure backup and backup email settings.

Staff:

- Create inbound, outbound, and adjustment records.
- View products, inventory, movements, and reports.
- Cannot manage users.
- Cannot change backup settings.

All stock operations record operator, timestamp, source IP if available, and operation type.

## 5. Inventory Accounting

Stock is driven by an append-only movement ledger.

Movement types:

- `inbound`: positive quantity, purchase unit price, purchase amount.
- `sales_outbound`: negative quantity, sale unit price, revenue amount, cost amount, gross profit.
- `adjustment`: positive or negative quantity, required reason.

Current inventory is stored in `inventory_snapshots` for fast listing. It is updated in the same database transaction as the movement record.

Cost method:

- Use moving weighted average cost.
- Inbound recalculates average cost from existing inventory value plus new purchase amount.
- Sales outbound uses the current average cost at the moment of sale.
- Gross profit equals sale amount minus cost amount.

Rules:

- Inbound quantity must be greater than zero.
- Sales outbound quantity must be greater than zero.
- Sales outbound cannot reduce stock below zero.
- Adjustment quantity cannot be zero and must include a reason.
- Historical stock movements cannot be edited or deleted.
- Products with movements can be disabled but not hard-deleted.

## 6. Data Model

Tables:

- `users`: id, name, email or username, password hash, role, enabled status, timestamps.
- `shops`: id, name, note, enabled status, timestamps.
- `products`: id, name, code, image path, default purchase price, default sale price, low-stock threshold, note, enabled status, timestamps.
- `stock_movements`: id, movement type, product id, shop id nullable, quantity delta, purchase unit price nullable, sale unit price nullable, cost unit price, amount fields, reason, operator id, timestamps.
- `inventory_snapshots`: product id, quantity, moving average cost, inventory value, updated timestamp.
- `audit_logs`: id, actor id, action, resource type, resource id, metadata, IP address, timestamp.
- `backup_jobs`: id, started at, finished at, status, file path, file size, email status, recipient, error message.
- `settings`: key, value, updated timestamp.

Money should use integer cents or fixed precision decimal in PostgreSQL. Floating point is not allowed for money or stock quantities.

## 7. UI And Design Direction

The UI is a modern operational admin system, not a template-like ERP screen.

Design read:

- Desktop-first inventory command center.
- Dense enough for repeated daily work.
- Modern, precise, and polished, inspired by Linear-style product UI.
- No decorative dashboard clutter.

Frontend stack:

- Next.js with TypeScript.
- Tailwind CSS v4.
- shadcn/ui components customized through local ownership.
- Icon library with a single consistent family.
- Motion only for meaningful state transitions, such as dialogs, drawers, row updates, and button feedback.

Design system requirement:

- Before UI implementation, create a root `DESIGN.md`.
- `DESIGN.md` must define atmosphere, color tokens, typography, spacing, components, motion, and surface depth.
- Components must cover loading, empty, error, disabled, focus, hover, and active states.

Primary layout:

- Left sidebar navigation.
- Top area for search, filters, and quick actions.
- Main table/list work area.
- Right-side detail drawer for product and movement detail.
- Dialog or drawer forms for inbound, outbound, and adjustment creation.

High-frequency UX:

- Quick product search.
- Default prices auto-filled from product.
- Clear save progress and success feedback.
- Inline stock shortage warning before submit.
- Backend stock shortage response mapped to a clear form error.
- Partial refresh after create operations rather than full-page disruption.

## 8. Technical Architecture

Use a small monorepo-style project with separate frontend and backend apps.

Recommended structure:

- `apps/web`: Next.js frontend.
- `apps/api`: Go API.
- `deploy`: Docker Compose, Nginx, backup scripts.
- `docs`: design and implementation documentation.

Backend:

- Go.
- Gin for HTTP routing, route groups, middleware, JSON binding, and validation.
- GORM for models, CRUD, migrations, associations, and transactions.
- PostgreSQL as the database.
- Centralized JSON error responses.
- Request logging, panic recovery, authentication middleware, and role middleware.

GORM usage rule:

- Normal CRUD can use GORM directly.
- Inventory operations must use explicit transactions.
- Inventory snapshot rows must be locked during stock-changing operations to prevent concurrent oversell.
- If GORM becomes unclear for a critical stock query, raw SQL inside the same transaction is acceptable.

Authentication:

- Username or email plus password.
- Passwords stored as hashes.
- Secure cookie-based session or signed token-based auth.
- Session expiration returns a consistent unauthorized response.

File upload:

- Product images are stored in a server uploads directory.
- Database stores relative paths.
- Nginx serves `/uploads`.
- Allowed image types and file size limit are enforced by the Go API.

API shape:

- `/api/v1/auth/*`
- `/api/v1/products/*`
- `/api/v1/shops/*`
- `/api/v1/inventory/*`
- `/api/v1/stock-movements/*`
- `/api/v1/reports/*`
- `/api/v1/users/*`
- `/api/v1/backups/*`
- `/api/v1/settings/*`

## 9. Backup And Email Delivery

Backup feature is in scope for the first release.

Behavior:

- Scheduled task runs `pg_dump`.
- Output filename includes timestamp.
- SQL file is gzip-compressed to `.sql.gz`.
- Uploads directory is backed up separately.
- Local backups are retained for a configurable number of days, default 7 and recommended 30.
- Backup result is written to `backup_jobs`.
- After database backup succeeds, the system sends the `.sql.gz` file to configured recipient email through SMTP.

Configuration:

- SMTP host.
- SMTP port.
- SMTP username.
- SMTP password or authorization code.
- Sender email.
- Recipient email.
- TLS mode.
- Attachment size limit.
- Retention days.

Failure handling:

- If `pg_dump` fails, mark backup failed and do not send email.
- If gzip fails, mark backup failed and keep error details.
- If email sending fails, keep the local backup and mark only email failed.
- If attachment exceeds configured size, keep the local backup and send a notification email without the attachment if possible.
- The UI shows latest backup status, latest email status, file size, and error message.

## 10. Deployment

Use Docker Compose on a cloud server.

Services:

- `web`: Next.js frontend.
- `api`: Go Gin API.
- `postgres`: PostgreSQL database.
- `nginx`: HTTPS reverse proxy and uploads static serving.

Routing:

- `/` routes to the frontend.
- `/api` routes to the Go API.
- `/uploads` serves uploaded product images.

Operational requirements:

- Environment variables for database, auth secret, upload path, backup path, and SMTP config.
- Persistent volumes for PostgreSQL data, uploads, and backups.
- HTTPS enabled through Nginx.
- A documented restore process from SQL backup.

## 11. Error Handling

Expected errors must be explicit and user-facing:

- `401`: not logged in.
- `403`: permission denied.
- `400`: validation error.
- `409`: stock conflict or insufficient stock.
- `413`: upload too large.
- `500`: unexpected server error.

Frontend maps these to specific UI states:

- Login redirect for expired sessions.
- Inline form messages for validation.
- Clear stock shortage messages for outbound attempts.
- Toast or banner for backup and upload failures.
- Empty states for no products, no stock movements, and no reports.

## 12. Testing And Acceptance

Backend tests:

- Inbound creates movement and updates inventory.
- Sales outbound creates movement, updates inventory, and calculates gross profit.
- Sales outbound fails when stock is insufficient.
- Adjustment creates movement and updates inventory.
- Concurrent outbound cannot oversell.
- Admin and staff permissions behave correctly.
- Backup creates SQL gzip file.
- Email failure does not delete local backup.

Frontend acceptance flows:

- Login.
- Create product with image.
- Create shop.
- Create inbound record.
- Create sales outbound record.
- See current stock decrease.
- See low-stock status.
- Search and filter stock movements.
- View sales, cost, and gross profit report.
- View latest backup status.

UI acceptance:

- Desktop 1280px layout is the primary target.
- Tables, forms, dialogs, drawers, loading states, empty states, and error states are visually polished.
- No text overflow in buttons, table cells, dialogs, or sidebars.
- Keyboard focus states are visible.
- Motion respects reduced-motion preference.

## 13. Open Implementation Notes

These are implementation choices, not unresolved product requirements:

- Decide exact session mechanism during implementation, but it must support secure cloud deployment.
- Decide whether scheduled backup runs inside the API container or a separate backup container during implementation.
- Start with local uploads and local backups. Object storage can be added later only when file volume or deployment topology requires it.

