# Implementation Tasks

## DiffViewer.tsx - Mobile Layout

- [ ] Import `useIsBigScreen` hook from `../../hooks/useIsBigScreen`
- [ ] Add `isMobile` state using `useIsBigScreen({ breakpoint: 'sm' })`
- [ ] Add `mobileView` state: `useState<'files' | 'diff'>('files')`
- [ ] Add mobile view toggle UI (ToggleButtonGroup with Files/Diff buttons)
- [ ] Conditionally render file list OR diff content on mobile (not both)
- [ ] Auto-switch to 'diff' view when `handleSelectFile` is called on mobile
- [ ] Pass `onBack` callback to DiffContent on mobile to return to file list

## DiffContent.tsx - Responsive Styling

- [ ] Add `onBack` and `isMobile` props to interface
- [ ] Add "Back to files" button in header when `onBack` is provided
- [ ] Make line number column width responsive: `width: { xs: 32, sm: 44 }`
- [ ] Hide old line number column on mobile (show only new line number)
- [ ] Add text truncation to file path with ellipsis
- [ ] Ensure +/- indicator column doesn't shrink on narrow screens

## DiffFileList.tsx - Touch Targets

- [ ] Verify list item height is at least 44px for touch accessibility
- [ ] Increase `py` padding if needed on ListItemButton

## Testing

- [ ] Test on mobile viewport (375px width)
- [ ] Test file selection flow: tap file → view diff → tap back → file list
- [ ] Test desktop layout remains unchanged
- [ ] Verify line numbers display correctly on both layouts