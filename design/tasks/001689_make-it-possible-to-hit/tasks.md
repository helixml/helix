# Implementation Tasks

- [ ] Add `useState(false)` for `createDialogOpen` in `SpecTaskDetailPage.tsx`
- [ ] Add Enter/Escape key handler `useEffect` (copy from `SpecTasksPage.tsx` lines ~380-433)
- [ ] Add plus icon button to `topbarContent` alongside the existing "Open in Split Screen" button
- [ ] Wrap the page children in a flex row `<Box>` to accommodate the slide-in form panel
- [ ] Add the `NewSpecTaskForm` slide-in panel (copy Box + conditional render from `SpecTasksPage.tsx`)
- [ ] Import `NewSpecTaskForm` and `Plus` (lucide-react) in `SpecTaskDetailPage.tsx`
- [ ] Test: Enter key opens panel, Enter/Escape closes it, plus button opens panel, task creation closes panel
