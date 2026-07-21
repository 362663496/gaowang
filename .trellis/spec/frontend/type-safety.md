# Type Safety

## Compiler Contract

`apps/web/tsconfig.json` enables `strict`, disallows JavaScript, and emits no files. Use type-only imports where the import is not a runtime value.

Run:

```bash
cd apps/web
npx tsc --noEmit --incremental false
```

## Type Ownership

- Shared API/domain types live in `src/features/types.ts`.
- Transport/session primitives (`Role`, `SessionUser`, `AuthPayload`, and `ApiError`) live with `src/lib/api.ts`.
- Component-only prop and aggregate row types stay next to the component/handler that owns them.
- Model finite states as unions: `Role`, `MovementType`, and backup status are current examples.

## API Contracts

- Every `apiGet`/`apiPost` call supplies its response envelope type, for example `{ items: Product[] }` or `{ settings: AppSettings }`.
- Match backend field names exactly. GORM model responses currently use PascalCase fields, while explicit report/audit/settings DTOs use snake_case and user DTOs use lowercase fields. Do not guess or normalize casing in a component.
- When a backend contract changes, update the handler/model response, `features/types.ts`, every consumer, and focused tests in the same change.
- Monetary values cross the API as integer cents; format and parse them only through `lib/format.ts` helpers.

## Runtime Boundaries

No schema-validation library is installed. Runtime checks are narrow and local:

- `AuthPayload` models the explicit lowercase `/auth/login` and `/auth/me` user DTO plus its permission-key array; components do not cast the response locally.
- `readError` accepts missing error fields and supplies stable fallbacks.
- Forms use typed Ant Design `Form` values and rules plus explicit string/number conversion; the Go API performs authoritative validation.
- `ApiError` carries `code`, `message`, and HTTP `status`; UI code normally displays `Error.message`.

Add a runtime validator only for an actually untrusted/complex contract that cannot be checked with a few explicit conditions.

## Avoid

- `any` in product code. Use `unknown` at an untyped boundary, then narrow it.
- Unchecked casts to paper over an API mismatch. The test suite uses `unknown as` only to build minimal DOM-event fakes.
- Duplicating the same response type in several pages.
- Floating-point money in API state; decimal numbers exist only as user-facing strings before `yuanToCents`.
