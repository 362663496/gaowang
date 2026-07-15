# Backend Directory Structure

## Layout

```text
apps/api/
├── cmd/api/main.go                 # process startup and shutdown
├── internal/config/                # environment parsing and validation
├── internal/db/                    # GORM connection and AutoMigrate list
├── internal/models/                # persisted models and domain enums
├── internal/http/router.go         # route groups and dependency wiring
├── internal/http/middleware.go     # authentication and role gates
├── internal/http/handlers/         # HTTP parsing, responses, and audit calls
└── internal/services/              # reusable business and side-effect logic
```

Tests stay beside the package they exercise as `*_test.go`.

## Dependency Direction

- `cmd/api/main.go` loads config, opens/migrates the database, builds the router, and owns process-level logging.
- `internal/http/router.go` constructs small handler values with explicit `DB` and `Cfg` fields and mounts protected/admin route groups.
- Handlers own HTTP concerns: Gin binding, UUID/form parsing, status codes, response envelopes, and audit calls. See `internal/http/handlers/inventory.go` and `products.go`.
- Services own logic that must remain correct outside one endpoint. Inventory accounting and transactions live in `internal/services/inventory.go`; backup, SMTP, password, and upload operations have their own service files.
- Models contain persistence shape, enums, associations, and UUID hooks. They do not call handlers or services.

## Adding Backend Behavior

- Register endpoints in `internal/http/router.go`; do not hide route registration in feature init functions.
- Add a handler to the existing domain file, or create one domain-named handler file when no owner exists.
- Put multi-step business rules, filesystem/process work, or reusable security logic in `internal/services`.
- Use GORM directly for ordinary CRUD. This project has no repository interface layer; do not add one for a single caller.
- Keep configuration fields and environment parsing in `internal/config/config.go`, then pass `config.Config` explicitly.

## Naming

- Go packages and filenames are lowercase; multiword filenames use underscores only when needed by Go conventions, such as `config_test.go`.
- Handlers use `<Domain>Handler`; service inputs use intent names such as `InboundInput` and `OutboundInput`.
- Persisted enums are named string types with constants, as in `models.Role`, `models.MovementType`, and `models.BackupStatus`.
- Import `internal/http` as `apihttp` where it would collide with `net/http`, following `cmd/api/main.go` and handler integration tests.

## Avoid

- Business calculations in route registration or `main.go`.
- A new `utils`, repository, interface, or factory package for code with one clear owner.
- Circular ownership between handlers, services, and models.
