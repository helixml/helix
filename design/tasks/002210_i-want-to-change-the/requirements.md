# Requirements: Add Open Questions Section to Spec Planning Prompt

## Background

The spec-driven task planning agent is driven by a single prompt template
(`api/pkg/services/spec_task_prompts.go`). This prompt is trusted and works
well. However, when the user's request is ambiguous or incomplete, the
planning agent tends to *invent* requirements rather than surface its
uncertainty — producing specs with made-up or wrong assumptions.

We want a very narrow change: instruct the agent to add an **Open Questions**
section to `requirements.md` (the first spec page) listing any questions or
unresolved assumptions it has for the user. This makes guesses visible so the
user can correct them during review instead of them silently becoming "the spec".

## User Stories

- **As a user reviewing a generated spec**, I want the planning agent to list
  its open questions on the requirements page, so I can see where it was unsure
  and correct wrong assumptions before implementation.
- **As a maintainer of the planning prompt**, I want this change to be minimal
  and additive, so it doesn't disturb the existing, well-tuned prompt behaviour.

## Acceptance Criteria

1. The planning prompt template instructs the agent to include an **Open
   Questions** section in `requirements.md` that lists any questions or
   uncertain assumptions it has for the user.
2. The instruction makes clear the section should hold *genuine* uncertainties
   (not filler) — if there are none, the agent may state "None" rather than
   fabricate questions.
3. The change is additive and concise (a few lines / one short instruction).
   No existing sections of the prompt are removed or reworded beyond what is
   needed to reference the new section.
4. The H1 title parsing (`SpecTitleFromRequirements` in `git_helpers.go`) is
   unaffected — it reads the first non-empty line, and the Open Questions
   section appears later in the file.
5. Existing prompt tests still pass, and the change is covered by a test
   asserting the new instruction is present in the generated prompt.

## Open Questions

- **Placement of the section within `requirements.md`**: recommended at the end
  of `requirements.md` (after acceptance criteria) so it never interferes with
  title parsing. Confirm this is acceptable vs. placing it near the top.
- **Section heading wording**: recommended `## Open Questions`. Confirm the
  exact heading text you want agents to emit.

## Out of Scope

- No UI changes to render the Open Questions section specially.
- No changes to how specs are reviewed or approved.
- No changes to `design.md` or `tasks.md` structure.
