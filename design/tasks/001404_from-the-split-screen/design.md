# Design: Back to Board View Navigation

## Overview

Add a prominent "Board View" button inside the Split Screen (TabsView) component that allows users to quickly switch back to the Kanban board view.

## Current Architecture

The `SpecTasksPage` component manages view mode state:
- `viewMode` state: `"kanban" | "workspace" | "audit"`
- Topbar contains small icon buttons to toggle between views
- `TabsView` component renders when `viewMode === "workspace"`
- `TabsView` does NOT currently receive a callback to change the view mode

## Solution

Pass a callback from `SpecTasksPage` to `TabsView` that allows it to request switching back to board view.

### Component Changes

**SpecTasksPage.tsx:**
- Add `onSwitchToBoard` prop when rendering `TabsView`
- Pass `() => setViewMode("kanban")` as the callback

**TabsView.tsx:**
- Add optional `onSwitchToBoard?: () => void` prop to `TabsViewProps`
- Render a "Board View" button in the empty state and/or as a persistent element
- Button positioned in top-left area for easy access

### UI Placement Options

**Recommended: Add to TaskPanel tab bar (left side)**
- Each TaskPanel already has a tab bar with action buttons on the right
- Add a "Board View" button on the left side of the first/primary panel's tab bar
- Uses existing UI real estate, no new chrome

### Visual Design

- Icon: `ViewKanban` (same as topbar) + "Board" text label
- Style: Text button, subtle but visible
- Position: Left side of tab bar, before the tabs

## Dependencies

- No new dependencies
- Uses existing MUI components and icons