# Requirements: Compact Mobile Header for Project List Page

## Problem Statement

On mobile devices, the project list page header (topbar) with multiple action buttons causes horizontal overflow or awkward wrapping. The "Repositories" view is particularly affected with three buttons: "Connect & Browse", "Link Manually", and "New Empty".

## User Stories

### US-1: Mobile User Views Repositories Tab
**As a** mobile user  
**I want** the header buttons to be accessible without horizontal scrolling  
**So that** I can easily access all repository actions on my phone

### US-2: Mobile User Creates New Repository
**As a** mobile user  
**I want** to quickly access repository creation options  
**So that** I can add repositories without struggling with the UI

## Acceptance Criteria

1. **AC-1**: On mobile screens (< 600px), header action buttons must not cause horizontal page overflow
2. **AC-2**: All action buttons remain accessible and functional on mobile
3. **AC-3**: Desktop layout remains unchanged
4. **AC-4**: Touch targets remain appropriately sized for mobile interaction (min 44px)

## Affected Areas

- `frontend/src/pages/Projects.tsx` - topbarContent for repositories view
- `frontend/src/components/system/AppBar.tsx` - may need responsive adjustments
- Potentially `frontend/src/components/system/Page.tsx` - topbar layout

## Out of Scope

- Redesigning the entire navigation system
- Changes to non-mobile layouts
- Other pages beyond the Projects page