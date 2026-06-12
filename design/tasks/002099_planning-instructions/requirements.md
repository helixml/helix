# Requirements: Collapse Spec-Approved Implementation Prompt in Chat

## Background

A recent change (task `001680_when-rendering-the-spec`) split the first user
message in a spec-task chat at the `**User Request:**` marker so the long
planning prompt is hidden behind a collapsible "Planning Instructions"
disclosure, and the user's own text is shown as the prominent bubble.

There is a second wall of system-generated instructions that arrives **after
the design is approved**, sent by
`AgentInstructionService.SendApprovalInstruction` via the
`approvalPromptTemplate` in
`api/pkg/services/agent_instruction_service.go:126`. It starts with
`## CURRENT PHASE: IMPLEMENTATION` and contains the full implementation
brief (critical rules, checklist instructions, repo layout, screenshot
guide, PR description guide, etc.). It is currently rendered as a giant
plain user message bubble in the spec-task chat, drowning out everything
else in the conversation.

Unlike the first planning message there is no embedded user text — the
entire prompt is system-generated, so the existing
`**User Request:**`-style split does not apply.

## User Stories

**As a user viewing a spec task's chat session after approval**, I want the
"begin implementation" instructions to render as a compact, collapsed marker
so the actual conversation between me and the agent stays readable.

**As a user**, I want to be able to expand that marker to read the full
approval prompt if I'm debugging the agent's behaviour or curious what it
was told.

**As a user**, I want this to feel consistent with how the initial
"Planning Instructions" block already renders.

## Acceptance Criteria

1. In the spec-task chat view, a user-role message that starts with the
   approval prompt anchor (e.g. `## CURRENT PHASE: IMPLEMENTATION`) is
   rendered as a single collapsed disclosure, not as a plain message bubble.
2. The disclosure is labelled descriptively (e.g. **"Spec Approved —
   Implementation Instructions"**), uses the same visual style as the
   existing `CollapsibleSystemPrefix` (left border, info icon, expand
   chevron), and is collapsed by default.
3. Expanding the disclosure shows the full markdown of the approval prompt,
   including any reviewer comments that were attached on approval.
4. **No empty user bubble** is rendered next to or below the disclosure
   (the entire message is system content; there is no user body).
5. Messages that do not match the approval prompt anchor are unaffected
   (existing `**User Request:**` / `**Original Request:**` collapse keeps
   working; ordinary user messages still render as bubbles).
6. When the user edits/regenerates the interaction, the textarea still
   contains the full original message (so nothing is silently dropped on
   regenerate), matching the pattern set by task 001680.
7. Out of scope for this task: the other system-generated instruction
   prompts (`# Review Comment`, `# Implementation Ready for Review`,
   `# Changes Requested`, `# Implementation Approved - Please Merge`).
   They have the same shape and would benefit from the same treatment, but
   the user request is specifically about the spec-approved prompt; other
   prompts can be folded in later if desired.
