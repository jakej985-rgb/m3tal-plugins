# Palette's Journal

## 2026-11-05 - Container Details Expand Accessibility & Focus Indicators
**Learning:** In micro-frontend plugin UIs that render dynamic table rows, icon-only disclosure buttons (e.g., `▶`) must provide dynamic `aria-label` and `aria-expanded` attributes to avoid being unannounced or ambiguous to assistive tech. Additionally, standardizing `:focus-visible` outlines using the system cyan accent token ensures seamless keyboard navigation without polluting mouse clicks.
**Action:** When adding or auditing expandable table rows across plugin dashboards, ensure disclosure buttons specify descriptive `aria-label` including the entity name, keep `aria-expanded` in sync in the toggle handler, and verify `.btn:focus-visible` / `.btn-expand:focus-visible` styles are declared.

## 2026-11-05 - WAI-ARIA Tab Navigation Pattern in Micro-Frontend Dashboards
**Learning:** Monorepo plugin UIs frequently implement tabbed navigation panels (`.tabs` and `.tab`) using plain unstyled `<div>` tags without keyboard focusability or accessibility semantics. Screen reader users cannot identify available panels or selected states, and keyboard users cannot switch tabs using arrow keys.
**Action:** Standardize plugin dashboard tab bars with `role="tablist"` + `role="tab"` + `role="tabpanel"`, bind `aria-selected` and `tabindex` (roving tabindex `0` for active tab, `-1` for inactive), and attach an `ArrowLeft` / `ArrowRight` / `Home` / `End` keydown handler to allow rapid keyboard-driven tab switching across panels.
