# Implementation Tasks

- [ ] Create `ResponsiveButton` component in `frontend/src/components/widgets/ResponsiveButton.tsx`
  - Accept `icon`, `label`, and standard Button props
  - Use MUI `sx` responsive props to show/hide text based on breakpoint
  - Wrap icon-only variant with `Tooltip` showing the label

- [ ] Update `frontend/src/pages/Projects.tsx` repositories view topbarContent
  - Replace the three `Button` components with `ResponsiveButton`
  - Ensure icons are consistent: `FolderSearch`, `Link`, `Plus`

- [ ] Update `frontend/src/components/system/AppBar.tsx` children Cell
  - Change `flexShrink: 0` to `flexShrink: 1` on the children Cell
  - Add `overflow: 'hidden'` to prevent content overflow

- [ ] Add horizontal scroll fallback to AppBar children container
  - Add `overflowX: 'auto'` for cases where buttons still need scrolling
  - Hide scrollbar with CSS for cleaner appearance

- [ ] Test on mobile viewport sizes (320px, 375px, 414px)
  - Verify all buttons are accessible
  - Verify tooltips appear on icon-only buttons
  - Verify no horizontal page overflow

- [ ] Verify desktop layout remains unchanged