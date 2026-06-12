# Design: Collapse Spec-Approved Implementation Prompt in Chat

## How the Data Flows

When the user approves the spec, the API path is:

`SpecDrivenTaskService.ApproveSpecs`
(`api/pkg/services/spec_driven_task_service.go:1167`)
→ `AgentInstructionService.SendApprovalInstruction`
(`api/pkg/services/agent_instruction_service.go:620`)
→ `BuildApprovalInstructionPrompt` (same file, line 452) renders the
`approvalPromptTemplate` (same file, line 126)
→ `s.messageSender(ctx, task, message, userID, false)` creates a real
chat interaction whose `prompt_message` is the rendered template.

The rendered template **always** begins with:

```
## CURRENT PHASE: IMPLEMENTATION

You are now in the IMPLEMENTATION phase. The planning/spec-writing phase is complete.
```

There is no `**User Request:**` or `**Original Request:**` marker anywhere
in this prompt — it is 100% system content — so the regex added in task
001680 cannot match it. The whole thing surfaces as a giant user bubble.

## Why Frontend Splitting (Not `DisplayMessage`)

Task 001680 explicitly rejected the backend `DisplayMessage` override path
in favour of a frontend split, to keep the change contained. We follow the
same precedent here: extend the frontend detection rather than mutate the
interaction at write time. This also avoids a schema/data migration for
existing approval interactions in the database.

## Where to Make the Change

Two files, both already touched by 001680:

1. `frontend/src/components/session/CollapsibleSystemPrefix.tsx` —
   extend `splitSystemPrefix` (or add a sibling detector) so it also
   recognises the approval-prompt anchor and returns a result that means
   *"the entire message is system content, render no user body"*.
2. `frontend/src/components/session/Interaction.tsx` — render the
   collapsible alone, without the user message bubble, when the split
   result has an empty `userText`.

## Detection Anchor

The approval prompt's opening lines are produced by a `text/template` whose
prefix is a literal string in the Go source — i.e. it is stable across
tasks and not user-controlled. Match on the **start of the message** to
avoid false positives:

```typescript
const APPROVAL_PROMPT_ANCHOR = /^## CURRENT PHASE: IMPLEMENTATION\b/;
```

Matching the start-of-line `## CURRENT PHASE: IMPLEMENTATION` is sufficient
and unambiguous: that string only appears in the approval prompt
template. Anchor on `^` to prevent any future user message that quotes the
phrase mid-text from being eaten by the disclosure.

## Proposed Shape of `splitSystemPrefix` (updated)

```typescript
export interface SplitResult {
  prefix: string | null;       // collapsed system text (markdown)
  userText: string;            // visible user-bubble text (may be "")
  label: string | null;        // raw marker captured, used to pick UI label
  kind: "user-request" | "approval" | null; // disambiguator for the UI
}

const USER_REQUEST_SPLIT =
  /^([\s\S]*?)\n\n\*\*(User Request|Original Request[^*]*?):\*\*\n?([\s\S]*)$/;

const APPROVAL_PROMPT_ANCHOR = /^## CURRENT PHASE: IMPLEMENTATION\b/;

export function splitSystemPrefix(message: string): SplitResult {
  if (!message) return { prefix: null, userText: message, label: null, kind: null };

  // Existing: planning prompt + user request body
  const m = message.match(USER_REQUEST_SPLIT);
  if (m) {
    return {
      prefix: m[1].trim(),
      userText: m[3].trim(),
      label: m[2],
      kind: "user-request",
    };
  }

  // New: pure system instruction (approval prompt) — no user body.
  if (APPROVAL_PROMPT_ANCHOR.test(message)) {
    return {
      prefix: message.trim(),
      userText: "",
      label: null,
      kind: "approval",
    };
  }

  return { prefix: null, userText: message, label: null, kind: null };
}
```

Adding a `kind` field is the cleanest way to drive the UI's label and
"render bubble or not" decision without re-matching the regex at the call
site. Existing callers that destructure only `prefix`/`userText`/`label`
keep working unchanged.

## UI Changes in `Interaction.tsx`

Current rendering (simplified, see `Interaction.tsx:310`):

```tsx
{userMessage && (
  <Box ...>
    {systemPrefix && !isEditing && (
      <CollapsibleSystemPrefix prefix={systemPrefix} label={...} />
    )}
    <UserMessageBubble message={... userMessageBody ...} />
  </Box>
)}
```

Two adjustments:

1. **Suppress the bubble when there's no user body.** When
   `systemPrefix` is set and `userMessageBody === ""`, render only the
   `CollapsibleSystemPrefix`. Don't render the bubble box at all.
2. **Pick label by `kind`.** Existing label logic stays; add an
   `"approval"` branch:

   ```tsx
   const label =
     kind === "approval"
       ? "Spec Approved — Implementation Instructions"
       : systemPrefixLabel?.startsWith("Original Request")
         ? "Planning Instructions (cloned task)"
         : "Planning Instructions";
   ```

3. **Edit/regenerate preserves the full message.** Already correct:
   `editedMessage` is initialised from `userMessage` (the full original),
   not `userMessageBody`, so the textarea on edit shows the entire
   approval prompt. No change needed beyond making sure we don't
   accidentally feed an empty string somewhere.

## Visual Style

Reuse `CollapsibleSystemPrefix` as-is. Its layout already aligns
`alignSelf: "stretch"` so it can sit alone in the message container.
The "approval" label is the only visible difference vs. the planning
disclosure — same icon, same border, same expand chevron.

## Scope

- Frontend-only. No backend, API, or types changes.
- Contained to two files: `CollapsibleSystemPrefix.tsx` and
  `Interaction.tsx`.
- The existing 7-test unit suite for `splitSystemPrefix` covers the
  user-request branch; add new tests for the approval branch (see tasks).

## Codebase Notes (for future cloners)

- The other system-generated prompts in `agent_instruction_service.go`
  (`commentPromptTemplate`, `implementationReviewPromptTemplate`,
  `revisionPromptTemplate`, `mergePromptTemplate`) all share the same
  shape: a heading marker (`# Review Comment`, `# Implementation Ready
  for Review`, `# Changes Requested`, `# Implementation Approved -
  Please Merge`) followed by pure system content. The detection
  function is structured so they can be added as additional anchored
  regexes in the same `splitSystemPrefix` later, with appropriate
  labels — that is the suggested follow-up if the user requests it.
- `Interaction.tsx`'s `useMemo` is the single source of truth for
  derived display data; keep all new derivation there to match the
  existing pattern.
- The `DisplayMessage` field on `Interaction` still exists and is
  unused for spec tasks. Continuing to ignore it is a deliberate
  consistency choice with task 001680.
- The approval prompt is sent via `messageSender` (i.e. the standard
  WebSocket+DB path), so it shows up in the same `interactions` array
  the chat renders from — no special-casing of the prompt source is
  needed in the UI.
