# Frontend Quality Guidelines

## Required Checks

Run from `apps/web`:

```bash
npm run lint
npx tsc --noEmit --incremental false
npm test
npm run build
```

ESLint uses Next.js core-web-vitals and TypeScript rules. The project explicitly disables only `react-hooks/set-state-in-effect`; do not broaden that exception list to silence new code.

## Tests

- Vitest is the installed test runner. Keep tests beside the module as `*.test.ts`.
- Test pure formatting/conversion helpers with direct inputs and outputs (`lib/format.test.ts`).
- Test API/session behavior with stubbed `fetch`, `window`, and local storage (`lib/api.test.ts`).
- Extract a form operation into a small feature helper when async DOM behavior needs a reliable test (`features/users/create-user.ts` and `password.ts`).
- Restore globals/mocks in `afterEach`; do not leak browser stubs between tests.
- No component-rendering or end-to-end test library is installed. Do not add one for behavior covered by a pure helper or API-client test.

## UI Acceptance

- Every remote screen exposes loading, error, retry where appropriate, and empty states.
- Mutating buttons show disabled/loading feedback and forms retain an actionable error.
- Icon-only controls have accessible names, keyboard focus remains visible, and status is not communicated by color alone.
- Tables remain usable at narrow widths through horizontal scrolling.
- Visual work follows `DESIGN.md`, shared CSS variables, and reduced-motion behavior.

## Review Checklist

- `"use client"` appears only where browser/reactive behavior requires it.
- API calls go through `lib/api.ts`; 401/403 behavior and the structured error shape remain centralized.
- API response field names and union values match the backend.
- Money conversion stays at the input/display edge and API values remain cents.
- New UI uses installed Ant Design components before adding another component or dependency; shared wrappers must provide domain behavior or page framing.
- Effects have accurate dependencies and cleanup where needed.
- A mutation refreshes the relevant server state and cannot double-submit.

## Avoid

- New dependencies for behavior already covered by React or Ant Design.
- Snapshot-heavy or tautological tests that cannot fail when the behavior breaks.
- Generic form/table abstractions before two real consumers share the same behavior.
