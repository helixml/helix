# Requirements: Fix Link Colors & Stale Highlight in Spec Review

## User Stories

- As a user reviewing specs in dark mode, I want links to be clearly readable so I don't strain to see dark blue/purple text on a dark grey background.
- As a user selecting text in the spec reviewer, I want old highlights to clear when I select new text, so I don't end up with multiple phantom-highlighted words.

## Acceptance Criteria

### Bug 1: Link colors
- [ ] Links (`<a>` tags) in spec review markdown are rendered in a readable color (teal `#00D5FF`) on dark backgrounds
- [ ] Links have a visible hover state (underline or color shift)
- [ ] Visited links do not fall back to browser-default dark purple
- [ ] Fix applies to both `DesignReviewContent.tsx` and `DesignDocPage.tsx`

### Bug 2: Stale text highlight
- [ ] Selecting new text clears any previous CSS Highlight API highlight
- [ ] Clicking without selecting text clears any existing highlight
- [ ] The highlight for the current selection still works correctly when the comment form is open
