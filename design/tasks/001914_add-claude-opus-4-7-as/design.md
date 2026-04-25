# Design

## Where the recommended list lives

`frontend/src/constants/models.ts` exports a single hardcoded array,
`RECOMMENDED_CODING_MODELS`, ordered by preference. The
`AdvancedModelPicker` component:

- accepts a `recommendedModels?: string[]` prop,
- sorts entries whose `id` is in that list to the top of the dialog, and
- renders a gold `StarIcon` next to them
  (`AdvancedModelPicker.tsx` lines ~310–335 for sort, ~552–562 for the icon).

Five surfaces import `RECOMMENDED_CODING_MODELS` and pass it as the prop:

- `frontend/src/pages/Onboarding.tsx`
- `frontend/src/pages/ProjectSettings.tsx`
- `frontend/src/components/tasks/NewSpecTaskForm.tsx`
- `frontend/src/components/project/CreateProjectDialog.tsx`
- `frontend/src/components/project/AgentSelectionModal.tsx`

Because the constant is the single source of truth, **one edit covers every
surface**. No component changes are needed.

## The change

Add `"claude-opus-4-7"` as the first entry in the `// Anthropic` block of
`frontend/src/constants/models.ts`:

```ts
export const RECOMMENDED_CODING_MODELS = [
  // Anthropic
  "claude-opus-4-7",          // <-- new (newest, listed first)
  "claude-opus-4-6",
  "claude-sonnet-4-6",
  "claude-haiku-4-6",
  "claude-opus-4-5-20251101",
  "claude-sonnet-4-5-20250929",
  "claude-haiku-4-5-20251001",
  // ... (rest unchanged)
];
```

## Key decisions

1. **Frontend-only change.** The picker shows whatever the Anthropic provider
   returns from `/v1/models` and stars entries that match the hardcoded ID.
   `claude-opus-4-7` is a real Anthropic model ID, so once Anthropic exposes it
   (or it's already exposed) the picker will list it; the constant edit just
   adds the star + top-sort. No backend code path requires a corresponding
   entry — `provider_handlers.go` enriches with `model_info.json` metadata
   when present and returns the model regardless when absent.

2. **Ordering: 4.7 above 4.6.** The file's own comment says "ordered by
   preference," and existing entries follow newer-first within each provider
   group. Putting 4.7 at the top of the Anthropic block matches that pattern
   and makes it the default visual choice.

3. **No `model_info.json` update in this task.** Adding a full entry there is a
   ~50-line JSON block that affects pricing display, billing gating, and
   context-length hints. It is orthogonal to "make this model show as
   starred." Keeping scope tight avoids dragging billing/pricing decisions
   into a one-line UI change. A follow-up task can add the metadata if/when
   pricing data is available.

4. **No DB / admin-UI change.** The current pattern is a hardcoded constant.
   Introducing a database-backed "starred models" table would be a much
   larger refactor and is not justified by this request.

## Verification

- `cd frontend && yarn build` — confirms the file still type-checks and
  bundles.
- Manual smoke test in the inner Helix at `http://localhost:8080`:
  open any page that hosts the AdvancedModelPicker (e.g. New Project →
  Advanced model picker), confirm `claude-opus-4-7` is listed with the gold
  star and sorted near the top of the Anthropic group.

## Notes for future agents

- The "starred / recommended" mechanism in this codebase is **purely a
  frontend constant** (`frontend/src/constants/models.ts`). There is no
  database table and no admin endpoint. Look here first for any future
  add/remove/reorder requests.
- The picker degrades gracefully when a recommended ID isn't returned by any
  provider — it just won't appear. So adding an ID before the model is
  available from the provider is harmless.
- `model_info.json` (in `api/pkg/model/`) is a separate registry used for
  pricing/context metadata. It is **not required** for a model to appear in
  the picker.
