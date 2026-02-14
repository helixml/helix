# Claude Subscription Provider

**Date:** 2026-02-14
**Status:** Draft
**Branch:** `feature/claude-subscription-provider`

## Problem

Users with Claude Max/Pro subscriptions want to use their existing subscription for Claude Code inside Helix spec tasks, rather than paying separately for API tokens. Claude Code's OAuth tokens are restricted to Claude Code only (Anthropic returns "This credential is only authorized for use with Claude Code" if you try to call `/v1/messages` directly), so this is a specialized provider type that only works through Claude Code running inside Zed.

## Design

### Key Constraints

1. **Claude OAuth tokens only work through Claude Code** - not usable as generic Anthropic API keys
2. **Access tokens expire every 8 hours** - Claude Code handles refresh automatically
3. **Claude Code reads from `~/.claude/.credentials.json`** - we must write this file inside containers
4. **Concurrent containers work fine** - empirically, copied credentials work across multiple machines without conflict. Anthropic's refresh token rotation appears to have a grace period or token family approach

### Architecture

Claude Code handles its own token refresh natively. No centralized refresh service needed.

```
User pastes credentials (or completes OAuth in desktop session)
    |
    v
POST /api/v1/claude-subscriptions
    |
    v
API encrypts + stores in claude_subscriptions table
    |
    v
User starts SpecTask with claude_code runtime
    |
    v
Desktop container starts, settings-sync-daemon polls /zed-config
    -> Gets CodeAgentConfig with runtime="claude_code"
    -> Gets claude_subscription_available=true
    |
    v
Daemon calls GET /sessions/{id}/claude-credentials
    -> API decrypts + returns stored credentials
    |
    v
Daemon writes ~/.claude/.credentials.json inside container
    |
    v
Daemon generates agent_servers config: {"claude": {command: "claude", args: ["--experimental-acp"]}}
    |
    v
Zed launches Claude Code via ACP, Claude Code reads credentials file
    |
    v
Claude Code handles token refresh internally when needed
```

### Why Not Reuse ProviderEndpoint

Claude subscriptions are fundamentally different from regular provider endpoints:
- Tokens only work through Claude Code, not as generic API keys
- Credentials are OAuth tokens (access + refresh), not static API keys
- Need org-level sharing with a single set of credentials

## Implementation Plan

### Phase 1: Data Model and Storage

**New file: `api/pkg/types/claude_subscription.go`**

```go
type ClaudeSubscription struct {
    ID                    string         `json:"id" gorm:"primaryKey"`
    Created               time.Time      `json:"created"`
    Updated               time.Time      `json:"updated"`
    OwnerID               string         `json:"owner_id" gorm:"not null;index"`
    OwnerType             OwnerType      `json:"owner_type" gorm:"not null"` // "user" or "org"
    Name                  string         `json:"name"`
    EncryptedCredentials  string         `json:"-" gorm:"type:text;not null"`
    SubscriptionType      string         `json:"subscription_type"` // "max", "pro"
    RateLimitTier         string         `json:"rate_limit_tier"`
    Scopes                pq.StringArray `json:"scopes" gorm:"type:text[]"`
    AccessTokenExpiresAt  time.Time      `json:"access_token_expires_at"`
    Status                string         `json:"status"` // "active", "expired", "error"
    LastRefreshedAt       *time.Time     `json:"last_refreshed_at,omitempty"`
    LastError             string         `json:"last_error,omitempty"`
    CreatedBy             string         `json:"created_by" gorm:"not null"`
}

type ClaudeOAuthCredentials struct {
    AccessToken      string   `json:"accessToken"`
    RefreshToken     string   `json:"refreshToken"`
    ExpiresAt        int64    `json:"expiresAt"` // Unix millis
    Scopes           []string `json:"scopes"`
    SubscriptionType string   `json:"subscriptionType"`
    RateLimitTier    string   `json:"rateLimitTier"`
}
```

**Modify: `api/pkg/store/store.go`** - Add interface methods:
- `CreateClaudeSubscription(ctx, sub) (*ClaudeSubscription, error)`
- `GetClaudeSubscription(ctx, id) (*ClaudeSubscription, error)`
- `GetClaudeSubscriptionForOwner(ctx, ownerID, ownerType) (*ClaudeSubscription, error)`
- `UpdateClaudeSubscription(ctx, sub) (*ClaudeSubscription, error)`
- `DeleteClaudeSubscription(ctx, id) error`
- `ListClaudeSubscriptions(ctx, ownerID) ([]*ClaudeSubscription, error)`
- `GetEffectiveClaudeSubscription(ctx, userID, orgID) (*ClaudeSubscription, error)` - checks user-level first, then org

**New file: `api/pkg/store/store_claude_subscription.go`** - GORM implementations

**Modify: `api/pkg/store/postgres.go`** - Add `&types.ClaudeSubscription{}` to AutoMigrate

### Phase 2: API Endpoints

**New file: `api/pkg/server/claude_subscription_handlers.go`**

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/v1/claude-subscriptions` | Connect subscription (manual paste) |
| `GET` | `/api/v1/claude-subscriptions` | List user's + org's subscriptions |
| `GET` | `/api/v1/claude-subscriptions/{id}` | Get subscription details (no secrets) |
| `DELETE` | `/api/v1/claude-subscriptions/{id}` | Disconnect subscription |
| `POST` | `/api/v1/claude-subscriptions/{id}/refresh` | Force token refresh |
| `GET` | `/api/v1/claude-subscriptions/models` | List available Claude models (hardcoded) |
| `GET` | `/api/v1/sessions/{id}/claude-credentials` | Get fresh credentials for a container |

Create endpoint accepts:
```json
{
  "name": "My Claude Max",
  "owner_type": "user",
  "credentials": {
    "claudeAiOauth": {
      "accessToken": "sk-ant-oat01-...",
      "refreshToken": "sk-ant-ort01-...",
      "expiresAt": 1771082960335,
      "scopes": ["user:inference", "user:mcp_servers", "user:profile", "user:sessions:claude_code"],
      "subscriptionType": "max",
      "rateLimitTier": "default_claude_max_20x"
    }
  }
}
```

Models endpoint returns hardcoded list:
- `claude-opus-4-6` (Claude Opus 4.6)
- `claude-sonnet-4-5-latest` (Claude Sonnet 4.5)
- `claude-haiku-4-5-latest` (Claude Haiku 4.5)

Session credentials endpoint (`/sessions/{id}/claude-credentials`):
1. Looks up session to find user
2. Calls `GetEffectiveClaudeSubscription` (user-level, then org)
3. Decrypts and returns stored `ClaudeOAuthCredentials`
4. Only accepts runner/session-scoped tokens (same auth as `getZedConfig`)
5. Claude Code inside the container handles its own token refresh

**Modify: `api/pkg/server/server.go`** - Register new routes

### Phase 3: Settings-Sync-Daemon

**Modify: `api/cmd/settings-sync-daemon/main.go`**

Add `claude_code` case to `generateAgentServerConfig()` (line 94):

```go
case "claude_code":
    env := map[string]interface{}{
        "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1",
        "DISABLE_TELEMETRY": "1",
    }
    return map[string]interface{}{
        "claude": map[string]interface{}{
            "name":    "claude",
            "type":    "custom",
            "command": "claude",
            "args":    []string{"--experimental-acp"},
            "env":     env,
        },
    }
```

Add `syncClaudeCredentials()` method:
1. Check if `ClaudeSubscriptionAvailable` is true in ZedConfigResponse
2. Call `GET /api/v1/sessions/{id}/claude-credentials`
3. Write to `/home/retro/.claude/.credentials.json` in Claude's expected format
4. Called during initial sync AND on each 30-second poll cycle

**Modify: `api/pkg/types/types.go`** - Add to `ZedConfigResponse`:
```go
ClaudeSubscriptionAvailable bool `json:"claude_subscription_available,omitempty"`
```

### Phase 4: Zed Config Integration

**Modify: `api/pkg/server/zed_config_handlers.go`**

In `buildCodeAgentConfig()`, handle `claude_code` runtime:
- `baseURL = ""` (Claude Code talks directly to Anthropic, not through Helix proxy)
- `apiType = ""`
- `agentName = "claude"`

In `getZedConfig()`, after building codeAgentConfig:
- Check if runtime is `claude_code` AND user has effective Claude subscription
- Set `response.ClaudeSubscriptionAvailable = true`

### Phase 5: Desktop Image

**Modify: `Dockerfile.ubuntu-helix`**

Install Claude Code CLI (after the qwen-code section ~line 808):
```dockerfile
# Claude Code CLI (for ACP agent support)
RUN npm install -g @anthropic/claude-code@latest
```

The existing `~/.claude/settings.json` privacy config (line 674) already disables telemetry.

### Phase 6: Frontend UI

**New file: `frontend/src/components/dashboard/ClaudeSubscriptionSection.tsx`**

Shows:
- Current subscription status (connected/disconnected/expired)
- Subscription type (Max/Pro), rate limit tier
- Token expiry info
- "Connect Claude" button -> opens paste dialog
- "Disconnect" button
- "Refresh Token" button
- Available models list

**New file: `frontend/src/components/dashboard/ConnectClaudeDialog.tsx`**

Paste dialog:
1. Instructions for finding `~/.claude/.credentials.json`
2. Text area for pasting JSON
3. Client-side validation of structure
4. Supports both `{"claudeAiOauth": {...}}` (full file) and just the inner object
5. Owner type selector (user vs org, if user is org admin)

**Modify: Dashboard page** - Add Claude Subscription section to settings area

**Run: `./stack update_openapi`** after adding swagger annotations to new handlers

### Phase 7: Desktop OAuth Login Flow (v1 bonus)

For users who prefer not to manually paste credentials:

**Modify: Frontend** - "Connect via Desktop" button that:
1. Starts a special lightweight desktop session (no project, no spec task)
2. Inside session, a startup script runs `claude` which opens the OAuth URL in the in-session browser
3. User completes login
4. Startup script detects `~/.claude/.credentials.json` was written
5. Script calls Helix API to upload the credentials
6. Session auto-terminates

This is lower priority than the paste flow and can be deferred if needed.

## Security

- **Encryption at rest**: Credentials encrypted with AES-256-GCM via existing `crypto.EncryptAES256GCM`, keyed by `HELIX_ENCRYPTION_KEY`
- **No credentials in API responses**: `ClaudeSubscription` uses `json:"-"` on `EncryptedCredentials`
- **Session-scoped credential delivery**: `/sessions/{id}/claude-credentials` only accepts runner/session tokens
- **No credentials in env vars**: Written to file by daemon, not passed as container env vars
- **Org sharing**: Org admin connects once, members use it. `GetEffectiveClaudeSubscription` checks user-level first (takes priority)

## Token Refresh

Claude Code handles token refresh natively inside each container. Each container gets its own copy of the credentials, and Claude Code refreshes when needed. Empirically, copied credentials work across multiple machines/containers without conflict - Anthropic appears to have a grace period or token family approach that prevents rotation from immediately invalidating copies.

## Files to Create/Modify

| File | Action | Description |
|------|--------|-------------|
| `api/pkg/types/claude_subscription.go` | Create | Type definitions |
| `api/pkg/store/store_claude_subscription.go` | Create | GORM CRUD |
| `api/pkg/store/store.go` | Modify | Add interface methods |
| `api/pkg/store/postgres.go` | Modify | AutoMigrate |
| `api/pkg/server/claude_subscription_handlers.go` | Create | API handlers |
| `api/pkg/server/server.go` | Modify | Route registration |
| `api/cmd/settings-sync-daemon/main.go` | Modify | claude_code runtime + credential sync |
| `api/pkg/types/types.go` | Modify | Add ClaudeSubscriptionAvailable to ZedConfigResponse |
| `api/pkg/server/zed_config_handlers.go` | Modify | claude_code in buildCodeAgentConfig |
| `Dockerfile.ubuntu-helix` | Modify | Install Claude Code CLI |
| `frontend/src/components/dashboard/ClaudeSubscriptionSection.tsx` | Create | UI component |
| `frontend/src/components/dashboard/ConnectClaudeDialog.tsx` | Create | Paste dialog |

## Implementation Progress

### Completed

- [x] **Phase 1: Data Model** - `claude_subscription.go`, store CRUD, AutoMigrate, mock generation
- [x] **Phase 2: API Endpoints** - All handlers, route registration, swagger annotations
- [x] **Phase 3: Settings-Sync-Daemon** - `claude_code` runtime config, credential sync
- [x] **Phase 4: Zed Config** - `buildCodeAgentConfig` handles `claude_code`, `ClaudeSubscriptionAvailable` flag
- [x] **Phase 5: Desktop Image** - Claude Code CLI installed, privacy config, exec allowlist updated
- [x] **Phase 6: Frontend UI** - Account settings section with paste + browser login
- [x] **Phase 7: Interactive Login** - Desktop session with `claude auth login`, polling, auto-capture
- [x] **Onboarding** - Claude subscription card in provider step, `claude_code` in runtime dropdown
- [x] **Providers page** - Claude subscription section with connect/connected state
- [x] **AppSettings** - `claude_code` runtime option, model picker with dual-mode note
- [x] **Error messaging** - Clear error when only Claude subscription present and user tries regular chat
- [x] **Zed: Re-enable Claude Code** - Moved Claude Code menu item outside cfg gate in agent_panel.rs
- [x] **Dual-mode Claude Code** - API key mode (through Helix proxy) and subscription mode (direct to Anthropic)

### Claude Code Dual-Mode Architecture

Claude Code ACP in Zed supports **two credential modes**:

1. **Subscription mode** (no provider/model set):
   - Claude Code talks directly to Anthropic using OAuth credentials from `~/.claude/.credentials.json`
   - Credentials synced from user's Claude subscription by settings-sync-daemon
   - Claude Code manages its own model selection internally
   - `baseURL=""`, `apiType=""` in CodeAgentConfig
   - Daemon sets no `ANTHROPIC_*` env vars

2. **API key mode** (Anthropic provider selected):
   - Claude Code routes through Helix API proxy (same as Zed Agent with Anthropic)
   - `baseURL=helixURL+"/v1"`, `apiType="anthropic"` in CodeAgentConfig
   - Daemon sets `ANTHROPIC_BASE_URL` and `ANTHROPIC_API_KEY` env vars on the claude process
   - User selects model in app settings (e.g., `claude-sonnet-4-5-latest`)

The frontend shows the model picker for all runtimes (including `claude_code`) with a note:
"Leave empty to use your Claude subscription, or select an Anthropic model to use via API key."

### Important: Claude Subscription is NOT an Inference Provider

Claude subscriptions only work with Claude Code inside desktop agents. They cannot be used for:
- Regular chat sessions
- Helix Agent (multi-turn)
- Basic Agent (RAG)
- Any non-desktop-agent feature

The frontend shows clear messaging about this distinction in the onboarding flow, providers page, and error messages.

## Verification

1. Build: `cd api && go build ./... && cd ..`
2. Unit tests: `cd api && go test ./pkg/store/... ./pkg/server/... && cd ..`
3. Frontend: `cd frontend && yarn test && yarn build && cd ..`
4. Desktop image: `./stack build-ubuntu`
5. Zed: `cd ../zed && ./stack build-zed` (Claude Code re-enabled)
6. Manual test:
   - Connect a Claude subscription via browser login or paste credentials
   - Verify it shows as "Connected" in Providers page, Onboarding, and Account settings
   - Start a spec task with `claude_code` runtime
   - Verify Claude Code appears in Zed's agent panel dropdown
   - Verify Claude Code works inside Zed
   - Check settings-sync-daemon logs for credential sync
   - Verify error message when trying regular chat with only Claude subscription
