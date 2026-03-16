# Implementation Tasks

## 1. Add Spec Documents Hook

- [ ] Add `useSpecTaskDocuments(taskId)` hook to `specTaskService.ts` that fetches requirements, design, and tasks docs in parallel using `v1SpecTasksDocumentsDetail`
- [ ] Return `{ requirements, design, tasks, isLoading }` object from the hook

## 2. Spec Tab in Right Panel

- [ ] Add `"spec"` to the `currentView` union type in `SpecTaskDetailContent.tsx`
- [ ] Add a Spec `<ToggleButton>` (with a `FileText` or `Description` icon) to the `ToggleButtonGroup` in the right panel header
- [ ] Update the default view logic: if `task.design_docs_pushed_at` is set, default `currentView` to `"spec"` instead of `"desktop"`
- [ ] Add `renderSpecContent()` function that renders:
  - **Original Request section**: styled block showing `task.description || task.original_prompt`
  - **Requirements accordion**: fetched requirements.md rendered as Markdown, expanded by default
  - **Design accordion**: fetched design.md rendered as Markdown, expanded by default
  - **Tasks accordion**: fetched tasks.md rendered as Markdown, expanded by default
- [ ] Show a loading skeleton while spec docs are being fetched
- [ ] Show a "No spec available yet" empty state if docs return 404/empty

## 3. Collapse System Prompt in Chat Thread

- [ ] Investigate whether `interaction.display_message` already excludes the system prompt (check backend behaviour)
- [ ] In `EmbeddedSessionView.tsx`, pass an `isFirstInteraction` boolean prop to the first `<Interaction>` component
- [ ] In `Interaction.tsx`, accept `isFirstInteraction` prop
- [ ] When `isFirstInteraction && interaction.system_prompt` is non-empty, wrap the system prompt content in a MUI `<Accordion defaultExpanded={false}>` labelled "System Prompt"
- [ ] Ensure the user's actual message (without the system prompt prefix) remains visible above or alongside the collapsed accordion

## 4. Mobile Layout

- [ ] Update mobile view toggle to include Spec option
- [ ] Ensure mobile defaults to `"spec"` view (instead of `"chat"`) when spec is available and task is not in an active running state

## 5. Testing & Polish

- [ ] Verify spec tab renders correctly for tasks in all states: backlog, spec_review, implementation, done
- [ ] Verify system prompt accordion works for tasks with and without `interaction.system_prompt` populated
- [ ] Verify no regression in Desktop / File Diff / Details / Chat views
- [ ] Check that `router.mergeParams({ view: "spec" })` correctly bookmarks the spec view in the URL
