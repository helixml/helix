# Requirements: Mobile-Friendly Spec Review Page Tabs

## User Story

As a user reviewing spec documents on a mobile device, I want to be able to see and interact with the document tabs (Requirements, Technical Design, Implementation Plan) without them being pushed off-screen by secondary information.

## Problem

On narrow screens (mobile), the spec review page header contains:
- **Left side**: Document tabs (Requirements, Technical Design, Implementation Plan)
- **Right side**: Git branch/commit chip, timestamp, share button, comment button

The right-side elements take up too much horizontal space, causing the tabs to be cut off or invisible on mobile devices.

## Acceptance Criteria

1. **On narrow screens (mobile)**:
   - The git branch/commit chip is hidden
   - The timestamp is hidden
   - The tabs remain fully visible and functional
   - The share and comment buttons remain visible (they're small icons)

2. **On wider screens (desktop)**:
   - All elements remain visible as they are today
   - No visual changes to the existing desktop layout

3. **Breakpoint**: Use MUI's `sm` breakpoint (~600px) as the threshold