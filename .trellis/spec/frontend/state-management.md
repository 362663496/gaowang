# State Management

## State Categories

The app intentionally uses no global state library.

- **Page/server data:** fetched into route-local `useState` and refreshed after successful mutations.
- **Form/dialog state:** local to the page or feature form (`open`, selected IDs, editable prices, `saving`, and `error`).
- **Derived state:** computed from page data with direct expressions or `useMemo`; do not persist totals or filtered arrays separately.
- **Filters:** local controlled inputs, serialized with `URLSearchParams` for API requests. Current pages do not mirror filters into the browser URL.
- **Session state:** the one shared browser state, loaded from `/auth/me` into `SessionProvider`; the browser-readable state contains only the returned user and permission keys.

## Session Contract

- `SessionProvider` owns `user`, `permissions`, `loading`, and `error`; pages consume `useSession()` and `hasPermission(key)` instead of copying session state.
- `fetchAuthMe()` is the source of truth. Refresh it on initial mount, window focus, and `gaowang:permissions-refresh`.
- `request` and `apiDownload` send `credentials: "include"`. The server owns the `HttpOnly` Cookie; client code never reads or stores the token, user ID, or role in `localStorage`.
- A `401` emits `gaowang:session-expired`, clears in-memory session state, and redirects to `/login`. A `403` emits only `gaowang:permissions-refresh`; it does not log the user out.
- Navigation and action visibility use the same permission keys returned by `/auth/me`, while the backend remains the authoritative enforcement layer.

References: `src/lib/api.ts`, `src/lib/api.test.ts`, `src/components/layout/session-context.tsx`, and `src/components/layout/app-shell.tsx`.

## Server Data And Mutations

- Treat the backend as source of truth. Do not optimistically patch inventory, financial, backup, or audit state.
- After a create/update succeeds, close the form, show an `App.useApp().message` success message, and call the existing page loader.
- Keep independent screen requests concurrent with `Promise.all` but commit each response to its own typed state.
- Preserve explicit loading, error, and empty states rather than encoding them in a single loosely typed object.

## When To Add Shared State

Do not add Context or a store for state used by one route. `SessionProvider` is the single exception because the shell, navigation, permission gates, and pages consume the same server-derived identity.

## Avoid

- Duplicating server records in a second global cache.
- Storing derived totals, badge labels, or formatted money strings.
- Persisting session identity, role, permissions, or tokens in browser-readable storage.
- Adding reducers or stores for independent booleans and form fields.
