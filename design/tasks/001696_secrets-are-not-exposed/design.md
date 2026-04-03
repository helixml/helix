# Design: Secrets Not Exposed to Human Desktop (Bug Fix)

## Root Cause Found

The spec task desktop injects project secrets via `GetProjectSecretsAsEnvVars()` (lines 554-561 in `spec_driven_task_service.go`):

```go
// Inject project secrets as environment variables
if s.GetProjectSecrets != nil && task.ProjectID != "" {
    projectSecrets, err := s.GetProjectSecrets(ctx, task.ProjectID)
    if err != nil {
        log.Warn().Err(err).Str("project_id", task.ProjectID).Msg("Failed to get project secrets, continuing without them")
    } else if len(projectSecrets) > 0 {
        envVars = append(envVars, projectSecrets...)
        log.Info().Int("secret_count", len(projectSecrets)).Str("project_id", task.ProjectID).Msg("Injected project secrets into desktop env")
    }
}
```

However, the exploratory session (`startExploratorySession` in `project_handlers.go`) does NOT have this code. It only adds:
- User API tokens via `addUserAPITokenToAgent()`
- But NOT project secrets

## Solution

Add project secret injection to the exploratory session startup path in `project_handlers.go`, using the existing `GetProjectSecretsAsEnvVars()` function.

## Files to Modify

| File | Change |
|------|--------|
| `api/pkg/server/project_handlers.go` | Add project secrets injection in both restart and new session paths |

## Implementation Notes

- The `GetProjectSecretsAsEnvVars` function already exists in `secrets_handlers.go`
- Need to call it for both code paths in `startExploratorySession`:
  1. Restart path (existing session, container stopped) - around line 1391
  2. New session path - around line 1546
- Secrets are added to `zedAgent.Env` before calling `addUserAPITokenToAgent()`
