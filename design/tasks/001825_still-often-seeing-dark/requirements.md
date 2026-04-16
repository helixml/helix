# Requirements

## User Story

As a user reviewing spec documents in the Helix spec editor, I need links to be clearly readable so I can see and click them without squinting at dark blue text on a dark grey background.

## Problem

Links in the spec review (`DesignReviewContent.tsx`) and public design doc page (`DesignDocPage.tsx`) render in the browser's default dark blue (`#0000EE`) because no explicit link color is set. On the dark MUI `background.paper` (`~#121212`), this produces a contrast ratio of ~1.4:1 — far below the WCAG AA minimum of 4.5:1.

Other parts of the app handle this correctly:
- `CodeIntelligenceTab.tsx` uses `#00d5ff` (teal) — excellent contrast on dark
- `Markdown.tsx` (session chat) uses `#bbb` in dark mode

## Acceptance Criteria

- [ ] Links in the spec review markdown body are clearly readable on both dark and light backgrounds
- [ ] Link color is consistent with the rest of the Helix app (teal `#00d5ff` preferred)
- [ ] Hover state provides visual feedback (underline)
- [ ] Visited links don't revert to the browser's default purple
- [ ] The public design doc page (`DesignDocPage.tsx`) also has readable link colors
