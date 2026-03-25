# Implementation Tasks

- [ ] In `TaskCard.tsx`: add `original_prompt?: string` and `description?: string` to the `SpecTaskWithExtras` interface
- [ ] In `TaskCard.tsx`: wrap the task name `<Typography>` with a MUI `<Tooltip>` showing `description || original_prompt || name`, using `whiteSpace: "pre-wrap"` to preserve newlines
- [ ] In `TabsView.tsx` `PanelTab`: update the `tooltipContent` memo's no-session/no-history branch to use `description || original_prompt || name` with `whiteSpace: "pre-wrap"`
- [ ] In `GlobalNotifications.tsx`: wrap the text content `<Box>` (containing the event title and task-name subtitle) in a MUI `<Tooltip>` showing the full `event.title` and `event.spec_task_name`, with `whiteSpace: "pre-wrap"`
