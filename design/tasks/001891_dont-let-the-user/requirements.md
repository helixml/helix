# Requirements: Gate Spec Approval on Tab Viewing

## User Stories

1. **As a reviewer**, I want to be prevented from approving a spec until I've viewed all three tabs (Requirements, Technical Design, Implementation Plan), so that I don't accidentally approve without reading everything.

## Acceptance Criteria

- [ ] The "Approve Design" button is disabled until the user has clicked on all three tabs at least once during the current review session.
- [ ] A tooltip on the disabled button tells the user which tab(s) they haven't viewed yet.
- [ ] Once all tabs are viewed, the button enables (still subject to the existing "unresolved comments" check).
- [ ] The "Request Changes" and "Reject Design" buttons are NOT gated — a reviewer can reject or request changes without reading everything.
- [ ] Tab viewing state is per-session (component state) — no backend persistence needed.
- [ ] The initial tab ("requirements") counts as viewed on mount.
