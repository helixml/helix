# Phase 3 — Recipe file browser + auto-derived name

## Problem

Configuring a Goose recipe today asks the user for two things, both
typed by hand:

- **Slash command name** — `release-notes`
- **Recipe path** — `.goose/recipes/release-notes.yaml`

This produces a class of failure where the project YAML validates but
the recipe loader returns `"recipe file not found in repo"` at session
start. Observed failure modes from real use:

- User typed `examples/goose_recipes` (directory, not file).
- User typed `examples/goose_recipes/release-notes.yaml` while the
  attached primary repo was a different repo that didn't have that path.
- User typed `release_notes` for the name but the file used hyphens.
- User typed the slash command with a leading `/`.

The free-text path is fragile: there is no signal that the path doesn't
exist on the attached repo until the daemon runs at session start, and
the `name` field is a parallel piece of state that the user must keep
in sync with the path.

## Decision

Two paired changes:

1. **File browser.** Replace the free-text path field in
   `GooseRecipesEditor` with a picker populated from a new backend
   endpoint that lists candidate `*.yaml` files under the
   recipe-repo `LocalPath`. The picker filters to YAML so the user
   can't pick a markdown file by accident. No tree view in v1 — flat
   list of file paths is enough; recipes already live in a small
   number of conventional directories (`.goose/recipes/`,
   `examples/goose_recipes/`).
2. **Auto-derive name from filename.** Drop the `name` field as
   *required* input. The slash command name defaults to
   `filepath.Base(strings.TrimSuffix(path, ".yaml"))` —
   `release-notes.yaml` → `release-notes`. Keep an *optional* "name
   override" input for the (rare) case where two recipes share a
   basename across directories.

The user picks a file; the system fills in everything else.

## API

New endpoint:

```
GET /api/v1/projects/:id/goose-recipes/candidates
```

Returns the list of `*.yaml` files under the recipe-repo `LocalPath`,
filtered to a sensible set:

- Include `*.yaml` and `*.yml` only.
- Skip `node_modules/`, `vendor/`, `.git/` for noise.
- Cap at 200 entries — if a repo has more, the user almost certainly
  isn't storing recipes there.

Reuses `resolveGooseRecipeRoot` so error paths are consistent with the
existing `goose-recipes` endpoint (e.g. external repo without a clone
returns `"recipe repository has not been cloned yet"`).

Response shape:

```json
{
  "files": [
    { "path": ".goose/recipes/release-notes.yaml", "title": "Generate release notes" },
    { "path": ".goose/recipes/triage.yaml",        "title": "Triage CI failures" }
  ]
}
```

`title` is parsed best-effort from the YAML's `title:` field — when
parsing fails we still return the path with an empty title so the UI
can render the entry.

## Backend changes

- `AssistantGooseRecipe.Name` becomes optional in `applyProject`
  validation. When empty, the validator derives it via
  `defaultRecipeName(path)` and uses the derived value for the
  uniqueness check. Persist the derived name explicitly so we don't
  need to re-derive in every downstream consumer.
- `defaultRecipeName(path string) string` is a single new helper in
  `api/pkg/goose/` — basename of path with `.yaml`/`.yml` extension
  stripped. Reused by the file-listing endpoint and the apply handler.
- `validateGooseAgentSpec` swaps the "non-empty name" check for "name
  must be valid identifier OR empty (derive)".
- New `listProjectGooseRecipeCandidates` handler in
  `goose_recipes_handlers.go` next to the existing
  `listProjectGooseRecipes`.

## Frontend changes

- `GooseRecipesEditor.tsx`:
  - Free-text path input becomes an `Autocomplete` (or `Select`)
    populated by the candidates endpoint when the editor mounts.
  - Slash command field becomes optional with a placeholder showing
    the derived value (`release-notes`).
  - Loading and error states surface to the user — if the repo isn't
    cloned yet, render the existing helper text rather than an empty
    dropdown.

## Backwards compatibility

- Existing projects with explicit `name` in YAML keep working — the
  field is still accepted, just not required.
- Existing projects in the DB keep their explicit names; the deriver
  is only applied when a new recipe entry is added without one.

## Out of scope (Phase 4+)

- Tree-view file browser. Flat list with autocomplete is enough.
- Inline recipe preview/edit. Recipes are still authored in your
  normal repo flow; the project UI stays read-only declarative.
- Cross-recipe deduplication when two files share a basename across
  directories. v1 surfaces a validation error and asks the user to
  set an explicit name override.
