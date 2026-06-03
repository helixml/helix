# Requirements: Fix Ugly Bright Blue Org Switcher Hover Color in Dark Mode

## Background

When dark mode was added, the org-switcher popover (the dropdown listing the user's organisations, opened from the sidebar) inherited an unsuitable hover colour. Hovering over an organisation row paints the entire row a solid bright Material-Design cyan (`#4fc3f7`), which is visually jarring against the dark background (`#121214` / `#1e1e24`) and clashes with the rest of the dark theme.

## User Story

As a Helix user with dark mode enabled, when I hover over an organisation in the org-switcher popover, I want the hover indicator to be a subtle highlight that fits the dark theme, so that the UI does not feel broken or distracting.

## Acceptance Criteria

1. In dark mode, hovering over a non-selected, joined organisation in the `UserOrgSelector` popover shows a subtle hover background (translucent cyan, in line with the existing selected/hover treatments at `rgba(0, 229, 255, 0.1)` / `rgba(0, 229, 255, 0.15)`), not the solid bright `#4fc3f7`.
2. In light mode, the hover treatment continues to be visible and consistent with the rest of the light theme.
3. The selected-org background and selected-org hover styling are unchanged (still `rgba(0, 229, 255, 0.1)` resting and `rgba(0, 229, 255, 0.15)` hover).
4. Non-member rows (`org.member === false`) continue to have no hover background — only joined orgs get a hover indicator.
5. No other component that uses `useLightTheme().highlightColor` (e.g. `AddMcpSkillDialog`, `AddApiSkillDialog`, `AccessManagement`) is regressed.
