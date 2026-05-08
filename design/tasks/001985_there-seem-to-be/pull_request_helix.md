# fix: agent settings clobbering on save

## Summary

Agent configuration changes were silently overwriting other persisted fields. The autosave path in `AppSettings.tsx` was sending the *entire* local form state on every debounced save, including fields not visible on the current agent type. For `zed_external + claude_code` agents this clobbered `generation_model` and `generation_model_provider` with empty strings every time the user typed in System Instructions, because those fields are only set by the multi-turn Helix-agent UI which the user wasn't using.

The fix: each handler now sends only the field(s) that actually changed. `mergeFlatStateIntoApp` in `useApp.ts` already ignores `undefined` fields, so partial updates merge cleanly with the persisted assistant config.

## Reproduction (pre-fix)

1. Create a Claude Code agent (default during onboarding).
2. Open `/orgs/<org>/agent/<id>?tab=settings`.
3. Type into "System Instructions".
4. Inspect the PUT request body in the network panel: `generation_model: ""`, `generation_model_provider: ""` — even though the saved values were `claude-opus-4-5-20251101` / `anthropic`.
5. Reload the page → the Model picker now shows blank.

## Verification (post-fix)

- Reproduced the failure exactly as above (DB row went from `"claude-opus-4-5-20251101"` to `""`).
- Applied the fix; the same edit now sends only the system_prompt-related fields. DB row keeps `generation_model = "claude-opus-4-5-20251101"`.
- Cross-checked on a Helix-agent (Optimus) by editing `max_iterations` from 15 → 25 — value persisted, all other fields untouched.
- `cd api && go build ./pkg/server/ ./pkg/store/ ./pkg/types/` — passes.
- `cd frontend && npx tsc --noEmit` — clean.

## Changes

- `frontend/src/hooks/useDebouncedCallback.ts` (new): proper callback debouncer that uses a ref for the latest callback.
- `frontend/src/components/app/AppSettings.tsx`:
  - Replace inline `useDebounce` with the shared `useDebouncedCallback` hook.
  - `debouncedUpdate` now sends only the field that changed (`onUpdate({ [field]: value })`) instead of rebuilding the whole `IAppFlatState` from local state.
  - `handleCheckboxChange`, `handleAgentTypeChange`, `handleModelChange`, `handleEffortSelect`, the model pickers, runtime/credentials selects, and resolution/zoom controls all now send minimal updates.
  - Align `useState` defaults and the post-mount `useEffect` re-init defaults — both now reference `DEFAULT_VALUES` (which mirrors `api/pkg/store/store_apps.go::setAppDefaults`).
- `api/pkg/server/app_handlers.go`: fix variable mismatch in the `updateApp` tool/knowledge validation loop — was iterating `updatedWithTools.Config.Helix.Assistants` but reading from `update.Config.Helix.Assistants[idx]` (latent bug if `ParseAppTools` changes assistant order/count).

## Test plan

- [x] Pre-fix repro confirmed (DB clobber observed)
- [x] Post-fix verification on Claude Code agent (System Instructions edit)
- [x] Post-fix verification on Helix agent (max_iterations edit)
- [x] Frontend type-check clean
- [x] Go build clean
- [ ] Drone CI green
