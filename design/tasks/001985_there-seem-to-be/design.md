# Design: Fix Agent Settings Persistence

## Save Flow (current)

```
User edits field in AppSettings.tsx
    ↓ onChange
setLocalState(value)  +  handleAdvancedChangeWithDebounce(field, value)
    ↓
debouncedUpdate(field, value)               ← see Bug A
    ↓
onUpdate(updatedApp: IAppFlatState)         ← prop = appTools.saveFlatApp
    ↓
saveFlatApp → mergeFlatStateIntoApp(app, updates) → saveApp(merged)
    ↓
api.put(`/api/v1/apps/${id}`, app)
    ↓
HelixAPIServer.updateApp (app_handlers.go)
    ↓
validateProvidersAndModels(&update)
ParseAppTools(&update) → updatedWithTools
loop: validate tools / knowledge        ← see Bug B
    ↓
Store.UpdateApp(updatedWithTools)
ensureKnowledge / ensureTriggerConfigurations
return updated → setApp(savedApp)
```

## Confirmed Bugs

### Bug A — `useDebounce` is misused as a function debouncer

`frontend/src/hooks/useDebounce.ts` is a **value debouncer** (`useDebounce<T>(value: T, delay): T`). It returns the most recent value after `delay` ms of stability.

`frontend/src/components/app/AppSettings.tsx:387` uses it like a callback debouncer:
```ts
const debouncedUpdate = useDebounce((field, value) => { …onUpdate(updatedApp) }, 300)
```

Two consequences:
1. The arrow function is a fresh reference every render, so `useDebounce` keeps resetting its 300ms timer; `debouncedUpdate` ends up holding **whichever closure was last "stable" for 300ms** — typically a stale one captured early in the edit session.
2. There is **no debouncing of the side effect**. `handleAdvancedChangeWithDebounce` (line 421-423) calls `debouncedUpdate(field, value)` synchronously on every keystroke. Each keystroke fires `onUpdate → saveFlatApp → PUT /api/v1/apps/:id`.

The captured closure references stale `app`, `system_prompt`, `model`, `provider`, `agent_mode`, etc. So when a save fires, the request body contains:
- `...app` from a previous render (most recent app that was the prop at capture time — this part is usually OK because it's the prop)
- Local-state fields like `model`, `provider`, `system_prompt` from the **render where the closure was captured** (potentially several keystrokes behind)
- Only the `field` argument that the caller just passed gets the freshest value

This produces "ghost saves" that overwrite recent edits to other fields with stale values, matching the reported "settings invariably don't persist" symptom — they get clobbered by an in-flight stale write.

### Bug B — Variable mismatch in update validation loop

`api/pkg/server/app_handlers.go:1052-1053`:
```go
for idx := range updatedWithTools.Config.Helix.Assistants {
    assistant := &update.Config.Helix.Assistants[idx]   // wrong: should be updatedWithTools
```

`updatedWithTools` is the parsed/normalized version returned by `store.ParseAppTools(&update)`. Tools and assistants on `update` may differ from `updatedWithTools` (parsing can move/rewrite tool entries). The loop validates the wrong assistant — usually harmless if the structures happen to be parallel, but if `ParseAppTools` adds, removes, or reorders an assistant, this either panics on an out-of-range index or validates the wrong entry.

Persistence impact is indirect: the loop never mutates state, so the persisted object is `updatedWithTools` regardless. But the bug should still be fixed — it's incorrect, latent, and called out in the explore pass.

### Bug C — Initialization defaults disagree with `useState` defaults

In `AppSettings.tsx`, the initial `useState` calls and the post-mount `useEffect` (which runs once via `isInitialized.current`) use **different fallback values** when the app field is unset:

| Field | useState (line 289-295) | useEffect (line 365-371) |
|---|---|---|
| `maxTokens` | `app.max_tokens \|\| 2000` | `app.max_tokens \|\| 0` |
| `temperature` | `app.temperature \|\| DEFAULT_VALUES.temperature` (0.1) | `app.temperature \|\| 0` |
| `topP` | `app.top_p \|\| 1` | `app.top_p \|\| 0` |
| `reasoningEffort` | `app.reasoning_effort \|\| 'none'` | `app.reasoning_effort \|\| DEFAULT_VALUES.reasoning_effort` (matches) |

For an app where these fields are unset/zero, the form briefly shows `2000` then resets to `0`. This isn't itself a "doesn't persist" bug, but it can manifest as one: a user opens settings, sees `0`, types `4000`, the in-flight stale-closure save (Bug A) clobbers it back to `0`, and reloading shows `0`.

## Likely Root Cause

**Bug A is the prime suspect.** The misused `useDebounce` causes every keystroke to fire a save with stale local-state values, so any field the user wasn't *just* typing into gets reverted to a previous value. This matches "invariably don't persist": each save reverts the previous field's edit.

Bug C amplifies the visible damage when default values are involved. Bug B is a separate latent correctness issue.

## Proposed Fixes

### Fix 1 — Use a real callback debouncer in `AppSettings.tsx`

Replace the misused `useDebounce` with a proper debounced-callback pattern. Two reasonable options:

**Option A** (preferred — minimal surface area): introduce a small `useDebouncedCallback` hook in `frontend/src/hooks/`:

```ts
import { useEffect, useMemo, useRef } from 'react'

export function useDebouncedCallback<A extends unknown[]>(
  fn: (...args: A) => void,
  delay: number,
) {
  const fnRef = useRef(fn)
  useEffect(() => { fnRef.current = fn }, [fn])
  return useMemo(() => {
    let t: ReturnType<typeof setTimeout> | null = null
    return (...args: A) => {
      if (t) clearTimeout(t)
      t = setTimeout(() => fnRef.current(...args), delay)
    }
  }, [delay])
}
```

The ref keeps the callback fresh (no stale closures), and the returned function actually defers execution by `delay` ms.

**Option B**: pull in `lodash.debounce` via `useMemo`. Adds a dependency line but is well-trodden.

In either case, change the `debouncedUpdate` definition in `AppSettings.tsx` to use the new hook, and ensure the closure passes through the **latest** local state — easiest by reading state via refs or by recomputing the merged object inside the hook each call.

Cleanest: stop merging local state in the debounced callback at all. Instead, on each `setX(value)` plus `handleAdvancedChangeWithDebounce(field, value)`, push **only the changed field** to `saveFlatApp({ [field]: value })`. `mergeFlatStateIntoApp` already only applies fields that are `!== undefined`, so partial updates work and are safer than spreading stale local state.

### Fix 2 — Correct the variable in `app_handlers.go:1053`

Change:
```go
assistant := &update.Config.Helix.Assistants[idx]
```
to:
```go
assistant := &updatedWithTools.Config.Helix.Assistants[idx]
```

### Fix 3 — Align initialization defaults

Pick one source of truth (the `DEFAULT_VALUES` constant) and use it in both `useState` and the `useEffect` initializer in `AppSettings.tsx`. The simpler approach is to drop the `useEffect` re-initialization entirely — `useState` already takes the initial value from `app` on first render. The `useEffect` exists because `app` may load asynchronously, so a guard like `if (!app) return null` at the component top, or rendering `AppSettings` only when `app` is defined, is more idiomatic than the manual `isInitialized.current` ref.

(If keeping the effect, just align the fallbacks.)

## Verification Plan

1. Reproduce the bug: load `/orgs/mola/agent/app_01kqx5wk13n0ej2av2xrz3rsad?tab=settings`, edit System Instructions, reload → confirm value reverts. Capture the network panel showing the stale PUT body.
2. Apply Fix 1 + Fix 3, reload, repeat: edit persists.
3. Add a test that drives the save path:
   - Frontend: vitest/react-testing-library for `AppSettings` — type into System Instructions, await debounce, assert `onUpdate` receives the typed value (and only the typed field changes if going with the partial-update approach).
   - Backend: Go integration test — `PUT /api/v1/apps/:id` with a payload, then `GET` and assert round-trip equality for the affected fields. Use `memorystore` per `CLAUDE.md`.
4. Apply Fix 2; add a unit test that gives `ParseAppTools` an input where the assistant order/count changes between `update` and `updatedWithTools`, and assert no panic / correct validation target.
5. Manual smoke test in inner Helix at `http://localhost:8080`: register `test@helix.ml` / `helixtest`, create an app, edit each field type from acceptance criteria, reload, verify.

## Out of Scope / Deferred

- Refactoring `AppSettings.tsx` (1335 lines) — only touch what is needed to fix the bug. The `CLAUDE.md` rule "extract components at 500+ lines" suggests this is overdue, but it's a separate task.
- The 3-second-vs-300ms autosave UX question (whether to keep autosave on every keystroke or move to an explicit Save button) — leave the autosave model as-is, just fix the correctness.
