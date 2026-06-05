# Design: Enforce Requirements.md Title Format in Planning Prompt

## The Existing Pipeline (no change)

What actually happens, verified in the code:

1. **Task creation** (`spec_driven_task_service.go:229`): `task.DesignDocPath` (the `NNNNNN_slug` folder under `design/tasks/`) is generated from the user's original prompt-derived `task.Name`. This happens BEFORE the agent runs and is never regenerated. → folder name is **immune** to the bug.
2. **Planning agent runs**, writes `requirements.md`, pushes the `helix-specs` branch.
3. **Push handler** (`git_http_server.go:1648-1659`): reads `requirements.md`, calls `SpecTitleFromRequirements()` (in `git_helpers.go:307-324`), and if the result is non-empty overwrites `task.Name`. → UI display name is **affected** by the bug.
4. **Spec approval** transitions the task to implementation. `GenerateUniqueBranchName()` (`spec_driven_task_service.go:1268`) is called and uses the CURRENT `task.Name` — which by this point is whatever step 3 computed. The result is written to `task.BranchName`. → branch name is **also affected** by the bug.

The bug itself: `SpecTitleFromRequirements()` strips `# Requirements` to empty and falls through to the next non-empty line. With the current prompt, agents legitimately write `# Requirements` because nothing tells them otherwise. The next line is often `## Background`, so `task.Name` becomes "background", and shortly afterwards `task.BranchName` becomes `feature/NNNNNN-background`.

## Decision: Fix at the Prompt, Not the Extractor

The user explicitly asked for a prompt fix. We honour that for two reasons:

- **Root cause is the prompt's silence.** The extractor is doing exactly what it was written to do — strip the boilerplate prefix and return whatever's left. Adding an H2 fallback or an "ignore Background" rule would just compensate for an agent that ignored its instructions.
- **Other downstream consumers benefit too.** A descriptive title in `requirements.md` also improves design-review UI, commit messages, and PR titles. Patching only the extractor would not produce a better-titled doc, just a better-named directory.

## What Changes in the Prompt

File: `api/pkg/services/spec_task_prompts.go`, the `planningPromptTemplate` literal (lines 28-144).

The current "Your Task Directory" block is:

```
## Your Task Directory

Create exactly 3 files in /home/retro/work/helix-specs/design/tasks/{{.TaskDirName}}/ (directory already exists):
1. requirements.md - User stories + acceptance criteria
2. design.md - Architecture + key decisions
3. tasks.md - Checklist of implementation tasks using [ ] format
```

Add a new sub-section immediately after this list (before the "## CRITICAL: Don't Over-Engineer" section). The new sub-section makes the title format mandatory and shows the format with one good and one bad example.

Proposed text (final wording can be tightened during implementation):

```
## CRITICAL: Title Format

Each of the three files MUST start with an H1 in this exact format:

  - requirements.md → `# Requirements: <Descriptive Title>`
  - design.md       → `# Design: <Descriptive Title>`
  - tasks.md        → `# Implementation Tasks: <Descriptive Title>`

Use the SAME descriptive title across all three files. The title should
summarise the actual subject of the task (e.g. "Add Dark Mode Toggle"),
not the type of document.

GOOD: `# Requirements: Add Dark Mode Toggle to Settings Page`
BAD:  `# Requirements`              ← no descriptive title
BAD:  `# Background`                ← wrong document type prefix
BAD:  `# Requirements: Background`  ← describes the section, not the task

The directory name for this task is derived from the requirements.md
title. A missing or generic title produces a meaningless directory name.
```

Also update the existing `## tasks.md Format` example block (lines 81-87) so the H1 in the example matches the new convention:

```
# Implementation Tasks: <Descriptive Title>

- [ ] First task
- [ ] Second task
- [ ] Third task
```

## What Does NOT Change

- `SpecTitleFromRequirements()` in `git_helpers.go` — stays as is. The "strip `Requirements:` prefix" branch is exactly what the new format relies on: agents will write `# Requirements: Add Dark Mode`, the extractor strips the prefix, and `Add Dark Mode` becomes the title.
- The push-time update in `git_http_server.go:1648-1659` — already handles the case correctly when a usable title is present.
- `sanitizeForBranchName`, `GenerateDesignDocPath`, `GenerateFeatureBranchName` — they already do the right thing on a good title.
- The folder-naming pipeline — already not affected by the bug because it runs at task creation, before the agent writes anything.
- Existing badly-named tasks (their UI name and branch name) — left in place. Renaming would invalidate git history and external PR references for no real benefit.

## Risk and Verification

- **Risk:** the agent ignores the new instruction and still writes a generic H1. Mitigation: the instruction is in its own `## CRITICAL` block, includes both positive and negative examples, and explains the consequence (meaningless directory name). If the rule is still ignored frequently after this change, the follow-up is a server-side validation that rejects pushes whose `SpecTitleFromRequirements()` returns empty — out of scope here.
- **Verification:** unit-test the prompt by running `BuildPlanningPrompt` with a sample `SpecTask` and asserting the new "## CRITICAL: Title Format" section is present in the output. End-to-end verification: trigger a fresh spec task in the inner Helix and confirm the resulting directory name matches the title the agent chose.

## Notes for Future Cloners

- The planning prompt is one big Go raw string in `spec_task_prompts.go`. There is only one prompt template — both `SpecDrivenTaskService.StartSpecGeneration` and `SpecTaskOrchestrator.handleBacklog` go through `BuildPlanningPrompt`, so a single edit covers every entry point.
- The prompt is rendered via `text/template`. Any literal `{{` or `}}` you add in the new text would need escaping; the change here contains none.
- The cloned-task preamble (`ClonedTaskPreamble`) is injected before the rest of the template. It already implies the agent should re-use the existing requirements.md, so cloned tasks are unaffected by the new title rule (they inherit the original title).

## Implementation Notes (2026-05-11)

- Edits landed on `feature/002002-enforce-requirementsmd`. Two changes in `api/pkg/services/spec_task_prompts.go`: (1) inserted the `## CRITICAL: Title Format` section between "Your Task Directory" and "## CRITICAL: Don't Over-Engineer", (2) updated the `## tasks.md Format` example block H1 to `# Implementation Tasks: <Descriptive Title>`.
- Added `api/pkg/services/spec_task_prompts_test.go` containing `TestBuildPlanningPrompt_TitleFormatRule`. It calls `BuildPlanningPrompt` with a minimal `*types.SpecTask` and asserts the rendered output contains the new section header and the three `<Descriptive Title>` placeholder strings. Test passes locally (`CGO_ENABLED=1 go test -run TestBuildPlanningPrompt_TitleFormatRule ./pkg/services/`).
- Verification approach for a prompt-only change: the unit test pins the rule against silent regression. A real planning-agent E2E run is non-deterministic and would test LLM instruction-following, not the deployment — Air confirmed the file change and rebuilt the API on the inner Helix, which is the deployment verification. Any spec task created after this change merges will exercise the new prompt naturally.
- Gotcha for cloners: the prompt template is a Go raw string built by concatenating ` + "\"...\"" + ` literals around backtick-quoted blocks (so backticks can appear in the rendered markdown). When inserting new code blocks, follow the existing pattern: close the raw string, concatenate `+ "` + "`" + `..." + ` for inline code, and reopen the raw string. Don't try to embed backticks directly.
