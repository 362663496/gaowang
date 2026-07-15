# Database Guidelines

## Stack And Schema

- Production uses PostgreSQL through GORM (`internal/db/db.go`). Tests commonly use an isolated in-memory SQLite database with a UUID in its DSN.
- `gorm.Config{SkipDefaultTransaction: true}` is intentional. Ordinary CRUD is direct; atomic workflows must open an explicit transaction.
- Schema creation currently uses `db.Migrate` and `AutoMigrate`. Add every new persisted model to the centralized list in `internal/db/db.go`; there is no versioned migration directory yet.
- UUID primary keys are assigned by `BeforeCreate` hooks in `internal/models/models.go`.

## Data Representation

- Money is integer cents in `int64` fields (`DefaultSaleCents`, `RevenueCents`, and related fields). Never use floating point for stored or calculated money.
- Stock quantities are `int64`. Movement direction is represented by signed `QuantityDelta`.
- Nullable database values use pointers, for example `ShopID`, `SaleUnitCents`, and `FinishedAt`.
- Persisted enum values use typed strings; JSON metadata uses PostgreSQL `jsonb` via `datatypes.JSON`.
- Let GORM map Go names to plural snake-case tables and snake-case columns unless an existing contract requires an explicit tag.

## Inventory Invariants

`internal/services/inventory.go` is the only write path for stock accounting:

- Run snapshot and movement writes in the same `DB.Transaction` callback.
- Lock an existing snapshot with `clause.Locking{Strength: "UPDATE"}` before changing stock.
- Append a `StockMovement`; never edit or delete historical movements to correct stock.
- Reject outbound or negative adjustments that would make stock negative with `ErrInsufficientStock`.
- Keep `Quantity`, `MovingAverageCostCents`, and `InventoryValueCents` consistent. When an outbound empties stock, consume the remaining stored value to avoid rounding residue.

References: `internal/services/inventory.go` and `internal/services/inventory_test.go`.

## Query Patterns

- Bind values with GORM parameters (`Where("product_id = ?", id)`); never concatenate input into SQL.
- Use `Preload` when list responses need associations, as in inventory, movements, and audit handlers.
- Use explicit projection structs for public or aggregate responses rather than exposing sensitive model fields. `userResponse` and the report row structs are the local examples.
- Aggregate reports use `Table`, `Select`, `Joins`, `Group`, and `Scan` in `internal/http/handlers/reports.go`. Keep dialect differences behind a small helper such as `reportDateExpr` so SQLite tests remain useful.
- Bound list size where endpoints can grow; movement, audit, and ranking handlers set explicit limits.

## Settings And Defaults

Runtime settings use the `settings` key/value table. Database values override environment fallbacks, as shown by `BackupHandler.backupRecipient`. Keep the key constant next to its handler and test both stored and fallback behavior.

## Avoid

- Stock updates outside `InventoryService` or outside an explicit transaction.
- Floating-point money, hard deletion of movement history, or a snapshot-only correction.
- Adding a model without updating `db.Migrate` and a relevant test database migration.
- Assuming SQLite proves PostgreSQL-only SQL; keep dialect-specific behavior explicit and exercise production queries when they become critical.
