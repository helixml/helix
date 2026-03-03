# Implementation Tasks

## DiffViewer.tsx - Mobile Layout

- [x] Import `useIsBigScreen` hook from `../../hooks/useIsBigScreen`
- [x] Add `isMobile` state using `useIsBigScreen({ breakpoint: 'sm' })`
- [x] Add `mobileView` state: `useState<'files' | 'diff'>('files')`
- [x] Add mobile view toggle UI (ToggleButtonGroup with Files/Diff buttons)
- [x] Conditionally render file list OR diff content on mobile (not both)
- [x] Auto-switch to 'diff' view when `handleSelectFile` is called on mobile
- [x] Pass `onBack` callback to DiffContent on mobile to return to file list

## DiffContent.tsx - Responsive Styling

- [x] Add `onBack` and `isMobile` props to interface
- [x] Add "Back to files" button in header when `onBack` is provided
- [x] Make line number column width responsive: `width: { xs: 32, sm: 44 }`
- [x] Hide old line number column on mobile (show only new line number)
- [x] Add text truncation to file path with ellipsis
- [x] Ensure +/- indicator column doesn't shrink on narrow screens

## DiffFileList.tsx - Touch Targets

- [x] Verify list item height is at least 44px for touch accessibility
- [x] Increase `py` padding if needed on ListItemButton

## Testing

- [ ] Test on mobile viewport (375px width)
- [ ] Test file selection flow: tap file → view diff → tap back → file list
- [ ] Test desktop layout remains unchanged
- [ ] Verify line numbers display correctly on both layouts