# Requirements: Fix Link Colors in Spec Review Dark Mode

## User Stories

- As a user reviewing specs in dark mode, I want links to be clearly readable so I don't strain to see dark blue/purple text on a dark grey background.

## Acceptance Criteria

- [ ] Links (`<a>` tags) in the spec review markdown content are rendered in a light, readable color (e.g., the theme's teal `#00D5FF`) on dark backgrounds
- [ ] Links have a visible hover state (underline or color shift)
- [ ] Visited links do not fall back to browser-default dark purple
- [ ] Fix applies to both `DesignReviewContent.tsx` (main spec review) and `DesignDocPage.tsx` (public share page)
- [ ] Light mode link colors remain reasonable (not broken by the fix)
