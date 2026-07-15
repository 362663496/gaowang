# Frontend Development Guidelines

These guides describe the Next.js app in `apps/web`: App Router pages, strict TypeScript, Tailwind CSS v4, local UI primitives, direct API calls, and page-local state.

## Guides

| Guide | Use it for |
| --- | --- |
| [Directory Structure](./directory-structure.md) | Route, component, feature, type, and utility ownership |
| [Component Guidelines](./component-guidelines.md) | Page/component composition, styling, forms, and accessibility |
| [Hook Guidelines](./hook-guidelines.md) | Existing hook and data-loading patterns |
| [State Management](./state-management.md) | Local, server-derived, session, and filter state |
| [Type Safety](./type-safety.md) | API contracts, strict TypeScript, and boundary conversion |
| [Quality Guidelines](./quality-guidelines.md) | Lint, type-check, tests, build, and review checks |

## Pre-Development Checklist

1. Always read [Directory Structure](./directory-structure.md), [Component Guidelines](./component-guidelines.md), and [Quality Guidelines](./quality-guidelines.md).
2. Read [Hook Guidelines](./hook-guidelines.md) and [State Management](./state-management.md) before changing client loading, filters, forms, or shared browser state.
3. Read [Type Safety](./type-safety.md) before changing API payloads, shared feature types, money fields, or session behavior.
4. For visual changes, follow root `DESIGN.md` and reuse `src/components/ui` primitives and CSS variables from `src/styles/globals.css`.
5. Keep frontend contracts aligned with the Go handlers/models in `apps/api`.

## Baseline Verification

```bash
cd apps/web
npm run lint
npx tsc --noEmit --incremental false
npm test
npm run build
```
