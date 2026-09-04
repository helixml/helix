# Highlight changed files in the workspace tree

## Summary
Highlight files changed by the task in the diff viewer's workspace tree using native Git-status colors and indicators. Parent folders now also indicate when they contain changed files, and the decorations stay current through the existing workspace review polling.

## Testing
Ran the focused workspace file tree and file surface test suites: 11 tests passed. Verified the patch with `git diff --check`. The repository-wide TypeScript check reached two unrelated existing dependency errors in `useLightTheme.tsx` and `sampleProjectIcons.tsx`; the changed files introduced no TypeScript errors.
