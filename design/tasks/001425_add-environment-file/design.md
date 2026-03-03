# Design: Project Secret Files

## Overview

Extend the existing project secrets system to support **files** in addition to environment variables. Secret files are stored encrypted in the database, written to the workspace at session start, and auto-gitignored.

## Architecture

### Existing Pattern (Project Secrets → Env Vars)

```
Project Settings UI → API (encrypted) → Database
                                              ↓
Session Start → GetProjectSecretsAsEnvVars() → DesktopAgent.Env[]
```

### New Pattern (Project Secret Files → Workspace Files)

```
Project Settings UI → API (encrypted) → Database (SecretFile table)
                                              ↓
Session Start → GetProjectSecretFiles() → Write to workspaceDir
                                        → Update .gitignore
```

## Data Model

### New Type: `SecretFile`

```go
// api/pkg/types/types.go
type SecretFile struct {
    ID        string    `json:"id" gorm:"primaryKey"`
    ProjectID string    `json:"project_id" gorm:"index"`
    Path      string    `json:"path"`              // e.g., ".env", "config/secrets.json"
    Content   []byte    `json:"content" gorm:"-"`  // Decrypted (never persisted)
    Value     []byte    `json:"-" gorm:"type:bytea"` // Encrypted
    Owner     string    `json:"-"`
    OwnerType OwnerType `json:"-"`
    Created   time.Time `json:"created"`
    Updated   time.Time `json:"updated"`
}
```

Re-uses existing encryption infrastructure (`pkg/encryption`).

## API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/projects/{id}/secret-files` | List secret files (paths only) |
| POST | `/api/v1/projects/{id}/secret-files` | Create secret file |
| GET | `/api/v1/projects/{id}/secret-files/{file_id}` | Get file (decrypted content) |
| PUT | `/api/v1/projects/{id}/secret-files/{file_id}` | Update file content |
| DELETE | `/api/v1/projects/{id}/secret-files/{file_id}` | Delete file |

## Key Implementation Points

### 1. File Injection at Session Start

In `hydra_executor.go` `StartDesktop()`, after workspace directory is created:

```go
// Write secret files to workspace (before startup script runs)
if err := h.writeSecretFiles(ctx, agent.ProjectID, workspaceDir); err != nil {
    log.Warn().Err(err).Msg("Failed to write secret files")
}
```

### 2. Auto-Gitignore

When writing files, also update `.gitignore`:

```go
func updateGitignore(workspaceDir string, paths []string) error {
    gitignorePath := filepath.Join(workspaceDir, ".gitignore")
    // Read existing, append missing paths with "# Helix secret files" header
}
```

### 3. Frontend Integration

Add "Secret Files" section to `ProjectSettings.tsx` below existing "Secrets" section:
- List with file path, "encrypted" chip, delete button
- Add dialog with path input + multiline content editor
- Edit dialog (loads decrypted content)

## Security Considerations

- Content encrypted at rest using existing `pkg/encryption`
- Decrypted content never logged
- API requires project update permission
- Files written with 0600 permissions

## File Changes

| File | Change |
|------|--------|
| `api/pkg/types/types.go` | Add `SecretFile` type |
| `api/pkg/store/store.go` | Add CRUD methods for SecretFile |
| `api/pkg/store/postgres.go` | Implement SecretFile store methods |
| `api/pkg/server/secret_files_handlers.go` | New: API handlers |
| `api/pkg/server/server.go` | Register routes, add callback |
| `api/pkg/external-agent/hydra_executor.go` | Write files at session start |
| `frontend/src/pages/ProjectSettings.tsx` | Add Secret Files UI section |