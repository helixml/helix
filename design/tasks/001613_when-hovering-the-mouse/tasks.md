# Implementation Tasks

- [x] In `spec_driven_task_handlers.go`: remove the `if updateReq.Name != ""` block; instead, whenever `description` is updated, recalculate `name` using `generateTaskNameFromPrompt(updateReq.Description)`
- [x] In `BacklogTableView.tsx`: fix `handlePromptClick` to initialize the inline editor from `task.description` (not `task.original_prompt`), keeping `original_prompt` as a fallback for very old tasks
- [~] In `TaskCard.tsx`: add `description?: string` to the `SpecTaskWithExtras` interface
- [~] In `TaskCard.tsx`: wrap the task name `<Typography>` with a MUI `<Tooltip>` showing `task.description || task.name` with `whiteSpace: "pre-wrap"`
- [ ] In `TabsView.tsx` `PanelTab`: update the `tooltipContent` memo's no-session/no-history branch to show `displayTask.description || displayTask.name` with `whiteSpace: "pre-wrap"`
- [ ] In `GlobalNotifications.tsx`: wrap the text content `<Box>` in a MUI `<Tooltip>` showing the full `event.title` and `event.spec_task_name` with `whiteSpace: "pre-wrap"`, `placement="left"`
