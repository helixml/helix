# Add claude-opus-4-7 to recommended models in Advanced Model Picker

## Summary
`claude-opus-4-7` (Anthropic's latest Opus) is now starred and sorted to the top of the Anthropic group in the Advanced Model Picker, alongside the existing 4.6 / 4.5 entries.

## Changes
- `frontend/src/constants/models.ts`: add `"claude-opus-4-7"` as the first entry in the `// Anthropic` block of `RECOMMENDED_CODING_MODELS` (newer-first ordering, matches the file's existing convention).

That's the entire diff — one line. The picker already drives starring and sort order from this constant, so the change automatically flows to all five host surfaces (Onboarding, ProjectSettings, NewSpecTaskForm, CreateProjectDialog, AgentSelectionModal). No backend / `model_info.json` change is required: the picker renders whatever Anthropic returns from `/v1/models` and stars matching IDs.

## Verification
- `cd frontend && yarn build` — succeeds (built via `npx vite build --outDir /tmp/...` because the in-repo `dist/` is bind-mounted root-owned by the Vite dev container; all 21066 modules transformed cleanly).
- Smoke-tested in the inner Helix at `http://localhost:8080` (Onboarding → Create your first project → Code Agent Model). `claude-opus-4-7` appears with the gold star icon, immediately below the currently-selected default. Search for "opus-4-7" resolves it.

## Screenshots
![Advanced Model Picker showing claude-opus-4-7 starred](https://github.com/helixml/helix/raw/helix-specs/design/tasks/001914_add-claude-opus-4-7-as/screenshots/01-picker-showing-opus-4-7.png)

![Search filter for opus-4-7](https://github.com/helixml/helix/raw/helix-specs/design/tasks/001914_add-claude-opus-4-7-as/screenshots/02-picker-search-opus-4-7.png)

## Design doc
`design/tasks/001914_add-claude-opus-4-7-as/` on the `helix-specs` branch.
