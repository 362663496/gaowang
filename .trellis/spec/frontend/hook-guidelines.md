# Hook Guidelines

## Current Pattern

There is no data-fetching hook library. Pages use React hooks directly; transient feedback comes from the contextual `App.useApp()` API provided at the root.

## Loading Data

- In a client page, wrap a reusable async `load` function in `useCallback`, call it from `useEffect` with `void load()`, and list every filter/query dependency.
- Use `Promise.all` for independent requests needed by one screen, as in inventory, dashboard, reports, movements, and audit pages.
- Clear the current error before a request and reset `loading` in `finally`.
- The shared API client already handles JSON, structured errors, auth headers, and 401/403 redirects. Hooks/pages call `apiGet`/`apiPost`, not raw `fetch`.

## Derived Values

- Use `useMemo` for derived collections, totals, or serialized query parameters whose inputs are state/props (`URLSearchParams`, inventory value, enabled product options).
- Leave a cheap one-use expression inline; do not add memoization by default.
- Do not copy props into state unless the user can edit the local value. Inventory forms synchronize selected product defaults with narrow effects.

## Shared Hooks

- Hook names start with `use` and live beside the feature that owns them.
- Reuse `App.useApp().message` and `.modal` instead of creating page-specific timers or using static feedback APIs outside the configured context.
- Extract another custom hook only after stateful behavior is reused or a page cannot be tested/read cleanly without it. Keep API-specific types and errors visible to the caller.

## Effect Hygiene

- Effects synchronize with external systems: initial/reload requests, local-storage session events, router redirects, or selected-product defaults.
- Include cleanup for listeners and other ongoing subscriptions. `components/layout/app-shell.tsx` is the listener cleanup reference.
- Avoid an effect for values that can be calculated during render. Ant Design form field dependencies/defaults should prefer `onValuesChange`, `Form.useWatch`, and `form.setFieldValue`.

## Avoid

- Adding React Query, SWR, or a custom cache until the current reload-on-mutation pattern measurably fails.
- A generic `useFetch` wrapper that hides the response envelope or auth redirect behavior.
- Suppressing hook dependency warnings instead of making dependencies stable.
