# Implementation Tasks

- [ ] In `TaskCard.tsx`: add `original_prompt?: string` and `description?: string` to the `SpecTaskWithExtras` interface
- [ ] In `TaskCard.tsx`: wrap the task name `<Typography>` with a MUI `<Tooltip>` whose title is `description || original_prompt || name`, using `whiteSpace: "pre-wrap"` to preserve newlines
- [ ] In `TabsView.tsx` `PanelTab`: update the `tooltipContent` memo so the no-session / no-title-history branch uses `description || original_prompt || name` and wraps the text with `whiteSpace: "pre-wrap"` to preserve newlines
