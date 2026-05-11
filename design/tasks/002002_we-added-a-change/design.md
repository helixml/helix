# Design: Enforce Requirements.md Title Format in Planning Prompt

## The Existing Pipeline (no change)

1. Agent runs in planning phase, writes `requirements.md` into the task directory.
2. Agent pushes the `helix-specs` branch.
3. The push handler in `api/pkg/services/git_http_server.go:1648-1659` reads `requirements.md`, calls `SpecTitleFromRequirements()` (in `git_helpers.go:307-324`), and overwrites `task.Name` with the result if non-empty.
4. The task name is later sanitised by `sanitizeForBranchName()` and turned into the directory slug (`design_docs_helpers.go:24-70`).

The bug: `SpecTitleFromRequirements()` strips `# Requirements` to empty and falls through to the next non-empty line. With the current prompt, agents legitimately write `# Requirements` because nothing tells them otherwise. The next line is often `## Background`, so the task ends up named "background".

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
- The `sanitizeForBranchName` / `GenerateDesignDocPath` helpers — they already do the right thing on a good title.
- Existing badly-named directories — left in place. Renaming would invalidate links and git history for no real benefit.

## Risk and Verification

- **Risk:** the agent ignores the new instruction and still writes a generic H1. Mitigation: the instruction is in its own `## CRITICAL` block, includes both positive and negative examples, and explains the consequence (meaningless directory name). If the rule is still ignored frequently after this change, the follow-up is a server-side validation that rejects pushes whose `SpecTitleFromRequirements()` returns empty — out of scope here.
- **Verification:** unit-test the prompt by running `BuildPlanningPrompt` with a sample `SpecTask` and asserting the new "## CRITICAL: Title Format" section is present in the output. End-to-end verification: trigger a fresh spec task in the inner Helix and confirm the resulting directory name matches the title the agent chose.

## Notes for Future Cloners

- The planning prompt is one big Go raw string in `spec_task_prompts.go`. There is only one prompt template — both `SpecDrivenTaskService.StartSpecGeneration` and `SpecTaskOrchestrator.handleBacklog` go through `BuildPlanningPrompt`, so a single edit covers every entry point.
- The prompt is rendered via `text/template`. Any literal `{{` or `}}` you add in the new text would need escaping; the proposed text contains none.
- The cloned-task preamble (`ClonedTaskPreamble`) is injected before the rest of the template. It already implies the agent should re-use the existing requirements.md, so cloned tasks are unaffected by the new title rule (they inherit the original title).
