# Polish project board: replace skip planning toggle with text link

## Summary
The backlog task card had an awkward UX: a "Skip planning" toggle switch sat directly under a "Start Planning" button, creating contradictory labels. This replaces it with a subtle "or skip to implementation" text link that keeps planning as the visually dominant action. Also removes the "Skip Planning" button from tasks already in the planning phase.

## Changes
- Replace the `Switch` toggle with a clickable `Typography` text link: "or skip to implementation"
- Primary button always shows "Start Planning" in yellow — no more color/label swap based on toggle state
- Remove the "Skip Planning" button from the spec_generation phase entirely
- Clean up unused imports (`FormControlLabel`, `Switch`, `SkipNext`, `useSkipSpec`)

## Screenshots
![Backlog card with new text link](https://github.com/helixml/helix/raw/helix-specs/design/tasks/001886_ui-revamp-the-ui-on-the/screenshots/01-backlog-card-after.png)
![Planning column with no skip button](https://github.com/helixml/helix/raw/helix-specs/design/tasks/001886_ui-revamp-the-ui-on-the/screenshots/02-planning-no-skip-button.png)
