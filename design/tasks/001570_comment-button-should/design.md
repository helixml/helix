# Design: Comment Button Spinner

## Context

The comment submit button lives in `InlineCommentForm.tsx`. It currently only has a `disabled` guard for empty text. The parent `DesignReviewContent.tsx` calls `createCommentMutation.mutateAsync()` in its `handleCreateComment` handler and passes `handleCreateComment` as the `onCreate` prop. The mutation's `isPending` state is never forwarded to the form.

## Key Files

- `frontend/src/components/spec-tasks/InlineCommentForm.tsx` — button is at line 126-133
- `frontend/src/components/spec-tasks/DesignReviewContent.tsx` — `handleCreateComment` at ~line 813, `createCommentMutation` from `useCreateComment()` hook
- `frontend/src/services/designReviewService.ts` — `useCreateComment` returns a React Query mutation with `.isPending`

## Pattern Used in Codebase

`SpecTaskActionButtons.tsx` demonstrates the standard approach:

```tsx
<Button
  disabled={mutation.isPending}
  startIcon={
    mutation.isPending
      ? <CircularProgress size={16} color="inherit" />
      : undefined
  }
>
  Comment
</Button>
```

## Approach

1. Add an `isSubmitting?: boolean` prop to `InlineCommentFormProps`.
2. In `DesignReviewContent.tsx`, pass `isSubmitting={createCommentMutation.isPending}` to `<InlineCommentForm>`.
3. In `InlineCommentForm.tsx`:
   - Import `CircularProgress` from `@mui/material`.
   - Set `disabled={!commentText.trim() || isSubmitting}` on the button.
   - Add `startIcon={isSubmitting ? <CircularProgress size={16} color="inherit" /> : undefined}`.

No new hooks, utilities, or abstractions needed — this is a two-file change.
