# Design: Agent Editor Tab Switching Loses Settings

## Root Cause

The agent editor renders tabs conditionally — only the active tab's component is mounted. Each tab component (AppSettings, AppearanceSettings) manages its own local state, initialized once from the `app` prop on mount.

The bug is a **race condition between concurrent saves using stale state**:

1. User changes resolution on Settings tab → `saveFlatApp()` fires an API PUT request
2. User switches to Appearance tab (AppSettings **unmounts**, its local state is destroyed)
3. User changes name → AppearanceSettings calls `saveFlatApp({ ...flatApp, name })` 
4. The `flatApp` prop and the `app` in `saveFlatApp`'s closure may still be **stale** (the API response from step 1 hasn't returned yet, or React hasn't re-rendered)
5. The second save sends the old `external_agent_config` to the API, overwriting the resolution change

Even without a race, the `isInitialized` ref pattern in AppSettings (line 332) means it only reads from the `app` prop once — if `app` changes while the component is mounted (e.g., from a concurrent save by another tab), AppSettings ignores the update.

### Key Files

| File | Role |
|------|------|
| `frontend/src/pages/App.tsx:214-242` | Conditional tab rendering (mount/unmount) |
| `frontend/src/components/app/AppSettings.tsx:330-377` | Local state init with `isInitialized` guard |
| `frontend/src/components/app/AppSettings.tsx:835-857` | Resolution onChange → immediate save |
| `frontend/src/components/app/AppearanceSettings.tsx:48-65` | Name/description save on blur with `...app` spread |
| `frontend/src/hooks/useApp.ts:555-565` | `saveFlatApp` — merges updates into `app` from closure |
| `frontend/src/hooks/useApp.ts:257-312` | `mergeFlatStateIntoApp` — applies flat state to IApp |
| `frontend/src/utils/app.ts:31-81` | `getAppFlatState` — flattens IApp to IAppFlatState |

### Why `...app` spread doesn't save us

AppearanceSettings does `{ ...app, name, description, conversation_starters }` which includes `external_agent_config` from the `flatApp` prop. But `flatApp` is derived from the hook's `app` state via `useMemo`. If the previous save hasn't completed (no optimistic update), `flatApp` still has the old resolution.

## Solution: Keep All Tabs Mounted

Replace conditional rendering with CSS visibility. Instead of:

```tsx
{tabValue === 'settings' && <AppSettings ... />}
{tabValue === 'appearance' && <AppearanceSettings ... />}
```

Use:

```tsx
<Box sx={{ display: tabValue === 'settings' ? 'block' : 'none' }}>
  <AppSettings ... />
</Box>
<Box sx={{ display: tabValue === 'appearance' ? 'block' : 'none' }}>
  <AppearanceSettings ... />
</Box>
```

This keeps all tab components permanently mounted, so:
- Local state in each tab survives tab switches
- The `isInitialized` pattern works correctly (component never re-mounts with stale data)
- Each tab's `onUpdate` always starts from the latest `app` in the `saveFlatApp` closure, because the closure updates whenever `app` state changes — and since the component stays mounted, it always has the current closure reference

### Why this approach

- **Minimal change**: Only touches `App.tsx` rendering logic — no refactoring of state management in any tab component
- **Already the pattern used elsewhere**: Material-UI's `TabPanel` and many tab libraries keep content mounted by default
- **No new state management complexity**: Avoids introducing centralized form state, save queues, or optimistic updates
- **Fixes the class of bugs, not just this instance**: Any future settings on any tab are automatically protected

### What about the remaining race condition?

With tabs always mounted, the save-overwrite race between two rapid saves across tabs technically still exists. However, it becomes much harder to trigger because:
1. Both components are always mounted, so both always have the latest `saveFlatApp` closure
2. The user would need to make changes on two tabs faster than the API round-trip (~100-200ms)
3. Both AppSettings and AppearanceSettings already save immediately on change/blur, so there's no "pending unsaved state" that accumulates

If this proves insufficient in practice, a follow-up could add `useSyncExternalStore` or debounced save queuing — but that's likely over-engineering for now.

## Codebase Patterns Observed

- This project uses React hook-based state (`useState`/`useCallback`/`useMemo`) — no Redux/Zustand
- Tab components receive a flattened app state (`IAppFlatState`) and an `onUpdate` callback
- `mergeFlatStateIntoApp` does field-level merging, not wholesale replacement
- The `isInitialized` ref pattern is used to prevent re-initialization on re-renders, but breaks when the component unmounts and remounts with changed data
