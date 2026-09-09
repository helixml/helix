# Speed up and paginate the admin organizations list

## Summary
Add server-side pagination and debounced organization search to the admin organizations table. Limit expensive organization detail loading to the current page and run membership and project lookups concurrently, substantially reducing initial load time while preserving the existing search behavior.

Regenerate the OpenAPI documentation and TypeScript API client for the paginated response, and add standard page-size and navigation controls to the UI.

## Testing
- `yarn tsc` passed.
- Focused admin organization backend tests passed.
- The full server test suite was run; unrelated SQLite tests failed because the environment uses `CGO_ENABLED=0` while `go-sqlite3` requires CGO.
- `git diff --check` passed.
