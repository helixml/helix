# Requirements: Hide Git Metadata on Mobile in Spec Viewer

## User Story

As a mobile user viewing a spec/design review, I don't want the GitHub branch chip and commit timestamp cluttering the header because they overflow the screen and are unnecessary on mobile.

## Acceptance Criteria

- [ ] On mobile viewports, the git branch + commit hash `Chip` is hidden
- [ ] On mobile viewports, the pushed-at timestamp `Typography` is hidden
- [ ] On desktop viewports, both elements remain visible as before
- [ ] The share icon and comment log icon buttons remain visible on mobile
- [ ] No layout shift or overflow on mobile after hiding elements
