# Sync Just Do It task names from agent thread titles

## Summary
Reuse the coding agent's existing thread-title event to rename its associated Just Do It task near the beginning of implementation. This gives build-only tasks concise, searchable names without requiring a planning artifact, PR metadata, or a separately configured enrichment model.

Planning tasks retain their existing requirements-title behavior, and explicit user title overrides are preserved.

## Testing
- `go test ./api/pkg/server -run 'TestWebSocketSyncSuite/TestThreadTitleChanged' -count=1`
