# Design: Persist Skip Planning and Start Immediately Toggles

## Summary

Mirror the existing `taskLabels` persistence pattern in
`NewSpecTaskForm.tsx` (lines 53, 109-115, 405) for the two checkbox toggles
`justDoItMode` and `autoStart`. Lazy-initialise both `useState` hooks from
`localStorage`, write through on every change, and stop clearing them in
`resetForm`.

## Key file

`/home/retro/work/helix/frontend/src/components/tasks/NewSpecTaskForm.tsx`

This is the only file that needs to change. There is no backend, no API
client, no shared component — the state lives entirely in this form.

## Storage keys

Two new module-level constants, alongside the existing
`LAST_LABELS_KEY` / `DRAFT_KEY_PREFIX` (line 53):

```ts
const LAST_JUST_DO_IT_KEY = "helix_new_spectask_just_do_it";
const LAST_AUTO_START_KEY = "helix_new_spectask_auto_start";
```

**Why global (not project-scoped)?** `taskLabels` is global
(`LAST_LABELS_KEY`) and these two checkboxes are workflow preferences ("I
always want to skip planning"), not per-project state. Per-project would
mean the user has to re-tick the box for every new project, which defeats
the point. The prompt draft IS project-scoped because draft text only
makes sense in the project where it was being typed.

**Why a separate key per checkbox** rather than one JSON blob? Smaller
blast radius if one of them gets corrupted; simpler reads; no migration
needed if we ever add a third toggle.

## Storage shape

Plain JSON booleans (`"true"` / `"false"`), stored with raw
`localStorage.setItem` and read with `JSON.parse`. No need for the
TTL helper in `frontend/src/utils/localStorage.ts` because:

- These are user preferences, not transient/dismissible UI hints — they
  shouldn't expire after N hours.
- The existing `taskLabels` and prompt draft also use raw `localStorage`,
  so we stay consistent with the file's own pattern.

## Initial state

Replace lines 120-121:

```ts
const [justDoItMode, setJustDoItMode] = useState(false);
const [autoStart, setAutoStart] = useState(false);
```

with lazy initialisers that mirror the `taskLabels` pattern (lines
109-115):

```ts
const [justDoItMode, setJustDoItMode] = useState<boolean>(() => {
  try {
    return JSON.parse(localStorage.getItem(LAST_JUST_DO_IT_KEY) || "false");
  } catch {
    return false;
  }
});
const [autoStart, setAutoStart] = useState<boolean>(() => {
  try {
    return JSON.parse(localStorage.getItem(LAST_AUTO_START_KEY) || "false");
  } catch {
    return false;
  }
});
```

The `try/catch` covers the "corrupt JSON" case from acceptance criterion 5.
`localStorage` itself can throw (Safari private mode, disabled storage),
which `try/catch` also handles.

## Persisting on change

Two options considered:

**Option A — write inside `onChange`** (chosen): replace the inline
`onChange={(e) => setJustDoItMode(e.target.checked)}` (line 990) with a
small handler that does `setJustDoItMode(v); localStorage.setItem(...)`.
Same for `autoStart` (line 1034). Simple, no new effects, fires exactly
when the user clicks. Matches the requirement that the next mount
reflects the toggle even if no task is submitted.

**Option B — `useEffect` watching the state.** Cleaner-feeling but it also
fires on the initial mount (writing the loaded value back to itself) and
adds an extra render hook for no reason.

Going with A.

Implementation sketch:

```ts
const handleJustDoItChange = (checked: boolean) => {
  setJustDoItMode(checked);
  try {
    localStorage.setItem(LAST_JUST_DO_IT_KEY, JSON.stringify(checked));
  } catch {
    /* ignore quota / disabled storage */
  }
};
const handleAutoStartChange = (checked: boolean) => {
  setAutoStart(checked);
  try {
    localStorage.setItem(LAST_AUTO_START_KEY, JSON.stringify(checked));
  } catch {
    /* ignore */
  }
};
```

Then in JSX (lines 990 and 1034):

```tsx
onChange={(e) => handleJustDoItChange(e.target.checked)}
onChange={(e) => handleAutoStartChange(e.target.checked)}
```

## resetForm changes

`resetForm` (lines 319-342) currently does:

```ts
setJustDoItMode(false);
setAutoStart(false);
```

Per acceptance criterion 3, **delete those two lines**. After a successful
task creation the boxes stay in whatever state the user last chose. This
is consistent with how `taskLabels` is handled — there's already a comment
at line 323 ("Labels intentionally kept …") explaining the same pattern;
add a sibling comment for the two checkboxes so the next reader doesn't
"helpfully" add the resets back.

## Why this is the smallest viable change

- One file changes.
- No new utility, no new hook, no new component.
- Writes happen on user interaction only — no extra effects.
- The lazy `useState` initialiser runs once per mount, same as labels.
- No backend, no migration, no feature flag.

## Risks / non-issues

- **Cross-tab sync.** If the user has two tabs open, toggling in one will
  not update the other until the second tab remounts the form. This is
  also true of `taskLabels` and the prompt draft; we're not introducing a
  regression. A `storage` event listener is unwarranted complexity for
  two checkboxes.
- **Privacy / shared machines.** `localStorage` is per-browser-profile
  per-origin; this is the same trust model as the existing draft and
  labels.
- **First-ever mount.** Returns `false` for both — same as today's
  defaults. No visible change for new users.

## Implementation notes (discovered during work)

- **Keyboard shortcut also toggles `justDoItMode`.** There's a `Ctrl/Cmd+J`
  global shortcut in a `useEffect` around line 484 that does
  `setJustDoItMode((prev) => !prev)`. It must also route through
  `handleJustDoItChange` so keyboard-triggered toggles persist the same as
  mouse clicks. Otherwise users hitting the shortcut would silently bypass
  the localStorage write. Updated the effect to call
  `handleJustDoItChange(!justDoItMode)` and added `justDoItMode` +
  `handleJustDoItChange` to its dependency array.
- **`yarn build` blocked by a sandbox `dist/` permission issue** unrelated
  to this change (`EACCES: permission denied, mkdir '.../frontend/dist/external-libs'`).
  The Vite transform of all 21104 modules completed successfully before
  failing on the output write. `yarn tsc --noEmit` is the more meaningful
  signal here — it passes clean.

## Testing

- Manual: tick "Skip planning", create a task, observe the next form
  still has it ticked. Reload the page, observe still ticked. Untick,
  create another task, observe unticked persists.
- Manual: tick both, switch projects, observe both still ticked.
- Manual: open DevTools → Application → Local Storage, corrupt the value
  to `not-json`, reload, confirm the form mounts with `false` and does
  not throw.
- `cd frontend && yarn build` must succeed (TS types).
- End-to-end in inner Helix at `http://localhost:8080` per the project's
  CLAUDE.md "Never Give Up on Testing" rule: register
  `test@helix.ml` / `helixtest`, complete onboarding, open the new spec
  task form, exercise all four combinations of the two checkboxes.
