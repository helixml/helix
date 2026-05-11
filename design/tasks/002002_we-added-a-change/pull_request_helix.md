# Require descriptive titles in spec-task planning prompt

## Summary

Spec-task names in the UI are derived from the H1 of `requirements.md` (via `SpecTitleFromRequirements()` in `api/pkg/services/git_helpers.go`). Until now the planning prompt did not specify a title format, so an agent could legitimately write a bare `# Requirements` H1. That line strips to empty, the extractor falls through to the next non-empty heading, and the task name ends up as something like "background" (because the next H2 was `## Background`). The same name later flows into `task.BranchName` at spec-approval time (`spec_driven_task_service.go:1268`), so the bad title also produces a bad git branch name.

This PR fixes the issue at the prompt level. The planning prompt now mandates the format `# <DocType>: <Descriptive Title>` for `requirements.md`, `design.md`, and `tasks.md`, with a worked example, three counter-examples, and a sentence explaining the downstream consequence so the rule is unambiguous. No change to the extractor, the branch-naming pipeline, or the design-doc-folder pipeline (the folder name is already immune — it is generated at task creation, before the agent runs).

## Changes

- `api/pkg/services/spec_task_prompts.go`: insert a `## CRITICAL: Title Format` section in `planningPromptTemplate`; update the `## tasks.md Format` example H1 to match the new convention.
- `api/pkg/services/spec_task_prompts_test.go` (new): `TestBuildPlanningPrompt_TitleFormatRule` pins the new rule against silent regression by asserting the rendered prompt contains the section header and the three `# <DocType>: <Descriptive Title>` placeholders.

## Test plan

- [x] `CGO_ENABLED=1 go test -run TestBuildPlanningPrompt_TitleFormatRule ./pkg/services/` passes.
- [x] `CGO_ENABLED=0 go build ./...` clean.
- [x] Inner-Helix Air rebuilt and reloaded the new code.
- [ ] Reviewer: confirm the rendered prompt for the next real spec task contains the new `## CRITICAL: Title Format` block (any agent will see it on its next planning run).
