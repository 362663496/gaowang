# Frontend Directory Structure

## Layout

```text
apps/web/src/
├── app/
│   ├── layout.tsx              # root metadata, locale, and global CSS
│   ├── login/page.tsx          # public login route
│   └── (app)/                  # routes wrapped by the authenticated shell
├── components/
│   └── layout/                 # AntD provider, app shell, page header, page feedback
├── features/
│   ├── inventory/              # inventory-specific forms
│   ├── users/                  # testable user form operations
│   ├── labels.tsx              # shared domain labels/badges
│   ├── pagination.ts           # server PaginationMeta → AntD Table adapter
│   ├── product-combobox.tsx    # shared searchable AntD product Select
│   ├── product-image.tsx       # shared AntD image/fallback behavior
│   ├── types.ts                # cross-page API/domain response types
├── lib/                        # API/session and formatting helpers
└── styles/globals.css          # design tokens and framework-gap layout CSS
```

Tests are collocated with the TypeScript module they exercise as `*.test.ts`.

## Placement Rules

- A route's data loading, filters, and one-off view pieces stay in its `app/**/page.tsx` while they have one owner. `products/page.tsx` and `reports/page.tsx` are the local pattern.
- Use Ant Design directly for generic controls and feedback. Move only shell/page framing into `components/layout`; do not build a second generic `components/ui` layer.
- Move domain behavior reused by a page or worth testing without rendering into `features/<domain>`. Existing examples are inventory action forms and user submit helpers.
- Keep transport/session logic in `lib/api.ts`; keep locale-aware display/input conversion in `lib/format.ts`.
- Put response types shared by multiple views in `features/types.ts`. Keep a truly local response row or prop type beside its consumer.
- Keep route groups structural: `(app)` supplies `AppShell` without changing URLs.

## Naming And Imports

- Files and route directories use lowercase kebab-case; React components and TypeScript types use PascalCase; hooks use `use*`.
- Import application code through the `@/*` alias defined in `tsconfig.json`. Existing relative imports inside the same small feature directory are acceptable; do not mix styles within one new module without a reason.
- Default-export Next.js pages/layouts; named-export reusable components, hooks, helpers, and types.
- Add `"use client"` only to modules that use browser APIs, event handlers, or React state/effects. Root layouts, redirects, and presentational components remain server-compatible by default.

## Avoid

- A global `store`, `services`, or barrel-export directory when current local state and direct imports cover the need.
- Moving a one-use page component into shared folders prematurely.
- Duplicating API/session or formatting helpers in a route.
