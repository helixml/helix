# Design: Add Open Questions Section to Spec Planning Prompt

## Overview

Single-file prompt edit plus a guard test. We add a short instruction to the
planning prompt template so the agent emits an **Open Questions** section in
`requirements.md`.

## Key File

`api/pkg/services/spec_task_prompts.go` — contains `planningPromptTemplate`
(a Go `text/template`) and `BuildPlanningPrompt(...)` which renders it. This is
the canonical planning prompt used by both explicit spec generation and the
auto-start orchestrator path.

The "## Your Task Directory" block already lists the three files the agent must
create:

```
1. requirements.md - User stories + acceptance criteria
2. design.md - Architecture + key decisions
3. tasks.md - Checklist of implementation tasks using [ ] format
```

## Change

Add a short, standalone instruction to the template — placed immediately after
the "## Your Task Directory" list (before "## CRITICAL: Title Format") — telling
the agent to end `requirements.md` with an Open Questions section. Proposed text:

```markdown
## Open Questions (requirements.md)

End `requirements.md` with an `## Open Questions` section listing any genuine
questions or uncertain assumptions you have for the user — anything you would
otherwise have to guess. This surfaces guesses so the user can correct them at
review time instead of them silently becoming the spec. Only list real
uncertainties; if there are none, write "None".
```

### Why this placement

- The H1 title is parsed from the *first* non-empty line of `requirements.md`
  (`SpecTitleFromRequirements`, `git_helpers.go:324`). Instructing the agent to
  put Open Questions at the *end* of the file guarantees no interference with
  title/branch-name derivation.
- Keeping the instruction as its own short section (rather than editing the
  one-line "requirements.md - User stories + acceptance criteria" description)
  keeps the change isolated and easy to revert, and gives the agent enough
  context to behave well without bloating the prompt.

## Decisions & Rationale

- **Additive, not a rewrite.** The prompt is trusted; we change as little as
  possible. One new section, no existing wording removed.
- **"None" escape hatch.** Explicitly permitting "None" prevents the agent from
  fabricating questions just to fill the section — mirrors the intent of
  reducing guessing, not adding noise.
- **No downstream/parsing changes needed.** Nothing reads requirements.md
  section-by-section except title extraction, which is unaffected.

## Testing

- Extend `spec_task_prompts_test.go` with an assertion that the rendered prompt
  contains the Open Questions instruction (mirroring the existing
  `TestBuildPlanningPrompt_TitleFormatRule` guard pattern).
- Verify the existing title-format test still passes.
- Optional manual check: run `go build ./api/pkg/services/` and, if convenient,
  generate a spec task in the inner Helix and confirm `requirements.md` gains an
  Open Questions section.

## Gotchas

- Do not touch the `{{.ClonedTaskPreamble}}` / `{{.Guidelines}}` / Kodit
  template variables — the edit is plain literal text inside the template
  string (note the string is assembled with `+ "..." +` concatenation for
  backtick/code-fence segments; add the new section as plain template text, not
  inside a concatenated code-fence unless a fenced example is desired).
