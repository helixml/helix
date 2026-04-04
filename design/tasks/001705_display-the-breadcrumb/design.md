# Design: Display Breadcrumb in Page Title

## Approach

Add a `useDocumentTitle` hook that sets `document.title` directly. Integrate it into the `Page` component so all pages using breadcrumbs automatically get dynamic titles.

## Key Decisions

**Why not react-helmet?**
- Extra dependency for a simple feature
- `document.title = x` works fine and is one line
- The codebase has no SSR, so no pre-rendering concerns

**Where to implement?**
- Create `useDocumentTitle` hook in `frontend/src/hooks/`
- Call it from `Page.tsx` using the existing `useBreadcrumbTitles` array
- This ensures any page using the `Page` component with breadcrumbs automatically gets a matching title

## Title Format

```
{last breadcrumb} - {second-to-last breadcrumb} - ... - Helix
```

Examples:
- `Fix login bug - My Project - Helix` (task detail)
- `My Project - Helix` (project page)
- `Projects - Helix` (projects list)
- `Helix` (home/no breadcrumbs)

## Truncation Strategy

If combined title > 60 chars:
1. Truncate middle breadcrumbs first (ellipsis)
2. Keep first and last breadcrumbs intact when possible
3. Last resort: truncate task name at ~40 chars

## Implementation Notes

- The `Page` component already computes `useBreadcrumbTitles` array
- Extract titles in reverse order: `titles.map(b => b.title).reverse().join(' - ')`
- Append ` - Helix` suffix
- Use `useEffect` cleanup to reset title on unmount (optional, depends on routing)
