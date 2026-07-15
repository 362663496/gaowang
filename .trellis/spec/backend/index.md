# Backend Development Guidelines

These guides describe the Go API in `apps/api` as it exists today: Gin handlers, GORM models, explicit inventory services, PostgreSQL in production, and SQLite-backed tests.

## Guides

| Guide | Use it for |
| --- | --- |
| [Directory Structure](./directory-structure.md) | Package ownership and where new backend code belongs |
| [Database Guidelines](./database-guidelines.md) | GORM models, queries, transactions, and money/stock rules |
| [Error Handling](./error-handling.md) | Service errors and the JSON API error contract |
| [Logging Guidelines](./logging-guidelines.md) | Startup logs, request logs, and persistent audit events |
| [Quality Guidelines](./quality-guidelines.md) | Formatting, tests, and review checks |

## Pre-Development Checklist

1. Always read [Directory Structure](./directory-structure.md) and [Quality Guidelines](./quality-guidelines.md).
2. Read [Database Guidelines](./database-guidelines.md) before changing models, queries, reports, settings, or inventory logic.
3. Read [Error Handling](./error-handling.md) before changing handlers, middleware, validation, or service failures.
4. Read [Logging Guidelines](./logging-guidelines.md) before changing startup behavior or auditable mutations.
5. Preserve the API contract used by `apps/web/src/features/types.ts` and `apps/web/src/lib/api.ts`.

## Baseline Verification

```bash
cd apps/api
gofmt -w <changed-go-files>
go test ./...
go vet ./...
```

`make api-test` is the repository-root shortcut for the full Go test suite.
