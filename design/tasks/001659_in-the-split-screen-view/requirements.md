# Requirements: Split-Screen Tab Independence

## Problem

In the split-screen (TabsView) workspace, when two spec tasks are visible simultaneously and a user changes the tab (e.g. "file view" or "details") on one of them, **all visible spec tasks switch to the same tab**. Each panel should control its own view independently.

## Root Cause

`SpecTaskDetailContent` syncs `currentView` state to the global URL query param `view` via:
- `router.mergeParams({ view: newView })` on change
- A `useEffect` that watches `router.params.view` and updates local state when the URL param changes

When two instances are mounted simultaneously, they share the same URL param, so a change in one propagates to all others.

## User Stories

- **As a user with two spec tasks in split-screen**, I want to set one to "details" tab and the other to "file view" tab, so I can compare them side-by-side without them interfering.
- **As a user**, I want the tab I select in one panel to stay on that panel only, so the split-screen layout remains useful.

## Acceptance Criteria

1. Changing the view tab (desktop/changes/details/chat) in one split-screen panel does NOT affect tabs in other panels.
2. Each `SpecTaskDetailContent` instance maintains its own independent `currentView` state.
3. Single-panel usage (non-split-screen) continues to work as before, including URL param sync if applicable.
4. Navigating directly to a URL with `?view=details` still initializes the view correctly when there is only one task visible.
