# Requirements: Board View Split-Screen and Audit Trail Button Grouping

## Problem

The three view-mode toggle buttons (Board, Split-Screen, Audit Trail) currently sit on the right-hand side of the topbar in `SpecTasksPage`, alongside share/invite controls and other secondary actions. This makes them feel like utility controls rather than primary navigation, even though they're the main way users switch between the three distinct views of a project.

## User Stories

**As a user**, I want the Board / Split-Screen / Audit Trail toggle to be visually grouped and anchored to the left of the topbar — immediately after the breadcrumbs — so I understand these are the primary navigation choices within the page, not secondary actions.

## Acceptance Criteria

1. The three view-mode toggle buttons (Board, Split-Screen, Audit Trail) appear immediately to the right of the breadcrumbs in the topbar, separated from the right-side content (share button, etc.).
2. The buttons remain visually grouped (the existing `Stack` with active-state highlight styling is preserved).
3. The right-hand side of the topbar retains the share/invite controls and other secondary actions.
4. Responsive behaviour is unchanged: Split-Screen button is still hidden at `xs` breakpoints.
5. Active/inactive button styling (background highlight, primary colour icon) is unchanged.
6. No other pages are affected.
