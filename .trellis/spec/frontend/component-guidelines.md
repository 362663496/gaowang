# Component Guidelines

## Composition

- Pages compose `PageHeader`, local summary/panel/table components, and the shared loading/empty/error states. Keep page-only subcomponents in the page file while they remain readable.
- Reuse primitives from `src/components/ui`: `Button`, `Field`/`Input`/`Select`/`Textarea`, `Dialog`, `Badge`, `MessageBar`, and the state blocks.
- Use named function components. Define compact props inline; introduce a local `type Props` when several related components share the shape, as in `features/inventory/action-forms.tsx`.
- Prefer composition through `children` or explicit slots such as `PageHeader.actions` and `EmptyBlock.action` over mode-heavy components.

## Client Pages And Data

Client pages follow the same small loop:

1. Keep `loading`, `error`, and result state locally.
2. Define a stable `load` function when it is reused by an effect, retry, or post-mutation refresh.
3. Render `LoadingBlock`, then `ErrorBlock`, then an empty/table/panel state.
4. After a mutation, close the dialog, show `MessageBar`, and reload only the affected page data.

References: `app/(app)/inventory/page.tsx`, `products/page.tsx`, and `settings/backups/page.tsx`.

## Forms And Dialogs

- Use semantic `<form onSubmit>` and browser validation attributes (`required`, `type`, `min`, `minLength`, `maxLength`, and `step`) for immediate feedback; the backend remains authoritative.
- Capture `event.currentTarget` before awaiting when a helper must reset the form later. `features/users/create-user.ts` is the tested example.
- Convert `FormData` values explicitly. Money input goes through `yuanToCents`; numeric quantities use `Number`; optional text defaults to an empty string.
- Track a `saving` flag, pass it to `Button.loading`, clear the local error before submit, and reset it in `finally`.
- Use controlled Radix dialogs when the parent must close them after success. Use `DialogClose` for cancel actions.

## Styling

- Use Tailwind utility classes and the CSS variables in `styles/globals.css`; root `DESIGN.md` is the visual contract.
- Use `cn` for conditional/merged classes and `class-variance-authority` only where a primitive has real variants, as in `Button`.
- Keep operational tables horizontally scrollable with an explicit `min-w-*`; do not crush columns on mobile.
- Prefer borders and tonal surfaces over nested cards or heavy shadows. Keep colors tied to action/status, not decoration.
- Use Lucide for icons and the existing badge tone vocabulary.

## Accessibility

- Use `Field` so controls have a visible label; supply `aria-label` for icon-only buttons and non-text controls.
- Set `type="button"` on buttons that must not submit a form.
- Give images meaningful `alt` text and preserve visible focus rings.
- Keep dialogs on the Radix primitive for focus/keyboard behavior.
- Preserve the global `prefers-reduced-motion` override in `globals.css`.
- Charts and status indicators must include readable numbers/text, as `reports/page.tsx` does; color alone is not the meaning.

## Avoid

- Rebuilding a raw button, field, dialog, badge, or loading/error block inside a page.
- Full-page reloads after small mutations.
- New component abstractions with one speculative use or dozens of boolean presentation props.
