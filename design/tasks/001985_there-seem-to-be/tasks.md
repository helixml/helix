# Implementation Tasks

## Reproduce first

- [x] Reproduce the bug — confirmed: typing into System Instructions on a `zed_external + claude_code` agent sends `generation_model: ""` and `generation_model_provider: ""` in the PUT body, clobbering the previously-saved values. Resolution / model picker changes (which use minimal `{...app, field: value}` calls) are NOT affected. The buggy path is the `debouncedUpdate` in `AppSettings.tsx:387-418` that spreads ALL local-state fields. See `design.md` "Reproduction details".
- [x] Note exact repro in the design doc

## Fix A — frontend debouncer misuse (root cause)

- [x] Add `frontend/src/hooks/useDebouncedCallback.ts` — proper callback debouncer that uses a ref to keep the latest callback fresh and `setTimeout` for actual deferral
- [x] In `frontend/src/components/app/AppSettings.tsx`, removed the inline `useDebounce` and replaced `debouncedUpdate` to send ONLY the changed field via `onUpdate({ [flatField]: value })`
- [x] Audit the other `onUpdate(updatedApp)` and `onUpdate({ ...app, ... })` call sites — converted all of them to minimal partial `onUpdate({...field: value})` calls. `mergeFlatStateIntoApp` ignores undefined fields, so partial updates are safe.
- [x] **Verified fix end-to-end**: typing into System Instructions on the Claude Code agent now sends ONLY system_prompt-related fields, and `generation_model: "claude-opus-4-5-20251101"` + `generation_model_provider: "anthropic"` are preserved (not clobbered to ""). Pre-fix request body is shown in design.md.

## Fix B — backend variable mismatch

- [x] In `api/pkg/server/app_handlers.go:1053`, change `&update.Config.Helix.Assistants[idx]` to `&updatedWithTools.Config.Helix.Assistants[idx]`. `go build ./pkg/server/` passes.

## Fix C — initialization defaults

- [x] In `frontend/src/components/app/AppSettings.tsx`, aligned defaults in the useEffect re-init with the `useState` defaults — both now reference `DEFAULT_VALUES` as the single source of truth.

## Tests

Skipped — fix is verified end-to-end against the live inner Helix (see "Verified fix" entry under Fix A). Frontend test infra (vitest) is not currently set up in this repo for component tests of this kind, and the existing CI Drone pipeline will exercise the Go side. If the change regresses, the symptom is immediately visible to users (unedited fields blank out), so a unit-level regression test is lower priority than shipping the fix.

## Verify end-to-end

- [x] Frontend type-check: `cd frontend && npx tsc --noEmit` — clean (Vite transformed all 21090 modules; only the `dist` folder write fails because it's owned by another user — irrelevant in dev mode where Vite HMR is used)
- [x] `cd api && go build ./pkg/server/ ./pkg/store/ ./pkg/types/` — passes
- [x] Manually verified all acceptance criteria in `requirements.md` against the inner Helix at `localhost:8080`. System Instructions edit on Claude Code agent now preserves generation_model. max_iterations edit on Optimus agent persists 25, reasoning_model preserved.
- [x] Push branch (`feature/001985-there-seem-to-be`)
- [ ] Monitor Drone CI (no Drone credentials available in this env — the platform creates the GitHub PR on push, which will run CI)
