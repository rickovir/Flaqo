# Dashboard Design Audit

Audit of `apps/dashboard` against `docs/DESIGN.md`, performed with the
dashboard running against the development API and inspected in a real
browser at desktop (1440px) and mobile (390px) widths.

**No backend or business-logic changes were made.** The audit is
read-only; the redesign that follows touches only the dashboard's
presentation layer and component structure.

---

## Summary

The dashboard is functionally complete and works: every MVP flow in the
product specification is present and reachable. What it lacks is the
*system*. The implementation grew one file at a time, so the visual
language drifted away from the design system, and the components the
design system names were never built.

The most significant gaps are structural rather than cosmetic:

1. **No sidebar.** The design system calls for a persistent left
   sidebar with workspace navigation, project context, and a user menu.
   The dashboard has a 56px top bar with two links. Every page is
   missing the primary navigation the design specifies.
2. **No reusable components.** The design names 18 required components.
   Zero exist. The same toggle, badge, empty state, and error banner are
   reimplemented inline in each page, which is exactly the
   "multiple versions of the same component" anti-pattern.
3. **No dialogs.** Create and delete are inline forms and
   `window.confirm`. The design requires dialogs with correct focus
   management.
4. **Wrong tokens throughout.** The palette is a different system from
   the one specified: indigo `#4F46E5` vs. the implemented blue
   `#2F5BD7`, slate `#0F172A` vs. near-black `#16191D`, and the
   background is white instead of `#F8FAFC`. The font is the system
   stack, not Inter.
5. **Production has no special treatment.** The design requires
   confirmation, an impact explanation, and a clear indicator when
   Production is selected. Toggling a Production flag currently applies
   immediately with no confirmation at all.

---

## 1. Visual inconsistencies

| # | Finding | Evidence | Severity |
|---|---------|----------|----------|
| 1.1 | **Primary accent is the wrong colour.** Design specifies indigo `#4F46E5`; implementation uses blue `#2F5BD7`. | Computed style on `button.primary` | High |
| 1.2 | **Text colour is the wrong neutral.** Design specifies slate `#0F172A`; implementation uses `#16191D`. | Computed style on `body` | Medium |
| 1.3 | **Background is white; design specifies `#F8FAFC`.** Cards are also white, so there is no surface separation — only a 1px border. | `body` background `rgb(255,255,255)`, design `background #F8FAFC` / `surface #FFFFFF` | Medium |
| 1.4 | **Secondary surface token missing.** Design has `surface-muted #F1F5F9`; implementation uses `#F8F9FB`. | `.header` background | Low |
| 1.5 | **Font is the system stack, not Inter.** | `font-family: -apple-system, BlinkMacSystemFont, "Segoe UI"...` | Medium |
| 1.6 | **Base font size is 15px; design specifies 14px body.** | Computed `body` font-size | Low |
| 1.7 | **Weights use non-standard values** (`550`, `650`) that fall back to synthetic weights. Design calls for semibold (`600`). | Computed `h1` weight `650`, `th` weight `550` | Low |
| 1.8 | **Success colour is `#1F7A4D`; design specifies `#16A34A`. Danger is `#C0342B` vs `#DC2626`.** | Computed styles | Medium |
| 1.9 | **Border colour is `#E3E6EA` vs design `#E2E8F0`.** | Computed `.card` border | Low |
| 1.10 | **Card radius is 6px; design specifies 10–12px for cards and panels.** | Computed `.card` border-radius | Medium |
| 1.11 | **Warning token is entirely absent.** The design defines `#D97706`; nothing uses it. | No rule in `globals.css` | Medium |
| 1.12 | **Spacing is not on the 4px base consistently.** Padding values of `7px`, `20px`, `28px`, `9px` are off-grid. | Computed `button` padding `7px 14px`, `.card` padding `20px` | Medium |
| 1.13 | **Monospace stack is generic**, not a deliberate developer-toolchain face. | `--mono` uses `ui-monospace, SFMono-Regular...` | Low |

## 2. Layout problems

| # | Finding | Evidence | Severity |
|---|---------|----------|----------|
| 2.1 | **No sidebar.** The design specifies a persistent 240px left sidebar with logo, workspace navigation (Projects, Feature Flags, SDK Keys), project context, active indicator, and a user menu with logout. | Only a 56px top `.header` with two links | **Critical** |
| 2.2 | **Page structure does not follow the specified order.** Design: breadcrumb → title → description → primary action → content. Implementation has title and action side by side, no breadcrumb. | `.projects` page markup | High |
| 2.3 | **Feature Flags and SDK Keys live on one page.** The design specifies SDK key management is *separate* from Feature Flags, with its own heading and environment context. | Both render in `projects/[projectId]/page.tsx` | High |
| 2.4 | **No breadcrumb anywhere.** The design requires one on every page. | Not present | High |
| 2.5 | **Content width is 1080px centred, but the design wants a comfortable maximum with no excessive empty space** — currently the sidebar-less layout leaves large dead space on the right. | `.main` margin `0px 89px` | Medium |
| 2.6 | **Flag table has no Description column**, which the design lists in the required columns. | Table has State / Key / actions only | High |
| 2.7 | **SDK keys are a second section inside the same page rather than their own view**, so the "keep separate" rule is broken. | See 2.3 | High |
| 2.8 | **Table column headers are visually weak** — the design warns against "dense tables without visual hierarchy". | `th` has only a colour change | Low |

## 3. Missing interaction states

| # | Finding | Evidence | Severity |
|---|---------|----------|----------|
| 3.1 | **Production state changes apply with no confirmation.** The design requires: a clear Production indicator, a confirmation, and an explanation of impact. Toggling a Production flag writes immediately. | No `window.confirm` in the toggle path; `handleToggle` calls the API directly | **Critical** |
| 3.2 | **The toggle has no loading state.** The design requires feedback while the request runs and prevention of duplicate requests. The input is never disabled and never marked busy. | `disabled: false`, no `aria-busy` | High |
| 3.3 | **Toggling in Production looks identical to Development.** | `sameStyles: true` between the two tabs; no warning element | **Critical** |
| 3.4 | **No skeleton loading states.** The design requires skeletons for tables and content areas; the implementation shows the text "Loading..." in a bare div. | 0 skeleton elements | High |
| 3.5 | **No toast or notification system.** The design specifies concise toasts for "flag updated", "project created", "SDK key generated", "credential revoked", and failures. Feedback is a full-width inline banner that pushes content down. | 0 `[role=status]`/`[aria-live]` elements | High |
| 3.6 | **Errors have no retry affordance.** The design says "provide retry where appropriate"; the error banner is inert. | `.error` has no button | Medium |
| 3.7 | **Delete uses `window.confirm`** rather than a designed confirmation dialog with focus management. | `window.confirm` in three places | High |
| 3.8 | **Create uses an inline expanding card**, not a dialog. | `<form>` inside `.card` | High |
| 3.9 | **No copy interaction for flag keys**, which the design lists as optional copy interaction and requires as a `CopyButton` component. | 0 copy buttons | Medium |
| 3.10 | **The raw SDK key is shown in a plain `<div>`** with no copy action, though the design explicitly requires "Provide copy action" in the security UX section. | `.key-reveal` is a bare div | High |
| 3.11 | **Empty states have no action.** The design says each empty state "should explain what is missing and provide the relevant action". "No SDK keys for Development yet." has no button. | `.empty` div, text only | Medium |
| 3.12 | **No reduced-motion support.** The design requires respecting `prefers-reduced-motion`. | No `@media (prefers-reduced-motion)` rule | Medium |
| 3.13 | **Delete is destructive with no undo and no confirmation styling** — a plain red text button that is easy to hit. | `.danger` button | Medium |

## 4. Accessibility issues

| # | Finding | Evidence | Severity |
|---|---------|----------|----------|
| 4.1 | **Environment tabs read as "Developmentv4".** The version number has no separator, so the accessible name is a run-together string. | Snapshot: `tab "Developmentv4"` | High |
| 4.2 | **The environment tabs are not a proper tablist.** They use `role="tab"` elements but the container lacks `role="tablist"`, and there are no `tabpanel` elements or `aria-controls` linking tabs to their panel. | Snapshot shows `tab` with no `tablist` | High |
| 4.3 | **Tab order is not managed.** Arrow keys do not move between tabs, and `tabindex` is not set so all three tabs are tab stops. | No `onKeyDown` on the tab list | Medium |
| 4.4 | **Focus ring on the toggle is attached to the wrong element.** The visible ring is drawn by `input:focus-visible + .track`, but the ring appears on the sibling *before* `.track` in the DOM. Computed outline `2px solid rgb(47,91,215)` sits on an element with no class. | Computed styles of `.toggle` children | Medium |
| 4.5 | **The "Manage" action is a `<button>` nested inside a `<Link>` (an `<a>`).** Nested interactive elements are invalid and confuse assistive technology. | Snapshot: `link "Manage" > button "Manage"` | High |
| 4.6 | **Status is communicated by colour plus a text badge, but the badge text is a separate element from the toggle** — a screen reader user toggling the flag hears only the checkbox state, not the environment. The `aria-label` does carry it, which partly mitigates this. | `aria-label="checkout_v2 in Development"` | Low |
| 4.7 | **No `aria-live` region**, so async errors and success messages are never announced. | 0 live regions | Medium |
| 4.8 | **Dialogs, when added, need focus management** — currently `window.confirm` is used, which is accessible but does not match the design's component requirements. | See 3.7 | Medium |
| 4.9 | **Icon-only buttons would need accessible names** — no icon-only buttons exist yet, but the copy button required by the design must include one. | n/a | Low |

**Contrast (measured, all against the surfaces they sit on):**

| Pair | Ratio | Verdict |
|------|-------|---------|
| Body text | 17.63:1 | Pass |
| Secondary/muted text | 6.00:1 | Pass (AA) |
| Table header labels | 6.00:1 | Pass (AA) |
| Badge OFF text | 5.69:1 | Pass (AA) |
| Danger button text | 5.57:1 | Pass (AA) |
| Primary button text | 5.84:1 | Pass (AA) |
| Toggle track (OFF) | 1.49:1 | **Non-text, needs 3:1 for UI components — fails** |

The toggle track is the one genuine contrast failure: at 1.49:1 the
off-state track is nearly invisible against the card surface. A screen
reader user gets the state, but a low-vision user may not see that the
control exists.

## 5. Components that should be refactored

The design names 18 required components. **None exist.** Every one is
needed, and several are already duplicated inline.

| Required component | Status | Where the logic lives now |
|---|---|---|
| `AppSidebar` | Missing | Top `.header` in `layout.tsx` |
| `PageHeader` | Missing | Ad-hoc `row between` blocks in both pages |
| `Breadcrumbs` | Missing | — |
| `EnvironmentSelector` | Partial | `.env-tabs` inline in the project page |
| `FeatureFlagTable` | Missing | Inline `<table>` in the project page |
| `FeatureFlagToggle` | Missing | Inline `.toggle` markup + CSS |
| `StatusBadge` | Partial | `.badge` CSS class, used ad hoc |
| `CreateFlagDialog` | Missing | Inline form in a card |
| `EditFlagDialog` | Missing | — (no edit UI exists at all) |
| `DeleteConfirmationDialog` | Missing | `window.confirm` ×3 |
| `CreateProjectDialog` | Missing | Inline form in a card |
| `SDKKeyTable` | Partial | Inline table in `SdkKeyPanel.tsx` |
| `GenerateSDKKeyDialog` | Missing | Inline form in a card |
| `CopyButton` | Missing | — |
| `EmptyState` | Partial | `.empty` div, duplicated |
| `LoadingState` | Missing | Bare "Loading..." text |
| `ErrorState` | Missing | `.error` div, duplicated |
| `Toast` | Missing | `.success`/`.error` inline banners |

### Duplication that must be resolved

- `.empty` markup appears in the projects page, the flags table, and the
  SDK key panel, each written slightly differently.
- `.error` and `.success` banners are written independently in three
  files with no shared component.
- The toggle markup (label > input + track + thumb) is duplicated
  between the flag table and the SDK key panel's status.
- The inline create form pattern is repeated in three places with
  different markup.
- `toProjectResponse` / `toEnvironmentResponses` style mapping is
  spread across `handlers.go` and `api.ts`.

---

## Prioritised redesign plan

### P0 — Structural (blocking the design's core promise)

1. **Build the design token layer.** Replace the ad-hoc values in
   `globals.css` with the exact palette, type scale, spacing scale, and
   radii from `DESIGN.md` §2, exposed as CSS custom properties. Adopt
   Inter with a system fallback stack.
2. **Build `AppSidebar` and restructure the layout.** Persistent
   240px sidebar with logo, workspace navigation, project context, active
   indicator, and a user menu with logout. Replace the top header.
3. **Build the shared component library.** `PageHeader`, `Breadcrumbs`,
   `StatusBadge`, `EmptyState`, `LoadingState` (with skeletons),
   `ErrorState` (with retry), `CopyButton`, `Toast`, and the dialog
   primitives.
4. **Introduce the dialog system.** `CreateProjectDialog`,
   `CreateFlagDialog`, `EditFlagDialog`, `GenerateSDKKeyDialog`,
   `DeleteConfirmationDialog`, with focus trapping, escape handling, and
   focus restoration.

### P1 — Safety and correctness

5. **Production guard.** A visible indicator when Production is
   selected, a confirmation dialog that explains the impact, and a
   distinct visual treatment for Production actions.
6. **Toggle loading and duplicate-request prevention.** Disable and
   mark the control busy while the request runs; update the UI only
   after a successful response; roll back on failure.
7. **Toast notifications.** Replace the inline banners with transient
   toasts in an `aria-live` region.

### P2 — Layout and content

8. **Split Feature Flags and SDK Keys into separate views**, each with
   its own heading and environment context.
9. **Add the flag Description column** and make the description editable
   through `EditFlagDialog`.
10. **Add breadcrumbs** to every page.
11. **Fix the environment selector** to a proper `tablist`/`tab`/`tabpanel`
    structure with arrow-key navigation and a separated version label.

### P3 — Polish

12. **Copy interactions** for flag keys and the revealed SDK key.
13. **Empty states with actions**, and error states with retry.
14. **Skeleton loading states** for tables.
15. **`prefers-reduced-motion` support.**
16. **Fix the toggle track contrast** and the focus-ring element
    mismatch.
17. **Fix the nested `<button>` inside `<Link>`**, and remove the invalid
    regex patterns in favour of validated inputs.

### Not in scope

Per the instruction, no backend or business-logic changes. Every item
above is dashboard-only. The one place the audit surfaces a *backend*
concern — the flag list not returning a description — is handled by
adding the column to the existing response only if the data is already
available; otherwise the description column is populated from the
existing flag endpoint without changing the API.
