# Design System — Feature Flag Platform

## 1. Product Design Vision

Build a modern, minimal, developer-focused SaaS dashboard for managing feature flags.

The interface should communicate:
- Clarity
- Reliability
- Precision
- Simplicity
- Developer friendliness

The product is a focused feature flag management platform, NOT an experimentation or analytics platform.

### Design references

Use these products as visual inspiration:
- Linear: typography, spacing, hierarchy, and minimalism.
- Vercel: developer-focused simplicity and information architecture.
- LaunchDarkly: feature flag management interaction patterns.

Do not directly copy their interfaces.

### Design principles

1. Content first: prioritize useful information over decoration.
2. Minimal but not empty: use whitespace intentionally.
3. Consistency: the same interaction should look and behave consistently.
4. Clear feedback: every action must have a visible result.
5. Safety: potentially disruptive Production changes require deliberate interaction.
6. Progressive disclosure: expose advanced details only when needed.

---

## 2. Visual Identity

### Theme

- Light mode by default.
- Neutral surfaces.
- The Flaqo violet as the primary brand accent.
- Subtle borders.
- Minimal shadows.
- Avoid excessive gradients.
- Avoid unnecessary decorative illustrations.

### Brand

The product is **Flaqo**. The supplied brand artwork is used as-is:

| Asset | File | Use |
|---|---|---|
| Flag mark | `apps/dashboard/public/brand/flaqo-flag-128.png` | Sidebar and login lockup |
| Wordmark | `apps/dashboard/public/brand/flaqo-wordmark-art.png` | Beside the mark |
| App icon | `apps/dashboard/public/icon-192.png` | Favicon and manifest |

The flag artwork already carries the brand gradient. Do not place it on a
coloured tile, and do not add a background behind it — that mutes the
gradient and muddies the mark.

The gradient is reused for primary buttons, the enabled toggle, and the
avatar, so the interface and the mark share one palette.

### Color tokens

The brand colours were sampled from the supplied artwork rather than
chosen by eye:

| Brand role | Value | Sampled from |
|---|---|---|
| brand-navy | #060E28 | The "Flaqo" lettering |
| brand-deep | #1D1EAA | The flagpole |
| brand-mid | #3F39D6 | The flag body |
| primary | #5A4CEE | The flag body |
| brand-lilac | #7C7DFB | The top highlight of the flag |

| Token | Value | Usage |
|---|---|---|
| background | #F7F8FC | Application background |
| surface | #FFFFFF | Cards and panels |
| surface-muted | #F2F4FA | Secondary surfaces |
| primary | #5A4CEE | Primary actions |
| primary-hover | #4A3CD6 | Hover state |
| text-primary | #060E28 | Main text |
| text-secondary | #5B6480 | Supporting text |
| text-muted | #8B93AA | Muted text |
| border | #E4E7F0 | Borders and dividers |
| success | #12855C | Enabled and active |
| danger | #D02638 | Destructive actions |
| warning | #B45309 | Warnings |

Use semantic colors consistently.

Do not communicate status using color alone. Always include a readable label or accessible state.

### Typography

Primary font: Inter.

Typography scale:
- Page title: 24–28px, semibold.
- Section title: 16–18px, semibold.
- Card title: 14–16px, semibold.
- Body: 14px, regular.
- Supporting text: 12–13px.
- Metadata: 11–12px.

Use monospace typography for:
- Feature flag keys.
- API identifiers.
- SDK keys.
- Technical values.

### Spacing

Use a consistent 4px spacing base.

Recommended spacing:
- 4px: compact inline spacing.
- 8px: related elements.
- 12px: component internal spacing.
- 16px: standard component spacing.
- 24px: section spacing.
- 32px: major section spacing.
- 40–48px: page-level spacing.

### Shape and elevation

- Small controls: 6–8px radius.
- Cards and panels: 10–12px radius.
- Dialogs: 12–16px radius.
- Prefer borders over shadows.
- Use subtle shadows only for floating surfaces.

---

## 3. Application Layout

Use a persistent left sidebar and a main content area.

### Sidebar

Recommended width:
- Expanded: 240px.
- Collapsed: 64–72px.

Sidebar sections:

WORKSPACE
- Projects
- Feature Flags
- SDK Keys

PREFERENCES
- Settings

Include:
- Product logo and name.
- Current project context.
- Active navigation indicator.
- User/account menu.
- Logout action.

Keep the sidebar visually quiet.

### Main content

Each page should follow a consistent structure:

1. Breadcrumb.
2. Page title.
3. Supporting description.
4. Primary contextual action.
5. Main content.

Use a centered content container with a comfortable maximum width.

Avoid excessive full-width empty space.

---

## 4. Page Specifications

### 4.1 Projects

Purpose: discover and manage projects.

Components:
- Page heading.
- Create Project button.
- Project list or card grid.
- Project name.
- Project description.
- Relevant metadata.
- Empty state.

Interactions:
- Create project.
- Open project.
- Edit project.
- Delete project with confirmation.

Keep project management straightforward.

### 4.2 Feature Flags

This is the primary product experience.

Header:
- Breadcrumb.
- Feature Flags title.
- Supporting description.
- New Flag action.

Environment selector:
- Development
- Staging
- Production

The selected environment controls which flag states are displayed and modified.

Feature flag table columns:
- Flag key
- Description
- State
- Actions

Use a clean table rather than excessive cards.

Feature flag key:
- Monospace.
- Easy to scan.
- Optional copy interaction.

State:
- Clear ON/OFF label.
- Accessible toggle.
- Consistent success and neutral states.

Interactions:
- Create flag.
- Edit flag.
- Delete flag.
- Toggle flag.
- Confirm destructive operations.

Production-specific behavior:
- Show clear indication when Production is selected.
- Require confirmation for state changes.
- Explain the impact of the change.
- Never silently apply a potentially disruptive action.

Do not introduce targeting, segments, percentage rollout, experimentation, or analytics.

### 4.3 SDK Keys

Keep SDK key management separate from Feature Flags.

Page content:
- SDK Keys heading.
- Environment context.
- Short explanation of credential scope.
- Generate Key action.
- Credentials table.

Table columns:
- Name
- Type
- Key identifier
- Status
- Actions

Security UX:
- Show the raw generated key only once.
- Provide copy action.
- Explain that the raw key cannot be retrieved later.
- Provide revoke confirmation.
- Never expose secret values in logs or browser storage.

### 4.4 Settings

Keep settings focused on the existing MVP.

Do not invent organization management, billing, analytics, or complex role management.

---

## 5. Core Components

Build reusable components with consistent behavior.

Required components:
- AppSidebar
- PageHeader
- Breadcrumbs
- EnvironmentSelector
- FeatureFlagTable
- FeatureFlagToggle
- StatusBadge
- CreateFlagDialog
- EditFlagDialog
- DeleteConfirmationDialog
- CreateProjectDialog
- SDKKeyTable
- GenerateSDKKeyDialog
- CopyButton
- EmptyState
- LoadingState
- ErrorState
- Toast / notification

Avoid creating multiple versions of the same component for different pages.

---

## 6. Interaction and Feedback

Every interactive component must have:
- Default state.
- Hover state.
- Focus state.
- Disabled state.
- Loading state where applicable.

### Feature flag toggle

- Clearly distinguish ON and OFF.
- Show loading feedback while the API request is running.
- Prevent duplicate requests.
- Update the UI only after successful API response, or use carefully implemented optimistic updates with rollback.
- Show a clear error if the operation fails.

### Forms

- Clear field labels.
- Helpful validation messages.
- Preserve user input when validation fails.
- Disable submit during submission.
- Show success feedback after completion.

### Dialogs

Use dialogs for:
- Create.
- Edit.
- Delete confirmation.
- SDK credential generation.
- Production state change confirmation.

Avoid unnecessary confirmation dialogs for harmless navigation.

### Notifications

Use concise toast messages.

Examples:
- Feature flag updated.
- Project created.
- SDK key generated.
- Credential revoked.
- Failed to update feature flag.

---

## 7. Loading, Empty, and Error States

Do not design only the happy path.

### Loading

- Use skeletons for tables and content areas.
- Avoid full-screen loading unless absolutely necessary.

### Empty

Examples:
- No projects yet.
- No feature flags in this project.
- No SDK keys for this environment.

Each empty state should explain what is missing and provide the relevant action.

### Error

- Explain what failed in plain language.
- Provide retry where appropriate.
- Avoid exposing stack traces or internal server errors.

---

## 8. Responsive Design

Desktop:
- Persistent sidebar.
- Full table layout.
- Comfortable content width.

Tablet:
- Collapsible sidebar.
- Adapt table spacing.

Mobile:
- Compact navigation.
- Horizontally scrollable tables when needed.
- Touch-friendly controls.
- Dialogs should fit viewport width.
- Avoid horizontal page overflow.

Do not sacrifice usability to fit every table column on mobile.

---

## 9. Accessibility

- Use semantic HTML.
- Support keyboard navigation.
- Provide visible focus indicators.
- Associate labels with inputs.
- Use accessible names for icon-only buttons.
- Ensure sufficient contrast.
- Toggles must expose their checked state.
- Dialogs must manage focus correctly.
- Do not rely on color alone to communicate status.

---

## 10. Motion

Use restrained motion.

- Short transitions for hover and focus.
- Subtle sidebar and dialog transitions.
- Avoid decorative animations.
- Respect reduced-motion preferences.

The interface should feel responsive, not animated for the sake of animation.

---

## 11. Design Anti-patterns

Avoid:
- Excessive cards inside cards.
- Too many colored badges.
- Unnecessary gradients.
- Large decorative icons.
- Excessive rounded containers.
- Oversized headings.
- Too many dashboard metrics.
- Unnecessary charts.
- Dense tables without visual hierarchy.
- Multiple competing primary buttons.
- Fake functionality.
- Placeholder interactions that do nothing.

---

## 12. Implementation Rules

1. Inspect the existing application before changing its design.
2. Reuse the existing stack and dependencies when possible.
3. Do not introduce a UI framework without justification.
4. Keep components reusable and maintainable.
5. Keep business logic in the Go API.
6. Use actual API data, not hardcoded production-looking data.
7. Keep frontend types aligned with backend contracts.
8. Maintain accessibility.
9. Test important user interactions.
10. Preserve existing working functionality.

### Design acceptance criteria

The implementation is successful when:

- All pages share a consistent visual language.
- Users can easily understand the selected project and environment.
- Feature flag states are immediately understandable.
- Production changes are handled carefully.
- SDK credential management is clearly separated.
- All actions provide appropriate feedback.
- Empty, loading, and error states are complete.
- The interface works on desktop and mobile.
- No MVP functionality is lost during redesign.
