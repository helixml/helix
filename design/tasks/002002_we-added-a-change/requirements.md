# Requirements: Enforce Requirements.md Title Format in Planning Prompt

## Background

Spec tasks get a human-readable directory name (e.g. `001049_invalid-request-format`) derived from the title of the agent's `requirements.md`. The title is extracted by `SpecTitleFromRequirements()` in `api/pkg/services/git_helpers.go:307` — it walks the file line-by-line, strips leading `#` characters and the literal prefixes `Requirements:` / `Requirements`, and returns the first non-empty leftover.

If the agent writes a generic H1 like `# Requirements` (with no descriptive title after it), that line strips to empty and the extractor falls through to the next non-empty heading. Recently this caused a task to be named "background" because the very next line was `## Background`. The directory is meaningless and the same trap will fire for any other generic second heading.

The cause is that the planning prompt (`api/pkg/services/spec_task_prompts.go:51-54`) only says "requirements.md - User stories + acceptance criteria" — it doesn't tell the agent how to title the file.

## User Stories

**As a Helix operator browsing the spec-task list,**
I want every task directory to be named after the actual subject of the task,
so that I can find a task by skimming directory names instead of opening files.

**As an agent writing a requirements.md,**
I want the planning prompt to specify the exact title format,
so that I don't accidentally write a generic title that breaks downstream naming.

## Acceptance Criteria

1. The planning prompt template in `spec_task_prompts.go` explicitly tells the agent to title `requirements.md` as `# Requirements: <Descriptive Title>` — never just `# Requirements` and never just a topic word.
2. The instruction includes a one-line example of a good title and a one-line example of a bad title, so the rule is unambiguous.
3. The instruction sits next to the existing "Create exactly 3 files" list (lines 51-54) so it can't be missed.
4. The instruction also covers `design.md` and `tasks.md` for consistency (e.g. `# Design: <Title>`, `# Implementation Tasks: <Title>`) — same descriptive title across all three files.
5. No code change to `SpecTitleFromRequirements()` or the directory-naming pipeline is required; this is a prompt-only fix.

## Out of Scope

- Adding a fallback in `SpecTitleFromRequirements()` to scan H2 headings or ignore "Background". The user's explicit request is to fix this at the prompt level. A code-level fallback would mask future agents that ignore the prompt.
- Renaming existing badly-named task directories. Past tasks stay where they are.
- Validating the title server-side after the push (could be a follow-up if the prompt change proves insufficient).
