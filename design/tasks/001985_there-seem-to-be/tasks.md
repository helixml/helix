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

- [ ] In `api/pkg/server/app_handlers.go:1053`, change `&update.Config.Helix.Assistants[idx]` to `&updatedWithTools.Config.Helix.Assistants[idx]`

## Fix C — initialization defaults

- [ ] In `frontend/src/components/app/AppSettings.tsx`, align defaults at lines 365-371 with the `useState` defaults at lines 289-295 — use `DEFAULT_VALUES` as the single source of truth for both
- [ ] Consider replacing the `isInitialized.current` ref + `useEffect` re-init pattern with a simple `if (!app) return null` guard so `useState(app.foo)` runs once with a real `app`

## Tests

- [ ] Add a frontend test for `AppSettings.tsx` (vitest + react-testing-library): type into System Instructions, advance timers past the debounce, assert `onUpdate` receives only `{ system_prompt: <typed> }`
- [ ] Add a Go integration test in `api/pkg/server/` (using `memorystore` per CLAUDE.md) that PUTs an app with custom values for system_prompt, model, max_iterations, then GETs and asserts round-trip equality
- [ ] Add a Go unit test for the `app_handlers.go:1052-1078` loop that gives `ParseAppTools` an input that mutates assistant order/count, asserts no panic and correct validation target

## Verify end-to-end

- [ ] Run `cd frontend && yarn build` — must pass
- [ ] Run `cd api && go build ./pkg/server/ ./pkg/store/ ./pkg/types/` — must pass
- [ ] Manually verify the acceptance criteria in `requirements.md` against the inner Helix at `localhost:8080`
- [ ] Push branch, open PR against `helixml/helix`, monitor Drone CI via `drone_build_info`, fix any failures
