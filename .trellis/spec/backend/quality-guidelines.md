# Backend Quality Guidelines

## Required Checks

Run these from `apps/api` after backend changes:

```bash
gofmt -w <changed-go-files>
go test ./...
go vet ./...
```

The root command `make api-test` runs `go test ./...`.

## Local Test Style

- Use the standard `testing` package; no assertion or mocking framework is installed.
- Name behavior tests `Test_<subject>_<behavior>`. Many tests use brief Given/When/Then comments, but clarity matters more than comments.
- Keep unit tests in the same package when they exercise unexported logic. Use an external `_test` package for route-level behavior when package internals are not needed.
- Route tests build `NewRouter`, send `httptest` requests, and assert both HTTP output and persisted state.
- Database tests use a unique in-memory SQLite DSN and a silent GORM logger. Migrate only the models the test requires.
- Use `t.Setenv`, `t.TempDir`, real JSON encoding, and other standard-library facilities before inventing fixtures.

References: `internal/config/config_test.go`, `internal/services/inventory_test.go`, `internal/http/router_test.go`, and `internal/http/handlers/audit_test.go`.

## What Must Be Tested

- Inventory quantity, valuation, insufficient-stock, transaction, and rounding behavior.
- Authentication/authorization and any response that might expose secrets.
- Request-boundary cases where zero, omission, UUID parsing, or legacy field casing differ.
- Dialect-sensitive aggregate queries and empty collection responses.
- Backup/file behavior that can lose data; keep local backup success independent from email success.

## Review Checklist

- Money remains integer cents and stock remains `int64`.
- Stock writes still route through `InventoryService` and append a movement in the same transaction.
- New routes are mounted in the correct protected or admin group.
- Public user responses never include `PasswordHash`.
- Errors retain context without leaking credentials or internal payloads.
- A new model is included in production and test migrations.
- API response names/casing still match the frontend types.

## Avoid

- Repository/interfaces/factories with one implementation.
- Framework-heavy fixtures for behavior covered by `httptest`, SQLite, or a small table test.
- Changing product source solely to make a speculative abstraction possible.
