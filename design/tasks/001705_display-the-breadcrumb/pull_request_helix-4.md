# Display breadcrumb in browser tab title

## Summary
Add dynamic browser tab titles that reflect the current page context (project name, task name) using the existing breadcrumb data.

## Changes
- Add `useDocumentTitle` hook in `frontend/src/hooks/useDocumentTitle.ts`
- Integrate hook into `Page.tsx` component to set document title from breadcrumbs
- Title format: `Task Name - Project Name - Helix` (most specific first)
- Truncation for titles over 60 characters

## Test Plan
- [ ] Navigate to a project page, verify tab shows "Project Name - Helix"
- [ ] Navigate to a task detail page, verify tab shows "Task Name - Project Name - Helix"
- [ ] Navigate between pages, verify title updates correctly
