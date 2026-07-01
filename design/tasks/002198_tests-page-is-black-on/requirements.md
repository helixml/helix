# Requirements: Fix Tests Tab Dark-on-Dark Colors in Light Mode (Agent Settings)

## Background

In the Agent settings editor (`pages/App.tsx`), the **Tests** tab renders
`components/app/TestsEditor.tsx`. That component hardcodes dark background hex
colors (`#2a2d3e`, `#1e1e2f`, `#0d1117`) and `color: 'white'` on several
elements. These do not adapt to the active MUI theme.

When the app is in **light mode**, MUI renders text with the light-theme text
color (near-black), but the hardcoded card/panel backgrounds stay dark. The
result is dark text on dark panels ("black on black") in the CLI-instructions
box, the CI/CD accordions, and the per-test/per-step cards — the content is
effectively unreadable.

## User Stories

### Story 1: Readable Tests tab in light mode
As a Helix user who uses light mode, I want the Tests tab in agent settings to
use light-theme colors so I can read the test cards, CLI instructions, and
CI/CD examples.

**Acceptance criteria:**
- [ ] In light mode, all text on the Tests tab has sufficient contrast against
      its background (no dark-on-dark).
- [ ] Test cards, step cards, the "Running Tests with CLI" box, and the code
      snippet boxes use light-appropriate backgrounds in light mode.
- [ ] The copy-command / copy-config icon buttons are visible in light mode
      (currently `color: 'white'`).

### Story 2: No regression in dark mode
As a Helix user who uses dark mode, I want the Tests tab to look the same as it
does today.

**Acceptance criteria:**
- [ ] In dark mode, the Tests tab retains its current dark appearance (panels,
      code snippets, and icons remain legible and visually consistent with the
      rest of the dark UI).

## Out of Scope
- No changes to test functionality (add/delete tests and steps, copy commands).
- No changes to other agent-settings tabs.
- No new theming infrastructure — reuse the existing `useLightTheme` hook.
