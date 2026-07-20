# Component Guidelines

## Composition

- Pages compose `PageHeader`, Ant Design `Card`/`Table`/`Form`, and Ant Design-backed page feedback. Keep page-only subcomponents in the page file while they remain readable.
- Import generic controls directly from `antd` and icons from `@ant-design/icons`. Shared project components must add domain behavior or page framing, not rename framework primitives.
- Use named function components. Define compact props inline; introduce a local `type Props` when several related components share the shape, as in `features/inventory/action-forms.tsx`.
- Prefer composition through `children` or explicit slots such as `PageHeader.actions` and `EmptyBlock.action` over mode-heavy components.

## Client Pages And Data

Client pages follow the same small loop:

1. Keep `loading`, `error`, and result state locally.
2. Define a stable `load` function when it is reused by an effect, retry, or post-mutation refresh.
3. Pass `loading` to `Table`/`Card`, or render `PageLoading`; expose failures with `Alert`/`PageError` and a retry action.
4. After a mutation, close the `Modal`, call `App.useApp().message`, and reload only the affected page data.

References: `app/(app)/inventory/page.tsx`, `products/page.tsx`, and `settings/backups/page.tsx`.

## Forms And Dialogs

- Use Ant Design `Form` with `layout="vertical"`, `requiredMark={false}`, concrete value types, and `rules`; the backend remains authoritative.
- Convert values explicitly at the API edge. Money input goes through `yuanToCents`; optional text defaults to an empty string. Build `FormData` manually only for multipart image upload.
- Track a `saving` flag, pass it to `Button.loading`, clear the local error before submit, and reset it in `finally`.
- Use controlled Ant Design `Modal` instances. Dangerous confirmation uses `App.useApp().modal.confirm`, never `window.confirm`.
- Display the original `Error.message` in an `Alert` inside the relevant form or page; do not hide server validation/conflict messages behind a generic toast.

## Styling

- Theme Ant Design centrally in `components/layout/app-provider.tsx`; root `DESIGN.md` and `styles/globals.css` remain the visual contract.
- Use Ant Design layout primitives (`Flex`, `Row`, `Col`, `Space`) first. Global CSS is for shell geometry, reusable data-cell layout, and framework gaps—not a replacement utility framework.
- Keep operational tables horizontally scrollable with `Table.scroll.x`; do not crush columns on mobile.
- Prefer borders and tonal surfaces over nested cards or heavy shadows. Keep colors tied to action/status, not decoration.
- Use `@ant-design/icons` and Ant Design `Tag` preset colors for status, always with readable text.

## Accessibility

- Use `Form.Item` so controls have a visible label; supply `aria-label` for icon-only buttons and non-text controls.
- Use `htmlType="submit"` only for the submit button; Ant Design buttons otherwise remain non-submitting.
- Give images meaningful `alt` text and preserve visible focus rings.
- Keep dialogs and drawers on Ant Design primitives for focus/keyboard behavior.
- Preserve the global `prefers-reduced-motion` override in `globals.css`.
- Charts and status indicators must include readable numbers/text, as `reports/page.tsx` does; color alone is not the meaning.

## Avoid

- Rebuilding a raw button, field, dialog, badge, table, pagination, select, upload, or loading/error block inside a page.
- Adding Radix, Tailwind, Lucide, or another general UI system alongside Ant Design without an explicit architecture decision.
- Full-page reloads after small mutations.
- New component abstractions with one speculative use or dozens of boolean presentation props.
