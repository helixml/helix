# Requirements: Fix Ugly Bright Blue Org Switcher Hover Color in Dark Mode

## Background

When dark mode was added to the Helix frontend, the org switcher (the popover that lists organizations and lets users switch between them) ended up with a hover background colour that's an aggressive bright cyan (`#4fc3f7`). In dark mode it reads as a glaring, almost neon highlight instead of the subtle hover affordance the rest of the dark UI uses.

The light-mode hover (teal `#0e7490`) doesn't have the same problem because it's tonally darker than the surrounding background.

## User Story

As a Helix user with dark mode enabled, when I hover over an organization in the org switcher popover I want a subtle, non-jarring highlight so the UI feels consistent with the rest of dark mode.

## Acceptance Criteria

- [ ] Hovering an org row in the org switcher popover in dark mode produces a subtle highlight that doesn't look "bright blue" or neon.
- [ ] The hover colour still provides clear visual feedback (user can tell which row will be clicked).
- [ ] Hover behaviour in **light mode** is unchanged (or at minimum, still looks correct).
- [ ] The "currently selected org" row stays visually distinct from the other rows on hover.
- [ ] Non-member org rows (which currently show `transparent` on hover) keep that behaviour.
- [ ] Manual verification: switch the OS to dark mode, open the org switcher, hover over each row — none of them should look out of place against the dark panel.

## Out of Scope

- Redesigning the org switcher layout.
- Changing the brand cyan (`#00E5FF`) used for borders / current-org indicators elsewhere in the component. Only the **hover background** is in scope.
- Changing hover colours in other components that happen to use `darkHighlight` — unless the fix is to retune that token, in which case the design doc will call this out.
