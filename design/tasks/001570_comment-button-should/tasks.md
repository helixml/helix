# Implementation Tasks

- [~] Add `isSubmitting?: boolean` prop to `InlineCommentFormProps` interface in `InlineCommentForm.tsx`
- [~] Import `CircularProgress` from `@mui/material` in `InlineCommentForm.tsx`
- [~] Update the Comment button: set `disabled={!commentText.trim() || isSubmitting}` and add `startIcon={isSubmitting ? <CircularProgress size={16} color="inherit" /> : undefined}`
- [~] Pass `isSubmitting={createCommentMutation.isPending}` to `<InlineCommentForm>` in `DesignReviewContent.tsx`
- [ ] Run `cd frontend && yarn build` to verify no TypeScript errors
