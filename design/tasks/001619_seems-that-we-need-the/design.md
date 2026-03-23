# Design: Add `workflow` Scope to GitHub Auth

## Context

GitHub's `workflow` scope is required to push files under `.github/workflows/`. It is NOT included in `repo` — you must request it explicitly. Classic PATs need it checked at validation time; OAuth flows need it in the scope list.

## Files to Change

### 1. Backend: PAT validation
**`api/pkg/server/git_provider_connection_handlers.go`** — `validateAndFetchUserInfo()` (lines ~308-317)

Currently checks only for `repo`. Add parallel check for `workflow`:

```go
hasRepo := false
hasWorkflow := false
for _, s := range scopes {
    if s == "repo" { hasRepo = true }
    if s == "workflow" { hasWorkflow = true }
}
var missing []string
if !hasRepo { missing = append(missing, "repo") }
if !hasWorkflow { missing = append(missing, "workflow") }
if len(missing) > 0 {
    return nil, fmt.Errorf("GitHub token is missing required scopes: %s. Your token has: %s. Create a classic token at https://github.com/settings/tokens", strings.Join(missing, ", "), strings.Join(scopes, ", "))
}
```

### 2. GitHub Skill YAML files
**`api/pkg/agent/skill/api_skills/github.yaml`** and **`api/pkg/agent/skill/api_skills/github_issues.yaml`**

Add `workflow` to the `oauth.scopes` list:
```yaml
oauth:
  provider: github
  scopes:
    - repo
    - user:read
    - workflow
```

### 3. Frontend: PAT helper text
**`frontend/src/components/project/forms/ExternalRepoForm.tsx`** (line ~224)

Update helper text from `(needs repo scope for private repos)` to `(needs repo and workflow scopes)`.

## Notes

- The `workflow` scope on GitHub grants write access to `.github/workflows/` files only. It does NOT grant broader permissions.
- Fine-grained PATs still get rejected (they don't return `X-OAuth-Scopes`), so no change needed there.
- OAuth provider configuration (in database/admin) does not need changes since scopes are passed per-flow from the skill YAML files.
- No changes needed to `oauth2.go` or `manager.go` — the scope plumbing already works; we just need to include `workflow` in the lists.
