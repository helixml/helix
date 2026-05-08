# Requirements

## User Story

As a Helix user picking a model in the **Advanced Model Picker**, I want to see
`claude-opus-4-7` as one of the starred (recommended) models so that I can
discover and select Anthropic's latest Opus model the same way I select the
other current Claude models.

## Background

The advanced model picker
(`frontend/src/components/create/AdvancedModelPicker.tsx`) sorts and stars
models whose IDs appear in the `RECOMMENDED_CODING_MODELS` constant in
`frontend/src/constants/models.ts`. This constant is hardcoded and currently
lists `claude-opus-4-6`, `claude-sonnet-4-6`, `claude-haiku-4-6`, and the older
4.5 variants — but not the newer `claude-opus-4-7`.

The picker is consumed by 5 surfaces (Onboarding, ProjectSettings,
NewSpecTaskForm, CreateProjectDialog, AgentSelectionModal); they all import the
same constant, so a single edit propagates everywhere.

## Acceptance Criteria

- [ ] `claude-opus-4-7` appears in the `RECOMMENDED_CODING_MODELS` array in
  `frontend/src/constants/models.ts`.
- [ ] It is placed in the Anthropic group (under the `// Anthropic` comment),
  ordered by preference — i.e. **above** `claude-opus-4-6` since 4.7 is newer.
- [ ] In the Advanced Model Picker dialog (any of the 5 host surfaces), when
  Anthropic returns `claude-opus-4-7` from `/v1/models`, the entry shows the
  gold star icon and is sorted near the top alongside the other recommended
  models.
- [ ] No other recommended models are removed or reordered.
- [ ] `cd frontend && yarn build` passes.

## Out of Scope

- Adding metadata for `claude-opus-4-7` to
  `api/pkg/model/model_info.json`. The picker only requires the ID to be in
  the recommended list; it does not require an entry in `model_info.json`. (If
  pricing/context-length display becomes a concern after the model lands, that
  can be a follow-up task.)
- Changing how recommendations are stored (no DB table, no admin UI — the
  hardcoded constant is the existing pattern and we keep it).
- Adding `claude-sonnet-4-7` / `claude-haiku-4-7` — the request is for Opus
  only.
