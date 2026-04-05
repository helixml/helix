# Design

## Approach

Add a `has_providers` boolean to the server config (`/api/v1/config`). The frontend uses this to filter out the "provider" step from the onboarding wizard, exactly like it already filters out the "subscription" step when `billing_enabled` is false.

## Why server config instead of frontend-only?

The current provider list API (`useListProviders`) requires an `orgId` — it only fires after the user creates/selects an org (step 2). The server config loads immediately on app init, so we can hide the step before the user even reaches it. No flash.

## Backend Changes

### `api/pkg/types/types.go` — `ServerConfigForFrontend`

Add field:
```go
HasProviders bool `json:"has_providers"` // Whether any global AI provider with enabled chat models exists
```

### `api/pkg/server/handlers.go` — `getConfig()`

After line ~116 (where `ProvidersManagementEnabled` is set), query global provider endpoints and check if any have enabled chat models:

```go
globalProviders, err := apiServer.Store.ListProviderEndpoints(ctx, store.ListProviderEndpointsQuery{
    EndpointType: types.ProviderEndpointTypeGlobal,
})
if err == nil {
    for _, p := range globalProviders {
        if hasEnabledChatModels(p) {
            config.HasProviders = true
            break
        }
    }
}
```

Note: The exact store query API and model-checking logic need to match existing patterns in the codebase. The provider endpoint listing already exists (used by `listProviderEndpoints` handler). The model check may need to load models for each provider or check a simpler flag — follow existing patterns in `provider_handlers.go`.

**Codebase patterns discovered:**
- `ServerConfigForFrontend` is at `api/pkg/types/types.go:1025`
- Config is built in `api/pkg/server/handlers.go:88`
- Provider endpoints are listed via `s.Store.ListProviderEndpoints()` (see `provider_handlers.go:71`)
- Provider type enum: `types.ProviderEndpointTypeGlobal`

## Frontend Changes

### `frontend/src/pages/Onboarding.tsx`

Extend the `visibleSteps` filter (line 327-332) to also exclude the "provider" step when `serverConfig.has_providers` is true:

```typescript
const visibleSteps = useMemo(() => {
  let steps = ALL_STEPS;
  if (!serverConfig?.billing_enabled) {
    steps = steps.filter((step) => step.type !== "subscription");
  }
  if (serverConfig?.has_providers) {
    steps = steps.filter((step) => step.type !== "provider");
  }
  return steps;
}, [serverConfig?.billing_enabled, serverConfig?.has_providers]);
```

This is the same pattern already used for `billing_enabled` / `subscription`. Step indices auto-adjust because everything uses `getStepIndexByType()`.

### `frontend/src/api/api.ts`

After regenerating the OpenAPI client (`./stack update_openapi`), the `TypesServerConfigForFrontend` interface will include the new `has_providers` field automatically.

## What NOT to change

- The existing auto-complete logic (lines 400-412) stays as-is. It handles org-level provider detection as a separate concern.
- No changes to the provider step UI content itself.
- No changes to provider listing APIs.
