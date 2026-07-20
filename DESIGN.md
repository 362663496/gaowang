# Gaowang Inventory Admin Design System

## 1. Atmosphere And Identity

Gaowang is a precise inventory command center for daily retail operations. The interface should feel quiet, fast, and deliberate: dark navigation, light working canvas, dense data tables, clear action surfaces, and drawers or dialogs that keep operators in flow.

The product is not a marketing site and not a decorative ERP template. It should prioritize scanning, comparison, and repeated stock entry.

## 2. Color Tokens

| Role | Token | Light | Usage |
| --- | --- | --- | --- |
| Page | `--surface-page` | `#f6f7f9` | App background |
| Panel | `--surface-panel` | `#ffffff` | Tables, forms, metric blocks |
| Raised | `--surface-raised` | `#ffffff` | Dialogs and overlays |
| Sidebar | `--surface-sidebar` | `#111317` | Navigation rail |
| Text primary | `--text-primary` | `#15171c` | Headings and primary cells |
| Text secondary | `--text-secondary` | `#5d6572` | Labels and support text |
| Text muted | `--text-muted` | `#8b929e` | Metadata |
| Border | `--border-subtle` | `#e3e6ec` | Table lines and controls |
| Accent | `--accent-primary` | `#5e6ad2` | Primary actions and focus |
| Accent hover | `--accent-strong` | `#4852bd` | Primary hover |
| Success | `--status-success` | `#138a45` | Success state |
| Warning | `--status-warning` | `#b7791f` | Low stock |
| Error | `--status-error` | `#c93535` | Destructive or failed state |

Avoid one-note palettes. The primary canvas stays neutral; color is reserved for action and state.

## 3. Typography

Use Geist Sans with system fallback. Use `ui-monospace` only for codes, IDs, and file paths.

- Body: 14px, 20px line height.
- Compact labels: 12px, 16px line height.
- Page titles: 24px, 32px line height, weight 650.
- Table cells: 14px, 20px line height.
- No negative letter spacing.

## 4. Layout

Desktop is primary. The shell uses a 232px sidebar, a 64px sticky topbar, and a constrained content width of 1440px. Mobile collapses navigation into a top bar and keeps tables horizontally scrollable instead of crushing columns.

Use 4px spacing increments. Common section gaps are 16px, 20px, and 24px. Cards have 8px radius or less.

## 5. Components

Buttons, inputs, selects, dialogs, tables, badges, tabs, empty states, loading states, errors, disabled states, hover states, active states, and focus rings must be implemented consistently.

Ant Design 6 is the implementation layer for these generic components. Theme it centrally through `AppProvider`; add project components only for inventory-domain behavior or shell/page framing, not a parallel primitive library.

Buttons use icons when the action is a tool-like command. Text buttons are reserved for clear create, save, or submit actions. Focus states use an accent ring. Tables use row hover and compact status badges.

Operational charts use compact bars and trend rows inside regular panels. They should share table density, use neutral rails with accent fills, and always include numeric labels so the chart is readable without interpreting color alone.

## 6. Motion

Motion is functional only. Use 120ms for button feedback and 180-220ms for dialog or drawer transitions. Animate opacity and transform. Respect `prefers-reduced-motion`.

## 7. Surface Depth

Prefer tonal contrast and borders over heavy shadows. Do not nest cards inside cards. Forms in dialogs or drawers may be framed; page sections should remain direct working surfaces.
