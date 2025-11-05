# Helix Development Rules

See also: @.cursor/rules/helix.mdc, @.cursor/rules/go-api-handlers.mdc, @.cursor/rules/use-gorm-for-database.mdc, @.cursor/rules/use-frontend-api-client.mdc

## CRITICAL: Fail Fast with Clear Errors

**NEVER write fallback code or silently continue after failures**

```go
// ❌ WRONG: Hiding failures
if err != nil {
    log.Warn().Err(err).Msg("Failed to setup worktree (continuing)")
}

// ✅ CORRECT: Fail fast
if err != nil {
    return fmt.Errorf("failed to setup design docs worktree: %w", err)
}
```

**Why:** Fallbacks hide problems, confuse debugging, waste time. This is customer-facing software.

## Documentation Organization

- **`design/`**: LLM-generated docs, architecture decisions, debugging logs. Format: `YYYY-MM-DD-descriptive-name.md`
- **`docs/`**: User-facing documentation only
- **Root**: Only `README.md`, `CONTRIBUTING.md`, `CLAUDE.md`

## Hot Reloading Stack

Frontend (Vite), API (Air), GPU Runner, Wolf, Zed all support hot reloading. Save files → changes picked up automatically.

## CRITICAL: Always Verify Build Status

After ANY code changes:

```bash
# Check API build
docker compose -f docker-compose.dev.yaml logs --tail 30 api
# Look for: "building..." → "running..." (success) or "failed to build" (error)

# Check frontend build
docker compose -f docker-compose.dev.yaml logs --tail 30 frontend
# Look for: "✓ built in XXms" or TypeScript errors
```

**ONLY declare complete after checking logs.** Compilation errors = broken code.

## Zed Build Process

```bash
# ✅ CORRECT: Use stack script
./stack build-zed        # Dev mode (fast, ~1.3GB)
./stack build-zed release # Release mode (slow, ~2GB)

# ❌ WRONG: Missing feature flag
cargo build --package zed
```

**Kill old builds first:** `pkill -f "cargo build" && pkill -f rustc`

**Hot reload:** Kill builds → Build with stack → Close Zed window → Auto-restart in 2s

## Sway Container Build

```bash
./stack build-sway  # ✅ CORRECT
```

Rebuild when: modifying `wolf/sway-config/`, `Dockerfile.sway-helix`, Go daemons, Sway configs.
**New sessions use updated image; existing containers don't.**

## Testing & Mocking

```bash
# ✅ Use gomock
mockgen -source api/pkg/external-agent/wolf_client_interface.go \
  -destination api/pkg/external-agent/wolf_client_interface_mocks.go \
  -package external_agent

# ❌ NEVER use testify/mock manually
```

## Key Development Rules

1. **Customer-facing software**: Must work on fresh installs, no manual setup
2. **All builds in containers**: Never check host packages
3. **Foreground builds only**: Never use `run_in_background: true` with builds
4. **One build at a time**: Wait for completion before starting another
5. **Host ≠ Container**: Host=Ubuntu 24.04, containers=Ubuntu 25.04
6. **Trust Docker cache**: NEVER use `--no-cache`
7. **Never clear BuildKit cache**: Cache is reliable; investigate root causes
8. **No unauthorized images**: Never build/push versioned images without permission
9. **Test after every change**: Big-bang approaches impossible to debug
10. **Check logs after changes**: Verify hot reload succeeded

## Wolf Development

```bash
./stack rebuild-wolf  # Rebuild Wolf (~30s)
./stack start        # Auto-builds Wolf if missing
```

**Wolf API (from API container only):**
```bash
docker compose -f docker-compose.dev.yaml exec api \
  curl --unix-socket /var/run/wolf/wolf.sock http://localhost/api/v1/apps
```

**Wolf app storage:**
- Static apps (config.toml): Persist across restarts
- Dynamic apps (API created): Cleared on restart
- Restart Wolf to clear broken apps

**Wolf version:** Upstream wolf-ui + auto-pairing PIN support

## Generated TypeScript Client & React Query

**MANDATORY: Use generated client + React Query**

```typescript
// ❌ WRONG
const response = await api.get('/api/v1/spec-tasks/board-settings')

// ✅ CORRECT
const { data } = useQuery({
  queryKey: ['board-settings'],
  queryFn: () => apiClient.v1SpecTasksBoardSettingsList(),
})
```

**Regenerate client:** `./stack update_openapi`

**Required Swagger annotations:**
```go
// @Summary List personal development environments
// @Description Get all personal development environments
// @Tags PersonalDevEnvironments
// @Success 200 {array} PersonalDevEnvironmentResponse
// @Router /api/v1/personal-dev-environments [get]
// @Security ApiKeyAuth
```

**React Query requirements:**
- Use for ALL API calls (queries + mutations)
- Proper query keys for cache management
- Invalidate queries after mutations
- Handle loading/error states

## Frontend UX

**Never use `type="number"`** - Spinners have terrible UX. Use text inputs + `parseInt()`/`parseFloat()`

**Extract reusable components** - Never duplicate complex UI logic

## Wolf Streaming

**Two use cases:**
1. **External Agents (PRIMARY)**: AI agents work autonomously, user connection optional
2. **Personal Dev Environments**: User workspace, connection required

**Testing External Agents:**
1. Navigate to "External Agents" → "Start Session"
2. Send message → verify bidirectional sync
3. Check logs: `docker compose -f docker-compose.dev.yaml logs --tail 50 api`

**Working XFCE config (tested):**
```json
{
  "type": "docker",
  "image": "ghcr.io/games-on-whales/xfce:edge",
  "env": ["GOW_REQUIRED_DEVICES=/dev/input/* /dev/dri/* /dev/nvidia*"],
  "base_create_json": {
    "HostConfig": {
      "IpcMode": "host",
      "CapAdd": ["SYS_ADMIN", "SYS_NICE", "SYS_PTRACE", "NET_RAW", "MKNOD", "NET_ADMIN"],
      "SecurityOpt": ["seccomp=unconfined", "apparmor=unconfined"]
    }
  }
}
```

## Docker Compose

**Always use:** `docker compose -f docker-compose.dev.yaml`

**Restart vs Down+Up:**
```bash
# ✅ For config/image changes
docker compose -f docker-compose.dev.yaml down wolf
docker compose -f docker-compose.dev.yaml up -d wolf

# ❌ Only restarts, doesn't recreate
docker compose -f docker-compose.dev.yaml restart wolf
```

Use `restart` only for bind-mounted file changes.

## Database Migrations

**Use GORM AutoMigrate ONLY** - Never create SQL migration files for schema changes

```go
// ✅ CORRECT
type StreamingAccessGrant struct {
    ID        string `gorm:"type:varchar(255);primaryKey"`
    SessionID string `gorm:"type:varchar(255);index;not null"`
}
db.AutoMigrate(&StreamingAccessGrant{})

// ❌ WRONG: SQL migration files for schema changes
```

SQL migrations only for: complex data transformations, one-time cleanup, renaming tables/columns.

## RBAC - AccessGrants System

**ONE unified RBAC: AccessGrants + Roles + RoleBindings**

```go
// ✅ CORRECT
err := apiServer.authorizeUserToResource(ctx, user, orgID, projectID,
  types.ResourceProject, types.ActionUpdate)

// ❌ WRONG: Separate membership tables
type ProjectMembership struct { ... }
```

**Only membership tables:** `OrganizationMembership`, `TeamMembership` (implementation details)

**Adding new resource type:**
1. Add to `types.Resource` constants
2. Create authorization helper in `{resource}_access_grant_handlers.go`
3. Create access grant handlers (list/create/update/delete)
4. Register routes
5. Add Swagger docs
6. Run `./stack update_openapi`
7. Create React Query hooks
8. Implement frontend UI

## General Guidelines

- Never create files unless absolutely necessary
- Prefer editing existing files
- Never proactively create markdown/README files
