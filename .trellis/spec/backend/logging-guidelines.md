# Logging Guidelines

The project has three intentionally small logging channels; it does not have a custom logger abstraction.

## Process Logs

`cmd/api/main.go` uses the standard-library `log/slog` package for startup, migration, server, and shutdown failures:

```go
slog.Error("open database", slog.Any("err", err))
```

Keep the message as a short operation and attach the error under `err`. Process startup failures exit with a non-zero status after logging.

## HTTP Logs

`internal/http/router.go` installs `gin.Logger()` and `gin.Recovery()` once on the engine. Do not add per-handler request logging or another recovery layer unless the global behavior is deliberately replaced.

## Audit Events

User and domain mutations are persisted as `models.AuditLog` through `recordAudit`/`recordAuditForActor` in `internal/http/handlers/audit.go`.

- Action names use `<resource>.<past-tense-action>`, for example `product.create`, `inventory.sales_outbound`, and `backup.run_failed`.
- Store resource type, resource ID, actor when known, client IP, and a small `map[string]string` of useful metadata.
- Record both successful and security-relevant failed actions when the current handlers do so; login is the reference.
- Audit writes are best effort and must not replace the primary operation response.

## Never Log

- Passwords or password hashes.
- `AUTH_SECRET`, SMTP credentials, or a database URL containing credentials.
- Full request bodies for authentication, settings, or uploads.
- Backup attachment contents.

`internal/http/handlers/audit_test.go` explicitly verifies that a failed-login audit record does not contain the submitted password.

## Avoid

- Adding a logger interface with one implementation.
- Logging an error in every layer; add context while returning it, then log once at the process boundary that owns the failure.
- Using audit metadata as a dump of arbitrary request data.
