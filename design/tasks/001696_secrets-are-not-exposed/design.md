# Design: Secrets Not Exposed to Human Desktop (Bug Fix)

## Current State

The spec task desktop receives secrets via `DesktopAgentAPIEnvVars()` in `api/pkg/external-agent/hydra_executor.go`:

```go
func DesktopAgentAPIEnvVars(apiKey string) []string {
    return []string{
        fmt.Sprintf("USER_API_TOKEN=%s", apiKey),
        fmt.Sprintf("ANTHROPIC_API_KEY=%s", apiKey),
        fmt.Sprintf("OPENAI_API_KEY=%s", apiKey),
        fmt.Sprintf("ZED_HELIX_TOKEN=%s", apiKey),
    }
}
```

However, the human/exploratory desktop does NOT call this function or equivalent, so it lacks the necessary environment variables for AI tools to function.

## Root Cause

The human desktop startup path is missing the secret injection that the spec task path has. Need to identify where human desktop env vars are set and add the missing call to `DesktopAgentAPIEnvVars()`.

## Proposed Solution

1. **Find the human desktop startup code path** - likely a different executor or entry point than spec tasks
2. **Add `DesktopAgentAPIEnvVars()` call** to inject secrets into human desktop environment
3. **Ensure user's API token is available** at that point in the code

## Files to Investigate

| File | Purpose |
|------|---------|
| `api/pkg/external-agent/hydra_executor.go` | Has `DesktopAgentAPIEnvVars()` - used by spec task |
| `api/pkg/external-agent/` | Look for human desktop executor |
| `api/pkg/server/` | Handlers that start human desktops |

## Codebase Pattern

The existing `DesktopAgentAPIEnvVars()` function is the correct pattern. The fix is calling it from the human desktop path, not creating something new.

## Risk

Low risk - this is adding existing functionality to a missing code path. No architectural changes needed.
