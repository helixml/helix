# Generate descriptive titles for Just Do It tasks

## Summary
Generate a concise task title asynchronously when a Just Do It task enters implementation, so build-only tasks become easy to find without depending on design documents or reaching the pull request stage.

Reuse the existing session title generation path for provider selection, model invocation, cleanup, and truncation. Preserve manual renames with a stale-write guard, while retaining PR metadata titles as a later refinement.

## Testing
- `go test ./api/pkg/services -count=1`
- `go test ./api/pkg/server -run '^TestCleanGeneratedTitle$' -count=1`
- Full `api/pkg/server` suite was also run; two unrelated tests require a CGO-enabled SQLite build in this environment.
