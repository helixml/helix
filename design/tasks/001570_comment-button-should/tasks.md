# Implementation Tasks

- [x] Add `isSubmitting?: boolean` prop to `InlineCommentFormProps` interface in `InlineCommentForm.tsx`
- [x] Import `CircularProgress` from `@mui/material` in `InlineCommentForm.tsx`
- [x] Update the Comment button: set `disabled={!commentText.trim() || isSubmitting}` and add `startIcon={isSubmitting ? <CircularProgress size={16} color="inherit" /> : undefined}`
- [x] Pass `isSubmitting={createCommentMutation.isPending}` to `<InlineCommentForm>` in `DesignReviewContent.tsx`
- [~] Run `cd frontend && yarn build` to verify no TypeScript errors
