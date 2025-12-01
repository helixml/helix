# Microsoft Teams Integration Plan

## Overview

Add Microsoft Teams integration for Helix apps, allowing apps to:
1. Respond to user messages in Teams channels/chats
2. Post messages back to Teams conversations
3. Maintain conversation threading/context

This follows the same pattern as the existing Slack integration.

## Architecture Summary

### Existing Slack Pattern (to replicate)

```
types.go
├── SlackTrigger struct (tokens, channels)
├── Trigger struct (contains all trigger types)
└── SlackThread struct (conversation tracking)

trigger/
├── trigger.go (Manager that starts all triggers)
├── slack/
│   ├── slack.go (reconciliation loop)
│   └── slack_bot.go (bot implementation)
└── shared/trigger_session.go (session creation)

store/
└── slack_threads.go (database operations)

config/config.go (server configuration)

frontend/src/components/app/
├── Triggers.tsx (main triggers UI)
├── TriggerSlack.tsx (Slack config UI)
└── TriggerSlackSetup.tsx (setup instructions)
```

## Implementation Plan

### Phase 1: Backend Types & Database

#### 1.1 Add TeamsTrigger type (`api/pkg/types/types.go`)

```go
type TeamsTrigger struct {
    Enabled     bool   `json:"enabled,omitempty"`
    AppID       string `json:"app_id" yaml:"app_id"`         // Microsoft App ID
    AppPassword string `json:"app_password" yaml:"app_password"` // Microsoft App Password
    TenantID    string `json:"tenant_id,omitempty" yaml:"tenant_id,omitempty"` // Optional: restrict to specific tenant
}
```

#### 1.2 Add Teams to Trigger struct (`api/pkg/types/types.go`)

```go
type Trigger struct {
    Discord        *DiscordTrigger        `json:"discord,omitempty" yaml:"discord,omitempty"`
    Slack          *SlackTrigger          `json:"slack,omitempty" yaml:"slack,omitempty"`
    Teams          *TeamsTrigger          `json:"teams,omitempty" yaml:"teams,omitempty"`  // NEW
    Cron           *CronTrigger           `json:"cron,omitempty" yaml:"cron,omitempty"`
    Crisp          *CrispTrigger          `json:"crisp,omitempty" yaml:"crisp,omitempty"`
    AzureDevOps    *AzureDevOpsTrigger    `json:"azure_devops,omitempty" yaml:"azure_devops,omitempty"`
    AgentWorkQueue *AgentWorkQueueTrigger `json:"agent_work_queue,omitempty" yaml:"agent_work_queue,omitempty"`
}
```

#### 1.3 Add TriggerTypeTeams constant (`api/pkg/types/types.go`)

```go
const (
    TriggerTypeSlack       TriggerType = "slack"
    TriggerTypeTeams       TriggerType = "teams"  // NEW
    TriggerTypeCrisp       TriggerType = "crisp"
    TriggerTypeAzureDevOps TriggerType = "azure_devops"
    TriggerTypeCron        TriggerType = "cron"
)
```

#### 1.4 Add TeamsThread struct (`api/pkg/types/types.go`)

```go
// TeamsThread used to track the state of Teams conversations where Helix agent is invoked
type TeamsThread struct {
    ConversationID string    `json:"conversation_id" gorm:"primaryKey"`
    AppID          string    `json:"app_id" gorm:"primaryKey"`
    ChannelID      string    `json:"channel_id" gorm:"index"`
    TeamID         string    `json:"team_id" gorm:"index"`
    Created        time.Time `json:"created"`
    Updated        time.Time `json:"updated"`
    SessionID      string    `json:"session_id"`
}
```

#### 1.5 Add database migration

GORM AutoMigrate will handle schema changes for `TeamsThread`.

### Phase 2: Store Layer

#### 2.1 Add store interface methods (`api/pkg/store/store.go`)

```go
CreateTeamsThread(ctx context.Context, thread *types.TeamsThread) (*types.TeamsThread, error)
GetTeamsThread(ctx context.Context, appID, conversationID string) (*types.TeamsThread, error)
DeleteTeamsThread(ctx context.Context, olderThan time.Time) error
```

#### 2.2 Create store implementation (`api/pkg/store/teams_threads.go`)

Similar to `slack_threads.go`, implement:
- `CreateTeamsThread` - Create new conversation thread
- `GetTeamsThread` - Get existing thread by app ID and conversation ID
- `DeleteTeamsThread` - Cleanup old threads

### Phase 3: Teams Bot Implementation

#### 3.1 Create teams package (`api/pkg/trigger/teams/`)

**teams.go** - Reconciliation loop (similar to `slack/slack.go`):
- `New()` - Create new Teams trigger manager
- `Start()` - Start reconciliation loop
- `reconcile()` - Check for apps with Teams triggers, start/stop bots
- `Stop()` - Stop all running Teams bots

**teams_bot.go** - Bot implementation (similar to `slack/slack_bot.go`):
- `NewTeamsBot()` - Create new bot instance
- `RunBot()` - Start the bot HTTP server/webhook handler
- `handleMessage()` - Process incoming messages via controller.RunBlockingSession
- `setStatus()` - Update trigger status for frontend

#### 3.2 Teams Bot SDK

Use `github.com/infracloudio/msbotbuilder-go` library:
- Handles Bot Framework authentication
- Provides activity handlers for messages
- Supports Teams-specific features (mentions, channels)

Key differences from Slack:
- **Authentication**: Uses Microsoft App ID + Password (OAuth2 with Azure AD)
- **Webhook vs Socket**: Teams uses webhooks (HTTP POST) instead of WebSocket
- **Endpoint**: Requires a public HTTPS endpoint (e.g., `/api/v1/triggers/teams/webhook`)

### Phase 4: Webhook Handler

#### 4.1 Add Teams webhook endpoint (`api/pkg/server/`)

Create `teams_webhook_handlers.go`:
```go
// POST /api/v1/triggers/teams/webhook/{appID}
func (s *HelixAPIServer) handleTeamsWebhook(w http.ResponseWriter, r *http.Request)
```

This endpoint:
1. Receives activities from Teams Bot Framework
2. Validates the JWT token from Azure Bot Service
3. Routes to appropriate bot based on app ID
4. Returns response to Bot Framework

#### 4.2 Register webhook route

Add to `server.go` router configuration.

### Phase 5: Configuration

#### 5.1 Add Teams config (`api/pkg/config/config.go`)

```go
type Teams struct {
    Enabled bool `envconfig:"TEAMS_ENABLED" default:"true"`
}
```

Add to `Triggers` struct:
```go
type Triggers struct {
    Discord Discord
    Slack   Slack
    Teams   Teams  // NEW
    Crisp   Crisp
}
```

### Phase 6: Trigger Manager Integration

#### 6.1 Update trigger manager (`api/pkg/trigger/trigger.go`)

Add Teams to the Manager:
```go
func (t *Manager) Start(ctx context.Context) {
    // ... existing triggers ...

    if t.cfg.Triggers.Teams.Enabled {
        t.wg.Add(1)
        go func() {
            defer t.wg.Done()
            t.runTeams(ctx)
        }()
    }
}

func (t *Manager) runTeams(ctx context.Context) {
    teamsTrigger := teams.New(t.cfg, t.store, t.controller)
    // ... similar to runSlack ...
}
```

### Phase 7: Frontend Components

#### 7.1 Create TriggerTeams component (`frontend/src/components/app/TriggerTeams.tsx`)

Similar to `TriggerSlack.tsx`:
- Toggle to enable/disable
- Input fields for App ID and App Password (masked)
- Status indicator (connecting/connected/error)
- Link to setup instructions

#### 7.2 Create TriggerTeamsSetup component (`frontend/src/components/app/TriggerTeamsSetup.tsx`)

Setup instructions dialog explaining:
1. Register a bot in Azure Portal
2. Create a Teams App in Teams Developer Portal
3. Configure the webhook URL
4. Add permissions and install to Teams

#### 7.3 Update Triggers component (`frontend/src/components/app/Triggers.tsx`)

Add `<TriggerTeams />` component to the list.

#### 7.4 Add Teams icon (`frontend/src/components/icons/ProviderIcons.tsx`)

Add Microsoft Teams logo/icon.

### Phase 8: OpenAPI & Generated Client

#### 8.1 Update Swagger annotations

Ensure new endpoints have proper Swagger docs.

#### 8.2 Regenerate TypeScript client

Run `./stack update_openapi` to regenerate frontend API client.

## File Changes Summary

### New Files
- `api/pkg/trigger/teams/teams.go`
- `api/pkg/trigger/teams/teams_bot.go`
- `api/pkg/trigger/teams/teams_bot_test.go`
- `api/pkg/store/teams_threads.go`
- `api/pkg/store/teams_threads_test.go`
- `api/pkg/server/teams_webhook_handlers.go`
- `frontend/src/components/app/TriggerTeams.tsx`
- `frontend/src/components/app/TriggerTeamsSetup.tsx`

### Modified Files
- `api/pkg/types/types.go` - Add TeamsTrigger, TeamsThread, TriggerTypeTeams
- `api/pkg/store/store.go` - Add TeamsThread interface methods
- `api/pkg/store/store_mocks.go` - Add mock implementations
- `api/pkg/config/config.go` - Add Teams config
- `api/pkg/trigger/trigger.go` - Add runTeams()
- `api/pkg/server/server.go` - Register webhook route
- `frontend/src/components/app/Triggers.tsx` - Add TriggerTeams
- `frontend/src/components/icons/ProviderIcons.tsx` - Add Teams icon

## Dependencies

Add to `go.mod`:
```
github.com/infracloudio/msbotbuilder-go v0.2.6
```

## Testing Plan

### Unit Tests
1. `teams_bot_test.go` - Test message handling, thread creation
2. `teams_threads_test.go` - Test store operations

### Integration Testing

#### Prerequisites
1. Azure account with Bot Framework registration
2. Microsoft 365 tenant with Teams
3. ngrok or similar for local webhook testing

#### Test Steps

1. **Register Azure Bot**:
   - Go to Azure Portal > Bot Services > Create
   - Note the App ID and create/copy the App Password
   - Set messaging endpoint to `https://<your-helix-url>/api/v1/triggers/teams/webhook/{appID}`

2. **Create Teams App**:
   - Go to Teams Developer Portal
   - Create new app, add Bot capability
   - Use the Azure Bot's App ID
   - Install app to a team/chat

3. **Configure Helix App**:
   - Enable Teams trigger on a Helix app
   - Enter App ID and App Password
   - Save configuration

4. **Test Message Flow**:
   - @mention the bot in Teams
   - Verify Helix receives the message (check API logs)
   - Verify bot responds in the thread
   - Send follow-up message in thread
   - Verify conversation context is maintained

5. **Test Error Scenarios**:
   - Invalid credentials → verify error status shown in UI
   - Bot disconnection → verify reconnection
   - Rate limiting → verify graceful handling

### Local Development Testing with ngrok

```bash
# Start ngrok tunnel
ngrok http 8080

# Update Azure Bot messaging endpoint to ngrok URL
# e.g., https://abc123.ngrok.io/api/v1/triggers/teams/webhook/{appID}

# Start Helix
./stack start

# Test in Teams
```

## Security Considerations

1. **Token Validation**: Validate incoming webhook requests using Microsoft Bot Framework JWT validation
2. **Credential Storage**: App Password stored encrypted in database (same as Slack tokens)
3. **Tenant Restriction**: Optional tenant ID to restrict bot to specific organizations
4. **Rate Limiting**: Implement rate limiting on webhook endpoint

## Notes

- Teams uses webhook-based communication (unlike Slack's WebSocket), so we need a publicly accessible HTTPS endpoint
- For local development, use ngrok or similar tunnel service
- Bot Framework SDK handles OAuth2 token refresh automatically
- Consider implementing proactive messaging in future (bot initiates conversation)

## References

- [Microsoft Bot Framework SDK for Go](https://github.com/infracloudio/msbotbuilder-go)
- [Teams Bot Framework Overview](https://learn.microsoft.com/en-us/microsoftteams/platform/bots/bot-features)
- [Azure Bot Service Documentation](https://learn.microsoft.com/en-us/azure/bot-service/)
- Existing Slack implementation: `api/pkg/trigger/slack/`
