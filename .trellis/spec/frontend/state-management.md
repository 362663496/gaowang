# State Management

## State Categories

The app intentionally uses no global state library.

- **Page/server data:** fetched into route-local `useState` and refreshed after successful mutations.
- **Form/dialog state:** local to the page or feature form (`open`, selected IDs, editable prices, `saving`, and `error`).
- **Derived state:** computed from page data with direct expressions or `useMemo`; do not persist totals or filtered arrays separately.
- **Filters:** local controlled inputs, serialized with `URLSearchParams` for API requests. Current pages do not mirror filters into the browser URL.
- **Session state:** the one shared browser state, centralized in `lib/api.ts` and stored under `gaowang.devSession` in `localStorage`.

## Session Contract

- Read/write/delete the session only through `readDevSession`, `writeDevSession`, and `apiDeleteSession`.
- `AppShell` listens to the custom `devSessionEvent` and the browser `storage` event so navigation reacts to session changes.
- `request` attaches the development auth headers and clears/redirects the session on HTTP 401 or 403.
- Keep server-rendering safe by guarding access to `window`/storage; `readDevSession` returns an empty session on the server or malformed JSON.

References: `src/lib/api.ts`, `src/lib/api.test.ts`, and `src/components/layout/app-shell.tsx`.

## Server Data And Mutations

- Treat the backend as source of truth. Do not optimistically patch inventory, financial, backup, or audit state.
- After a create/update succeeds, close the form, show a success message, and call the existing page loader.
- Keep independent screen requests concurrent with `Promise.all` but commit each response to its own typed state.
- Preserve explicit loading, error, and empty states rather than encoding them in a single loosely typed object.

## When To Add Shared State

Do not add Context or a store for state used by one route. Add shared state only when multiple mounted branches must edit the same client-owned value and the current session event/helper pattern cannot cover it.

## Avoid

- Duplicating server records in a second global cache.
- Storing derived totals, badge labels, or formatted money strings.
- Reading/writing the session key directly from components.
- Adding reducers or stores for independent booleans and form fields.
