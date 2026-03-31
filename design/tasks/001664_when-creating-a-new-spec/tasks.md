# Implementation Tasks

- [x] In `frontend/src/components/tasks/NewSpecTaskForm.tsx`, move `queryClient.invalidateQueries({ queryKey: ["spec-tasks"] })` to immediately after `v1SpecTasksFromPromptCreate` succeeds (before the label mutation loop)
- [x] Keep a second `invalidateQueries` call after the label loop completes so the list refreshes with label data too
- [x] Verify the task appears in the list within ~1 second after submission (with and without labels)
