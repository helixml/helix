# Handle duplicate project secrets without crashing settings

## Summary
Return duplicate secret creation as an HTTP 409 conflict instead of an internal server error, and safely display either plain-text or JSON API errors in project settings. This keeps the page usable and gives the user a clear message when a secret name already exists in the selected environment.

## Testing
- `go test ./api/pkg/server -run '^(TestCreateProjectSecretReturnsConflictForDuplicate|TestProjectSecretRoutesNameMissingProject)$' -count=1` passed.
- `yarn tsc` passed in `frontend`.
- The PostgreSQL-backed store test could not run because PostgreSQL is not configured in the sandbox; its existing duplicate-secret test now asserts the conflict sentinel for CI coverage.
