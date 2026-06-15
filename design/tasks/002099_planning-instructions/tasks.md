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
- [x] Test end-to-end in the inner Helix browser at `http://localhost:8080`: injected a chat session whose `prompt_message` matches the real approval template output (the full spec-task approval flow takes too long; the rendering is pure frontend and triggers on `prompt_message` content). Verified collapsed-by-default, expand-shows-full-content, no empty user bubble.
- [x] Take before/after screenshots into `screenshots/` and reference them in `pull_request_helix.md`. Captured 3: `00-before-wall-of-text.png`, `01-after-collapsed.png`, `02-after-expanded.png`. Generated "before" by checking out `main` for the two files, reloading, and screenshotting; then restored via `git checkout HEAD --`.
- [x] Write `pull_request_helix.md` with summary, screenshots, and test plan; push helix-specs
- [x] Push feature branch to `helix` repo (`feature/002099-collapse-spec-approved`, 1 commit ahead of origin/main)
