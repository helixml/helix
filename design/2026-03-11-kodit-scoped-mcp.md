# Kodit Feature Request: Scoped MCP Handler

**Date:** 2026-03-11
**Status:** Implemented (kodit v1.1.7)
**Author:** Helix team

## Problem

Helix's Kodit MCP backend (`mcp_backend_kodit.go`) forwarded every request to the
in-process Kodit MCP server with no filtering. Any agent could see all repositories
across all organizations. We needed to restrict MCP access to only the repositories
belonging to the current session's project.

A previous attempt reimplemented all 14 Kodit MCP tool handlers inside Helix
(~1088 lines duplicated from Kodit). This was rejected — the tool definitions,
instructions, handler logic, URI building, and result formatting belong in Kodit,
not Helix. Helix should only be responsible for resolving which repository IDs are
allowed and passing them to Kodit.

## API

Kodit v1.1.7 added:

```go
// NewScopedMCPHandler creates an MCP HTTP handler where all operations are
// restricted to the given set of Kodit repository IDs.
func NewScopedMCPHandler(client *Client, repoIDs []int64) http.Handler
```

When `repoIDs` is empty or nil, the handler behaves identically to the
unscoped MCP handler (all repos visible).

## Helix Integration

```go
scope := resolveKoditRepoScope(ctx, store, sessionID, user)
handler := kodit.NewScopedMCPHandler(koditClient, scope.idSlice)
handler.ServeHTTP(w, r)
```

## Scoped Interfaces

Kodit wraps 6 of its internal interfaces with decorator types:

| Interface | Scoping strategy |
|-----------|-----------------|
| `RepositoryLister` | Inject `repository.WithIDIn(allowedIDs)` into `Find()` calls |
| `SemanticSearcher` | Merge `search.WithSourceRepos(allowedIDs)` into filters |
| `KeywordSearcher` | Same + intersect caller-requested repos with allowed set |
| `FileContentReader` | Validate `repoID` before delegating |
| `FileLister` | Validate `repoID` before delegating |
| `Grepper` | Validate `repoID` before delegating |

The remaining interfaces (`CommitFinder`, `EnrichmentQuery`, `EnrichmentResolver`,
`FileFinder`) need no wrapping — they're always called downstream from an
already-validated repo.
