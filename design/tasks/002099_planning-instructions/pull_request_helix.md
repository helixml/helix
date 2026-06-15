# fix(frontend): collapse spec-approved implementation prompt in chat

## Summary

After a spec task's design is approved, the API sends an "implementation
instructions" message to the agent via `SendApprovalInstruction` — a
multi-page wall of system content rendered by `approvalPromptTemplate`
in `agent_instruction_service.go`. Until now it showed up in the chat
view as a giant user bubble, drowning out the actual conversation.

This change extends the existing `splitSystemPrefix` helper (added by
task 001680 for the `**User Request:**` planning split) to recognise the
approval prompt's stable opening anchor — `## CURRENT PHASE: IMPLEMENTATION`
at the start of the message — and render it inside the existing
`CollapsibleSystemPrefix` disclosure, labelled **"Spec Approved —
Implementation Instructions"**.

Because the approval prompt has no embedded user text, the user message
bubble is suppressed entirely when this case fires.

## Changes

- `frontend/src/components/session/CollapsibleSystemPrefix.tsx`: add
  `APPROVAL_PROMPT_ANCHOR` regex, add `kind` discriminator
  (`"user-request" | "approval" | null`) to `SplitResult`, extend
  `splitSystemPrefix` to return `kind: "approval"` when the anchor
  matches at the start of the message.
- `frontend/src/components/session/Interaction.tsx`: plumb `kind`
  through the `useMemo` displayData; pick the new label when
  `kind === "approval"`; suppress the user bubble + edit/copy controls
  when the entire message is system content
  (`!!systemPrefix && userMessageBody.length === 0`).
- `frontend/src/components/session/CollapsibleSystemPrefix.test.ts`:
  three new tests covering approval-anchor-at-start matches, mid-message
  anchor does not match, and user-request marker wins when both shapes
  appear. Existing 7 tests still pass (10/10 total).

## Test plan

- [x] `yarn tsc` clean
- [x] `yarn vitest run CollapsibleSystemPrefix.test.ts` — 10/10 pass
- [x] End-to-end in inner Helix at `http://localhost:8080`: injected a
      chat session whose `prompt_message` is the actual approval prompt
      template output. Verified:
  - Collapsed by default with new label (screenshot
    `01-after-collapsed.png`).
  - Expanding shows the full markdown including all the IMPLEMENTATION /
    CRITICAL RULES / Guidelines sections (screenshot
    `02-after-expanded.png`).
  - No empty user bubble next to the disclosure.
  - Assistant response below renders unchanged.
- [x] Reverted to `main` briefly to capture the wall-of-text baseline
      (screenshot `00-before-wall-of-text.png`) so the contrast is
      obvious to reviewers.
- [x] **Verified on the spec-task detail page too** (the more important
      surface, since this is where users review approved tasks). Attached
      the test session to a `spec_tasks` row and loaded
      `/orgs/:org/projects/:proj/tasks/:taskId`. The detail page renders
      its chat through `EmbeddedSessionView`, which mounts the same
      `Interaction` component — confirmed empirically that the
      "Spec Approved — Implementation Instructions" disclosure appears
      collapsed (screenshot `03-after-spec-task-detail-collapsed.png`)
      and expands correctly (`04-after-spec-task-detail-expanded.png`).
      Only one importer of `Interaction.tsx` exists in the codebase
      (`EmbeddedSessionView.tsx`), so this fix is automatically applied
      everywhere chat is rendered.

## Screenshots

**Before** — the approval prompt rendered as a giant user bubble,
pushing the actual chat off-screen:

![Before — wall of text](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002099_planning-instructions/screenshots/00-before-wall-of-text.png)

**After — collapsed by default:**

![After — collapsed disclosure](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002099_planning-instructions/screenshots/01-after-collapsed.png)

**After — expanded:**

![After — expanded disclosure](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002099_planning-instructions/screenshots/02-after-expanded.png)

**Spec-task detail page — collapsed:**

![Spec-task detail collapsed](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002099_planning-instructions/screenshots/03-after-spec-task-detail-collapsed.png)

**Spec-task detail page — expanded:**

![Spec-task detail expanded](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002099_planning-instructions/screenshots/04-after-spec-task-detail-expanded.png)

## Follow-up (out of scope)

The same `agent_instruction_service.go` file ships three more
system-generated prompts (`commentPromptTemplate`,
`implementationReviewPromptTemplate`, `revisionPromptTemplate`,
`mergePromptTemplate`) which share the same shape. They are also good
candidates for collapsing the same way (each has its own stable opening
heading like `# Review Comment` or `# Implementation Approved - Please
Merge`); deferred to a follow-up task per the spec.
