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

## Tests

Test both status and observable state. Handler tests use `httptest` through `NewRouter`; service tests use `errors.Is` for sentinel errors and verify persisted snapshot/movement values.
