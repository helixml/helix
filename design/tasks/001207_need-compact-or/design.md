# Design: Compact Mobile Header for Project List Page

## Overview

Make the Projects page header responsive on mobile devices, particularly for the Repositories view which has three action buttons that overflow on narrow screens.

## Current Architecture

The header layout flows through these components:
1. `Projects.tsx` passes `topbarContent` (action buttons) to `Page`
2. `Page.tsx` renders breadcrumbs + topbarContent in `AppBar`
3. `AppBar.tsx` uses `Row`/`Cell` layout with flexbox

Current issue: The Cell containing children (topbarContent) has `flexShrink: 0`, preventing buttons from adapting to narrow screens.

## Solution: Responsive Button Group

Use MUI's responsive `sx` props to adapt the button layout on mobile:

### Option A: Icon-Only Buttons on Mobile (Recommended)
- On mobile (< sm breakpoint): Show icon-only buttons with tooltips
- On desktop: Show full buttons with text + icons
- Minimal code changes, uses existing MUI patterns

### Option B: Overflow Menu
- Collapse buttons into a "More" dropdown on mobile
- More complex, requires new component

**Decision: Option A** - simpler, maintains direct access to all actions.

## Implementation Approach

### 1. Create Responsive Button Wrapper Component
Location: `frontend/src/components/widgets/ResponsiveButton.tsx`

```tsx
// Shows full button on desktop, icon-only on mobile
<ResponsiveButton
  icon={<FolderSearch />}
  label="Connect & Browse"
  ...buttonProps
/>
```

### 2. Update Projects.tsx topbarContent
Replace the three repository buttons with `ResponsiveButton` components.

### 3. AppBar.tsx Cell Adjustment
Allow the children Cell to shrink when needed:
```tsx
<Cell grow end sx={{ minWidth: 0, flexShrink: 1 }}>
```

## Key Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Mobile breakpoint | 600px (sm) | Standard MUI breakpoint |
| Icon-only vs menu | Icon-only | Faster access, fewer taps |
| Implementation location | New widget | Reusable across pages |

## Dependencies

- MUI `useMediaQuery` or responsive `sx` props
- Existing Lucide icons already in use

## Risks

- **Low**: Icon-only buttons may be less discoverable → Mitigated by tooltips
- **Low**: Multiple icon buttons may still be tight → Group can scroll horizontally if needed