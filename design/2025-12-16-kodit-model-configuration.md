# Kodit Model Configuration via Helix Proxy

**Date:** 2025-12-16
**Status:** Draft
**Author:** Luke / Claude

## Problem Statement

Kodit requires LLM configuration for two key capabilities:
1. **Enrichments** - Generating code documentation, examples, architecture docs (requires an LLM like `qwen3:8b`)
2. **Embeddings** - Semantic code search (requires an embedding model like `MrLight/dse-qwen2-2b-mrl-v1`)

Currently, neither Docker Compose nor Helm chart deployments have proper model configuration for Kodit. The settings exist but are commented out or missing, leaving Kodit unable to generate enrichments.

## Proposed Solution

Introduce a **proxy model pattern** where Kodit uses a special model name `kodit-model` that Helix dynamically substitutes with the actual configured provider/model at runtime.

### Architecture Overview

```
┌──────────────────────────────────────────────────────────────────────────────┐
│                           KODIT MODEL PROXY FLOW                             │
├──────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│   ┌─────────┐    ENRICHMENT_ENDPOINT_MODEL     ┌──────────────────────────┐  │
│   │  Kodit  │ ─────────────────────────────────▶  Helix OpenAI API       │  │
│   │         │    = openai/kodit-model           │  /v1/chat/completions   │  │
│   └─────────┘                                   └───────────┬──────────────┘  │
│                                                             │                │
│                                                             ▼                │
│                                              ┌──────────────────────────────┐│
│                                              │   Model Substitution Logic   ││
│                                              │                              ││
│                                              │   if model == "kodit-model": ││
│                                              │     lookup SystemSettings    ││
│                                              │     replace with real model  ││
│                                              └───────────┬──────────────────┘│
│                                                          │                   │
│                                                          ▼                   │
│   ┌──────────────────────────────────────────────────────────────────────┐  │
│   │                      Actual LLM Provider                              │  │
│   │  e.g., together_ai/Qwen/Qwen3-8B, openai/gpt-4o, anthropic/claude    │  │
│   └──────────────────────────────────────────────────────────────────────┘  │
│                                                                              │
└──────────────────────────────────────────────────────────────────────────────┘
```

## Implementation Components

### 1. Extend SystemSettings Table

Add Kodit model configuration to the existing `SystemSettings` table with separate provider and model fields:

```go
// api/pkg/types/system_settings.go

type SystemSettings struct {
    ID      string    `json:"id" gorm:"primaryKey"`
    Created time.Time `json:"created"`
    Updated time.Time `json:"updated"`

    // Existing fields
    HuggingFaceToken string `json:"huggingface_token,omitempty"`

    // NEW: Kodit enrichment model configuration (separate provider and model)
    KoditEnrichmentProvider string `json:"kodit_enrichment_provider,omitempty"` // e.g., "together_ai", "openai", "anthropic"
    KoditEnrichmentModel    string `json:"kodit_enrichment_model,omitempty"`    // e.g., "Qwen/Qwen3-8B", "gpt-4o"

    // Future: Kodit embedding model (when we support proxying embeddings)
    // KoditEmbeddingProvider string `json:"kodit_embedding_provider,omitempty"`
    // KoditEmbeddingModel    string `json:"kodit_embedding_model,omitempty"`
}

type SystemSettingsRequest struct {
    HuggingFaceToken        *string `json:"huggingface_token,omitempty"`
    KoditEnrichmentProvider *string `json:"kodit_enrichment_provider,omitempty"`
    KoditEnrichmentModel    *string `json:"kodit_enrichment_model,omitempty"`
}

type SystemSettingsResponse struct {
    // ... existing fields ...

    KoditEnrichmentProvider string `json:"kodit_enrichment_provider"`
    KoditEnrichmentModel    string `json:"kodit_enrichment_model"`
    KoditEnrichmentModelSet bool   `json:"kodit_enrichment_model_set"` // true if both provider and model are set
}
```

### 2. OpenAI API Model Substitution

Add special handling in the OpenAI chat completions handler:

```go
// api/pkg/server/openai_chat_handlers.go

func (s *HelixAPIServer) createChatCompletion(w http.ResponseWriter, r *http.Request) {
    // ... existing code ...

    // Special handling for kodit-model proxy
    if req.Model == "kodit-model" {
        settings, err := s.Store.GetEffectiveSystemSettings(r.Context())
        if err != nil {
            http.Error(w, "Failed to get system settings", http.StatusInternalServerError)
            return
        }
        if settings.KoditEnrichmentProvider == "" || settings.KoditEnrichmentModel == "" {
            http.Error(w, "Kodit enrichment model not configured in system settings",
                http.StatusBadRequest)
            return
        }

        // Combine provider and model into the format expected by Helix routing
        // e.g., "together_ai" + "Qwen/Qwen3-8B" -> "together_ai/Qwen/Qwen3-8B"
        resolvedModel := settings.KoditEnrichmentProvider + "/" + settings.KoditEnrichmentModel
        log.Debug().
            Str("original_model", "kodit-model").
            Str("provider", settings.KoditEnrichmentProvider).
            Str("model", settings.KoditEnrichmentModel).
            Str("resolved_model", resolvedModel).
            Msg("Substituted kodit-model with configured enrichment model")
        req.Model = resolvedModel
    }

    // ... continue with normal processing ...
}
```

### 3. Runner Token Authentication for External Providers - BUGS FOUND

**Investigation completed.** Two bugs were found that would cause runner token requests to fail in hosted environments.

#### Bug 1: Token Quota Check Fails for Runner Tokens

**Location:** `api/pkg/controller/sessions.go:202-232`

When quota checking is enabled (`SubscriptionQuotas.Enabled = true`), the following code fails:

```go
func (c *Controller) checkInferenceTokenQuota(ctx context.Context, userID string, provider string) error {
    // ...

    // Get user's current monthly usage
    monthlyTokens, err := c.Options.Store.GetUserMonthlyTokenUsage(ctx, userID, types.GlobalProviders)
    if err != nil {
        return fmt.Errorf("failed to get user token usage: %w", err)  // FAILS for "runner-system"
    }

    // Check if user is pro tier
    pro, err := c.isUserProTier(ctx, userID)  // FAILS for "runner-system"
    if err != nil {
        return fmt.Errorf("failed to check user tier: %w", err)
    }
    // ...
}
```

When `userID = "runner-system"` (from runner token auth), `GetUserMonthlyTokenUsage` and `GetUserMeta` will fail because there's no user with that ID in the database.

**Fix:** Skip quota check for runner tokens:
```go
func (c *Controller) checkInferenceTokenQuota(ctx context.Context, userID string, provider string) error {
    // Skip quota check for runner tokens (system-level access)
    if userID == "runner-system" {
        return nil
    }
    // ... rest of function
}
```

#### Bug 2: Balance Check Fails for Runner Tokens

**Location:** `api/pkg/controller/balance_check.go:10-35`

When Stripe billing is enabled AND the client has billing enabled:

```go
func (c *Controller) HasEnoughBalance(ctx context.Context, user *types.User, orgID string, clientBillingEnabled bool) (bool, error) {
    if !c.Options.Config.Stripe.BillingEnabled {
        return true, nil  // OK if billing disabled
    }

    if !clientBillingEnabled {
        return true, nil  // OK if client billing disabled
    }

    // ... but if both are enabled:
    wallet, err = c.Options.Store.GetWalletByUser(ctx, user.ID)  // FAILS for "runner-system"
    if err != nil {
        return false, fmt.Errorf("failed to get wallet: %w", err)
    }
}
```

**Fix:** Skip balance check for runner tokens:
```go
func (c *Controller) HasEnoughBalance(ctx context.Context, user *types.User, orgID string, clientBillingEnabled bool) (bool, error) {
    // Skip balance check for runner tokens (system-level access)
    if user.TokenType == types.TokenTypeRunner {
        return true, nil
    }
    // ... rest of function
}
```

#### Impact

- **Self-hosted deployments:** Usually unaffected (quotas and billing typically disabled)
- **Hosted environments (helix.ml):** Runner token requests would fail with quota/billing errors

#### Fixes Applied

Both bugs have been fixed in this implementation:

**Bug 1 fix** (`api/pkg/controller/sessions.go:202-207`):
```go
func (c *Controller) checkInferenceTokenQuota(ctx context.Context, userID string, provider string) error {
    // Skip quota check for runner tokens (system-level access)
    if userID == "runner-system" {
        return nil
    }
    // ... rest of function
}
```

**Bug 2 fix** (`api/pkg/controller/balance_check.go:10-15`):
```go
func (c *Controller) HasEnoughBalance(ctx context.Context, user *types.User, orgID string, clientBillingEnabled bool) (bool, error) {
    // Skip balance check for runner tokens (system-level access)
    if user.TokenType == types.TokenTypeRunner {
        return true, nil
    }
    // ... rest of function
}
```

#### Root Cause

The authentication middleware creates a user with `ID: "runner-system"` for runner tokens, but the downstream code assumes this is a real user ID with associated database records (user_meta, wallet, usage tracking).

#### Bug 3: Provider Validation Passes but Client Lookup Fails

**Location:** `api/pkg/server/openai_chat_handlers.go:279-316` and `api/pkg/openai/manager/provider_manager.go:324-371`

The `isKnownProvider` function returns `true` for hardcoded global providers (openai, togetherai, anthropic, helix, vllm) even if they're not actually configured with API keys:

```go
func (s *HelixAPIServer) isKnownProvider(ctx context.Context, providerName, ownerID string) bool {
    // Check global providers first (fast path)
    if types.IsGlobalProvider(providerName) {
        return true  // Returns true even if provider has no API key configured!
    }
    // ...
}
```

Then when `GetClient` is called, it fails because:
1. `globalClients["openai"]` doesn't exist (OPENAI_API_KEY not set)
2. Database has no provider endpoint named "openai"
3. Error: "no client found for provider: openai"

**Example scenario:**
- Kodit configured to use `openai/kodit-model`
- `OPENAI_API_KEY` is NOT set (intentional - want to use helix runners)
- But actual Kodit model is configured as `helix/llama3` in SystemSettings
- Provider validation passes for "openai" (hardcoded as global)
- But no OpenAI client exists → request fails

**Fix:** Either:
1. Check if the provider is actually configured, not just in the hardcoded list
2. Or skip provider prefix parsing for the special `kodit-model` and handle it specially

```go
// Option 1: Validate that global providers are actually configured
func (s *HelixAPIServer) isKnownProvider(ctx context.Context, providerName, ownerID string) bool {
    if types.IsGlobalProvider(providerName) {
        // Also check if it's actually configured
        if s.providerManager.HasClient(providerName) {
            return true
        }
    }
    // ... rest of function
}
```

#### Bug 4: Inconsistent Owner ID Between Validation and Client Lookup

**Location:** `api/pkg/server/openai_chat_handlers.go:69-72` vs `api/pkg/controller/inference.go:414`

There's an inconsistency in the owner ID used for runner tokens:

**In handler (for validation):**
```go
ownerID := user.ID  // "runner-system"
if user.TokenType == types.TokenTypeRunner {
    ownerID = oai.RunnerID  // Changes to "runner"
}
// Provider validation uses ownerID = "runner"
```

**In controller (for client lookup):**
```go
func (c *Controller) getClient(ctx context.Context, organizationID, userID, provider string) {
    owner := userID  // Still "runner-system" - NOT changed!
    // ...
    c.providerManager.GetClient(ctx, &manager.GetClientRequest{
        Owner: owner,  // Uses "runner-system"
    })
}
```

While both `"runner"` and `"runner-system"` should work for querying global endpoints (due to the `WithGlobal: true` flag), this inconsistency is confusing and could cause subtle bugs if the query logic changes.

**Fix:** Use consistent owner ID handling. Either:
1. Pass `ownerID` through to the controller instead of `user.ID`
2. Or check for `TokenTypeRunner` in the controller and adjust accordingly

### 4. Admin Panel - Code Intelligence Settings Page

Create a new admin page at `/admin/code-intelligence`:

```typescript
// frontend/src/pages/AdminCodeIntelligence.tsx

const AdminCodeIntelligence: FC = () => {
    const { data: settings } = useSystemSettings()
    const updateSettings = useUpdateSystemSettings()

    return (
        <AdminLayout>
            <h1>Code Intelligence Configuration</h1>

            <Section title="Kodit Enrichment Model">
                <p>
                    Select the model used by Kodit to generate code documentation,
                    examples, and architecture documentation.
                </p>

                <AdvancedModelPicker
                    provider={settings?.kodit_enrichment_provider}
                    model={settings?.kodit_enrichment_model}
                    onChange={(provider, model) => updateSettings({
                        kodit_enrichment_provider: provider,
                        kodit_enrichment_model: model
                    })}
                />

                {settings?.kodit_enrichment_model_set && (
                    <StatusIndicator status="configured">
                        Using {settings.kodit_enrichment_provider}/{settings.kodit_enrichment_model}
                    </StatusIndicator>
                )}
            </Section>

            {/* Future: Embedding model configuration */}
        </AdminLayout>
    )
}
```

### 5. Helm Chart Configuration

Update the Helix controlplane Helm chart to pass runner token to Kodit:

```yaml
# charts/helix-controlplane/values.yaml

kodit:
  enabled: true

  controllers:
    kodit:
      containers:
        kodit:
          envFrom:
            - secretRef:
                name: "{{ .Release.Name }}-kodit"
            # NEW: Reference the runner token secret
            - secretRef:
                name: "{{ .Release.Name }}-runner-token"
          env:
            DATA_DIR: /data
            DB_URL: postgresql+asyncpg://...
            DEFAULT_SEARCH_PROVIDER: vectorchord

            # NEW: Enrichment endpoint configuration
            ENRICHMENT_ENDPOINT_TYPE: openai
            ENRICHMENT_ENDPOINT_BASE_URL: "http://{{ .Release.Name }}:80/v1"
            ENRICHMENT_ENDPOINT_MODEL: openai/kodit-model
            ENRICHMENT_ENDPOINT_NUM_PARALLEL_TASKS: "3"
            ENRICHMENT_ENDPOINT_TIMEOUT: "120"
            # API key comes from secretRef above as ENRICHMENT_ENDPOINT_API_KEY
```

The runner token secret structure:
```yaml
# Customer creates this secret with their runner token
apiVersion: v1
kind: Secret
metadata:
  name: helix-runner-token
stringData:
  ENRICHMENT_ENDPOINT_API_KEY: "their-runner-token-here"
```

### 6. Docker Compose Configuration

Update docker-compose.yaml to enable Kodit model configuration:

```yaml
# docker-compose.yaml

kodit:
  profiles: [kodit]
  image: registry.helixml.tech/helix/kodit:latest
  environment:
    - DATA_DIR=/data
    - DB_URL=postgresql+asyncpg://...
    - DEFAULT_SEARCH_PROVIDER=vectorchord

    # Enrichment endpoint - uses Helix as proxy
    - ENRICHMENT_ENDPOINT_TYPE=openai
    - ENRICHMENT_ENDPOINT_BASE_URL=http://api:8080/v1
    - ENRICHMENT_ENDPOINT_MODEL=openai/kodit-model
    - ENRICHMENT_ENDPOINT_API_KEY=${RUNNER_TOKEN}
    - ENRICHMENT_ENDPOINT_NUM_PARALLEL_TASKS=3
    - ENRICHMENT_ENDPOINT_TIMEOUT=120

    # Embeddings - use internal Kodit embeddings for now
    # Future: Add EMBEDDING_ENDPOINT_* for Helix-proxied embeddings
```

## Current Test Configuration

Docker-compose files have been configured for testing runner token authentication:

```yaml
# Both docker-compose.yaml and docker-compose.dev.yaml now have:
kodit:
  environment:
    - ENRICHMENT_ENDPOINT_BASE_URL=http://api:8080/v1
    - ENRICHMENT_ENDPOINT_MODEL=openai/kodit-model
    - ENRICHMENT_ENDPOINT_API_KEY=${RUNNER_TOKEN-oh-hallo-insecure-token}
    - ENRICHMENT_ENDPOINT_NUM_PARALLEL_TASKS=3
    - ENRICHMENT_ENDPOINT_TIMEOUT=120
```

**How it works:**
1. Kodit uses LiteLLM which parses `openai/kodit-model` as provider=openai, model=kodit-model
2. LiteLLM sends just `kodit-model` as the model name to Helix
3. Helix receives the request with runner token authentication
4. Currently this will fail because `kodit-model` isn't a real model - the substitution logic needs to be implemented

**To test runner token routing:**
1. Start services with `docker compose --profile kodit up`
2. Trigger an enrichment in Kodit (e.g., index a repository)
3. Check Helix API logs for the incoming request
4. Observe error: model "kodit-model" not found (expected until substitution is implemented)

## Kodit MCP Proxy

Expose Kodit's MCP server through Helix API, authenticated with user's Helix API key.

### Architecture

```
┌───────────────────────────────────────────────────────────────────────────────┐
│                        KODIT MCP PROXY ARCHITECTURE                           │
├───────────────────────────────────────────────────────────────────────────────┤
│                                                                               │
│   ┌──────────────┐   User API Key   ┌──────────────────────────────────────┐  │
│   │  AI Coding   │ ─────────────────▶  Helix API                           │  │
│   │  Assistant   │                   │  /api/v1/kodit/mcp                  │  │
│   │  (e.g. Zed)  │                   │  (auth + proxy)                     │  │
│   └──────────────┘                   └──────────────┬─────────────────────┘  │
│                                                     │                        │
│                                                     ▼                        │
│                                      ┌──────────────────────────────────────┐│
│                                      │         Kodit MCP Server             ││
│                                      │    http://kodit:8632/mcp             ││
│                                      │                                      ││
│                                      │    - search_symbols                  ││
│                                      │    - search_code                     ││
│                                      │    - get_snippet                     ││
│                                      │    - list_repositories               ││
│                                      └──────────────────────────────────────┘│
│                                                                               │
└───────────────────────────────────────────────────────────────────────────────┘
```

### Implementation

#### 1. MCP Proxy Handler

Create an HTTP handler that proxies MCP requests to Kodit:

```go
// api/pkg/server/kodit_mcp_proxy.go

func (s *HelixAPIServer) koditMCPProxy(w http.ResponseWriter, r *http.Request) {
    user := getRequestUser(r)
    if !hasUser(user) {
        http.Error(w, "unauthorized", http.StatusUnauthorized)
        return
    }

    // Proxy to Kodit's MCP endpoint
    koditURL := s.Cfg.Kodit.URL + "/mcp"
    proxy := httputil.NewSingleHostReverseProxy(koditURL)

    // Forward request with Kodit API key
    r.Header.Set("Authorization", "Bearer " + s.Cfg.Kodit.APIKey)
    proxy.ServeHTTP(w, r)
}
```

#### 2. Route Registration

```go
// api/pkg/server/server.go

// In registerRoutes():
r.HandleFunc("/api/v1/kodit/mcp", s.koditMCPProxy).Methods("GET", "POST", "OPTIONS")
r.HandleFunc("/api/v1/kodit/mcp/{path:.*}", s.koditMCPProxy).Methods("GET", "POST", "OPTIONS")
```

#### 3. Configuration

Add Kodit URL to config:

```go
// api/pkg/config/config.go

type KoditConfig struct {
    URL    string `envconfig:"KODIT_URL" default:"http://kodit:8632"`
    APIKey string `envconfig:"KODIT_API_KEY"`
}
```

#### 4. Settings Sync Daemon Integration

The settings-sync-daemon should automatically configure Kodit as an MCP server for agents:

```go
// api/cmd/settings-sync-daemon/mcp_config.go

func getKoditMCPConfig(helixURL, apiKey string) *MCPServerConfig {
    return &MCPServerConfig{
        Name:        "kodit",
        Description: "Code search and indexing",
        ServerURL:   helixURL + "/api/v1/kodit/mcp",
        Headers: map[string]string{
            "Authorization": "Bearer " + apiKey,
        },
    }
}
```

### Roadmap

1. **Phase 6: MCP Proxy** (Current)
   - [x] Design MCP proxy architecture
   - [ ] Implement HTTP proxy handler
   - [ ] Add route registration with auth middleware
   - [ ] Add Kodit URL configuration

2. **Phase 7: Agent Integration**
   - [ ] Update settings-sync-daemon to expose Kodit MCP to agents
   - [ ] Configure MCP server in Zed's agent_servers settings
   - [ ] Test MCP tools in sandbox environment

3. **Future: Optional Kodit Configuration**
   - [ ] Make Kodit MCP server optional via agent skills configuration
   - [ ] Allow per-user/per-organization Kodit settings
   - [ ] Support multiple Kodit instances for different codebases

## Implementation Order

1. **Phase 1: Backend Foundation**
   - [ ] Extend `SystemSettings` with `KoditEnrichmentModel` field
   - [ ] Add GORM migration for new column
   - [ ] Update `SystemSettingsRequest` and `SystemSettingsResponse`
   - [ ] Update system settings handlers

2. **Phase 2: Model Substitution**
   - [ ] Add `kodit-model` substitution logic in OpenAI chat handler
   - [ ] Verify runner token works with external provider routing
   - [ ] Add logging for model substitution

3. **Phase 3: Admin UI**
   - [ ] Create `/admin/code-intelligence` page
   - [ ] Add `AdvancedModelPicker` for Kodit enrichment model
   - [ ] Add to admin sidebar navigation

4. **Phase 4: Deployment Configuration**
   - [ ] Update `docker-compose.yaml` with Kodit enrichment config
   - [ ] Update `docker-compose.dev.yaml` for development
   - [ ] Update Helm chart `values.yaml` with secret references
   - [ ] Document runner token secret creation for Kubernetes

5. **Phase 5: Testing & Documentation**
   - [ ] Test end-to-end with various providers (OpenAI, Together AI, etc.)
   - [ ] Update Kodit documentation
   - [ ] Update Helix deployment documentation

## Future Enhancements

1. **Embedding Model Proxy**: Add `kodit-embedding-model` for proxying embedding requests through Helix, allowing use of GPU-accelerated embeddings on Helix runners.

2. **Per-Organization Settings**: Allow organizations to configure their own Kodit models, falling back to system defaults.

3. **Model Validation**: Validate that the selected model supports the required capabilities (chat completions for enrichments, embeddings for search).

## Security Considerations

1. **Runner Token Exposure**: The runner token is passed to Kodit as `ENRICHMENT_ENDPOINT_API_KEY`. This is acceptable because:
   - Kodit runs in the same trusted network as Helix
   - Runner tokens already have system-level access
   - The token is stored in Kubernetes secrets (not in values.yaml)

2. **Model Access Control**: The `kodit-model` substitution should respect the same access controls as direct model requests.

## Open Questions

1. Should we support multiple Kodit model configurations (e.g., different models for different enrichment types)?

2. Should embedding model proxying be prioritized, or is internal Kodit embedding sufficient for initial release?

3. What happens if the configured model is unavailable? Should Kodit fall back to a default, or fail explicitly?

## References

- [Kodit Configuration Documentation](https://docs.helixml.tech/kodit/reference/configuration/)
- [Kodit Enrichment Endpoints](https://docs.helixml.tech/kodit/reference/configuration/#enrichment-endpoints)
- Helix SystemSettings: `api/pkg/types/system_settings.go`
- Helix OpenAI Handlers: `api/pkg/server/openai_chat_handlers.go`
