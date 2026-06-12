# Implementation Tasks: Collapse Spec-Approved Implementation Prompt in Chat

- [ ] In `frontend/src/components/session/CollapsibleSystemPrefix.tsx`, add an `APPROVAL_PROMPT_ANCHOR` regex (`/^## CURRENT PHASE: IMPLEMENTATION\b/`) and a `kind` field to `SplitResult`
- [ ] Extend `splitSystemPrefix` to return `{ prefix: message.trim(), userText: "", label: null, kind: "approval" }` when the approval anchor matches at the start of the message
- [ ] Tag the existing user-request branch with `kind: "user-request"` so call sites can disambiguate without re-matching
- [ ] In `frontend/src/components/session/Interaction.tsx`, plumb the new `kind` field through the `useMemo` displayData destructure
- [ ] In `Interaction.tsx`, when `systemPrefix` is set and `userMessageBody` is empty, render only `CollapsibleSystemPrefix` (suppress the user bubble entirely)
- [ ] In `Interaction.tsx`, when `kind === "approval"` pass label `"Spec Approved — Implementation Instructions"` to `CollapsibleSystemPrefix`
- [ ] Confirm edit/regenerate still puts the full original message into the textarea (no behavioural change expected — verify only)
- [ ] Add unit tests in `CollapsibleSystemPrefix.test.ts`: approval anchor at start triggers collapse with empty userText, anchor appearing mid-message does NOT trigger, existing user-request cases still pass
- [ ] `cd frontend && yarn tsc` clean
- [ ] `cd frontend && yarn build` clean
- [ ] Test end-to-end in the inner Helix browser at `http://localhost:8080`: register if needed, create a spec task, approve the spec, confirm the implementation prompt renders as a collapsed disclosure (not a wall of text) and that expanding it shows the full prompt
- [ ] Take before/after screenshots into `screenshots/` and reference them in `pull_request_helix.md`
- [ ] Write `pull_request_helix.md` with summary, screenshots, and test plan; push helix-specs
- [ ] Push feature branch to `helix` repo
