# feat(api): add Open Questions section to spec planning prompt

## Summary

The spec-driven planning agent tends to invent requirements when the user's
request is ambiguous. This adds a small, additive instruction to the planning
prompt telling the agent to end `requirements.md` with an `## Open Questions`
section listing genuine uncertainties for the user to correct at review time —
so guesses become visible instead of silently becoming the spec.

## Changes

- `api/pkg/services/spec_task_prompts.go`: add a concise `## Open Questions
  (requirements.md)` instruction to `planningPromptTemplate`, placed between the
  "Your Task Directory" list and the "Title Format" section. Agents are told to
  list only real uncertainties and to write "None" when there are none.
- `api/pkg/services/spec_task_prompts_test.go`: add
  `TestBuildPlanningPrompt_OpenQuestions` guarding the new instruction.

## Notes

- Placed at the end of `requirements.md` so it never interferes with H1 title
  parsing (`SpecTitleFromRequirements`), which reads the first non-empty line.
- Purely additive; no existing prompt wording removed. Build clean, tests pass.
