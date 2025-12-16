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

### 3. Runner Token Authentication for External Providers

Currently, runner tokens work for internal Helix operations but may not properly authenticate for external provider calls. Need to verify and fix:

```go
// api/pkg/server/auth_middleware.go

// Ensure runner token authentication creates a user context that works
// with external provider routing
if token == auth.cfg.runnerToken {
    user := &types.User{
        ID:       "runner-system",
        Type:     types.OwnerTypeRunner,
        // Ensure this user can access configured providers
    }
    // ...
}
```

**Investigation needed:** Trace the code path from runner token auth through to external provider calls to identify any gaps.

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
