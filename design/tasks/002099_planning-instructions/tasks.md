# Implementation Tasks: Collapse Spec-Approved Implementation Prompt in Chat

- [x] In `frontend/src/components/session/CollapsibleSystemPrefix.tsx`, add an `APPROVAL_PROMPT_ANCHOR` regex (`/^## CURRENT PHASE: IMPLEMENTATION\b/`) and a `kind` field to `SplitResult`
- [x] Extend `splitSystemPrefix` to return `{ prefix: message.trim(), userText: "", label: null, kind: "approval" }` when the approval anchor matches at the start of the message
- [x] Tag the existing user-request branch with `kind: "user-request"` so call sites can disambiguate without re-matching
- [x] In `frontend/src/components/session/Interaction.tsx`, plumb the new `kind` field through the `useMemo` displayData destructure
- [x] In `Interaction.tsx`, when `systemPrefix` is set and `userMessageBody` is empty, render only `CollapsibleSystemPrefix` (suppress the user bubble entirely)
- [x] In `Interaction.tsx`, when `kind === "approval"` pass label `"Spec Approved — Implementation Instructions"` to `CollapsibleSystemPrefix`
- [x] Confirm edit/regenerate still puts the full original message into the textarea (no behavioural change expected — verified by reading code: `editedMessage` is initialised from `userMessage`, the full original)
- [x] Add unit tests in `CollapsibleSystemPrefix.test.ts`: approval anchor at start triggers collapse with empty userText, anchor appearing mid-message does NOT trigger, existing user-request cases still pass (10/10 passing)
- [x] `cd frontend && yarn tsc` clean (passed in helix-frontend-1 container)
- [~] Test end-to-end in the inner Helix browser at `http://localhost:8080`: register if needed, find or create a spec task that has had its spec approved, confirm the implementation prompt renders as a collapsed disclosure (not a wall of text) and that expanding it shows the full prompt
- [ ] Take before/after screenshots into `screenshots/` and reference them in `pull_request_helix.md`
- [ ] Write `pull_request_helix.md` with summary, screenshots, and test plan; push helix-specs
- [ ] Push feature branch to `helix` repo
