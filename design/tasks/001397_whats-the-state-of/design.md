# Design: Project Secrets Architecture

## Overview

Project secrets allow users to store encrypted credentials (API keys, tokens, etc.) that are automatically injected as environment variables into desktop containers when agents work on that project.

## Current Architecture

```
┌─────────────────────┐     ┌──────────────────────┐     ┌─────────────────────┐
│   Frontend UI       │────▶│   API Server         │────▶│   PostgreSQL        │
│ ProjectSettings.tsx │     │ secrets_handlers.go  │     │   secrets table     │
└─────────────────────┘     └──────────────────────┘     └─────────────────────┘
                                      │
                                      │ GetProjectSecretsAsEnvVars()
                                      ▼
                            ┌──────────────────────┐
                            │ SpecDrivenTaskService│
                            │ (StartSpecGeneration │
                            │  StartJustDoItMode)  │
                            └──────────────────────┘
                                      │
                                      │ DesktopAgent.Env
                                      ▼
                            ┌──────────────────────┐
                            │  HydraExecutor       │
                            │  (container launch)  │
                            └──────────────────────┘
```

## Key Components

### Database Schema (`types.Secret`)
```go
type Secret struct {
    ID        string    // Primary key (sec_xxx)
    Owner     string    // User who created the secret
    OwnerType OwnerType // Always "user" for now
    Name      string    // Env var name (e.g., STRIPE_SECRET_KEY)
    Value     []byte    // AES-256-GCM encrypted value
    ProjectID string    // Associated project
    AppID     string    // Optional app association (unused for project secrets)
}
```

### Encryption (`crypto/encryption.go`)
- Algorithm: AES-256-GCM
- Key source: `HELIX_ENCRYPTION_KEY` env var
- Key derivation: SHA-256 hash if not exactly 32 bytes hex
- Format: Base64-encoded (nonce + ciphertext)

### API Endpoints
| Method | Path | Handler | Purpose |
|--------|------|---------|---------|
| GET | `/api/v1/projects/{id}/secrets` | `listProjectSecrets` | List secrets (no values) |
| POST | `/api/v1/projects/{id}/secrets` | `createProjectSecret` | Create encrypted secret |
| DELETE | `/api/v1/secrets/{id}` | `deleteSecret` | Remove secret |

### Injection Flow

1. **Task starts** → `StartSpecGeneration()` or `StartJustDoItMode()`
2. **Fetch secrets** → `GetProjectSecrets(ctx, task.ProjectID)`
3. **Decrypt** → `crypto.DecryptAES256GCM()` for each secret
4. **Format** → `fmt.Sprintf("%s=%s", secret.Name, decryptedValue)`
5. **Append to env** → `envVars = append(envVars, projectSecrets...)`
6. **Pass to container** → `DesktopAgent.Env` → `HydraExecutor.buildEnvVars()`

## Security Model

### At Rest
- Encrypted with AES-256-GCM before storage
- Key managed via `HELIX_ENCRYPTION_KEY` environment variable

### In Transit
- HTTPS for API calls
- Values never returned after creation (only name + metadata)

### Authorization
- Create/List: Must have project access (`authorizeUserToProjectByID`)
- Delete: Must own secret OR have project delete permission
- Injection: No auth check (server-side operation during task start)

## Design Decisions

### Why project-scoped secrets (not organization or user)?
Project secrets are the right granularity because:
- Different projects need different credentials (dev vs staging)
- Secrets shouldn't leak between unrelated projects
- Organization-level would over-share; user-level would require per-project overrides anyway

### Why inject at task start (not on-demand)?
- Simpler: env vars set once at container creation
- Secure: no runtime secret-fetching API needed in container
- Compatible: standard env var pattern works with any tool/SDK

### Why auto-uppercase secret names?
- Convention: env vars traditionally uppercase
- Consistency: prevents `stripe_key` vs `STRIPE_KEY` confusion
- Frontend enforces: `toUpperCase().replace(/[^A-Z0-9_]/g, "_")`

## Files Reference

| File | Purpose |
|------|---------|
| `api/pkg/server/secrets_handlers.go` | API handlers, `GetProjectSecretsAsEnvVars()` |
| `api/pkg/store/store_secrets.go` | Database operations |
| `api/pkg/crypto/encryption.go` | AES-256-GCM encrypt/decrypt |
| `api/pkg/services/spec_driven_task_service.go` | Injection at task start |
| `api/pkg/types/types.go` | `Secret` struct definition |
| `frontend/src/pages/ProjectSettings.tsx` | UI for managing secrets |

## Conclusion

**No implementation needed** — project secrets are fully functional. Users can add secrets in Project Settings and they will be available as environment variables in all desktop sessions for that project.