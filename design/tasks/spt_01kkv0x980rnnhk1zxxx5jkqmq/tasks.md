# Implementation Tasks

- [ ] Create `frontend/src/components/tasks/TaskSpecPanel.tsx` — renders `task.original_prompt` as a styled quote block and spec tabs (Requirements / Design / Tasks) from `task.requirements_spec`, `task.technical_design`, `task.implementation_plan` using ReactMarkdown + remarkGfm; shows "Spec not yet generated" when fields are empty
- [ ] In `Interaction.tsx`, add optional `isFirstInteraction` prop; when true and `userMessage.length > 500`, render a collapsed placeholder `"System prompt [Show ▼]"` with a `useState` toggle to expand the full bubble
- [ ] In `EmbeddedSessionView.tsx`, pass `isFirstInteraction={index === 0}` to each `<Interaction>`
- [ ] In `SpecTaskDetailContent.tsx` desktop split-screen layout: swap left panel to render `<TaskSpecPanel task={task} />` (replacing `EmbeddedSessionView`) and expand right panel to include a Chat tab that contains the current left-panel `EmbeddedSessionView` + prompt input
- [ ] Update the right panel `ToggleButtonGroup` to include a "Chat" tab icon; set "Chat" as the default `currentView` when a session becomes active (currently defaults to "details" then switches — keep same logic but for the right panel)
- [ ] Verify the no-session and mobile paths still work correctly (single panel, tab toggles) — no changes needed there, just regression-check
