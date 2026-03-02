# Design: Onboarding API Keys Not Working in Zed Sessions

## Overview

Fix the provider endpoint lookup to find user-level providers when org-level providers are not found, ensuring API keys configured during onboarding work in organization context.

## Architecture

### Current Flow (Broken)

```
Onboarding → AddProviderDialog(orgId="") → ProviderEndpoint(owner="", owner_type="user")
                                                    ↓
Session(OrganizationID="org_xxx") → Proxy → Query(Owner="org_xxx") → NOT FOUND → Env var fallback
```

### Proposed Flow (Fixed)

```
Onboarding → AddProviderDialog(orgId=createdOrg.id) → ProviderEndpoint(owner="org_xxx", owner_type="org")
                                                              ↓
Session(OrganizationID="org_xxx") → Proxy → Query(Owner="org_xxx") → FOUND ✓
```

**Alternative/Additional Fix (Belt and Suspenders):**

```
Query(Owner="org_xxx") → NOT FOUND → Query(Owner=user.ID) → FOUND ✓
```

## Detailed Design

### Option A: Fix Onboarding to Pass Org ID (Recommended)

The onboarding flow already tracks the created organization in `createdOrg.id`. The fix is to pass this to `AddProviderDialog`.

**File:** `frontend/src/pages/Onboarding.tsx`

```typescript
// Current (line ~2024):
<AddProviderDialog
  orgId=""
  provider={selectedOnboardingProvider}
/>

// Fixed:
<AddProviderDialog
  orgId={createdOrg?.id || ''}
  provider={selectedOnboardingProvider}
/>
```

This ensures providers created during onboarding belong to the organization, matching the session context.

### Option B: Fallback to User-Level Providers (Defense in Depth)

Even with Option A, add a fallback in the backend for robustness.

**File:** `api/pkg/server/anthropic_api_proxy_handler.go`

```go
func (s *HelixAPIServer) getProviderEndpoint(ctx context.Context, user *types.User) (*types.ProviderEndpoint, error) {
    provider := "anthropic"
    
    // ... existing project lookup code ...
    
    if user.OrganizationID != "" {
        // Try org-level first
        endpoint, err := s.Store.GetProviderEndpoint(ctx, &store.GetProviderEndpointsQuery{
            Owner: user.OrganizationID,
            Name:  provider,
        })
        if err == nil {
            return endpoint, nil
        }
        
        // NEW: Fall back to user-level provider
        endpoint, err = s.Store.GetProviderEndpoint(ctx, &store.GetProviderEndpointsQuery{
            Owner: user.ID,
            Name:  provider,
        })
        if err == nil {
            return endpoint, nil
        }
    }
    
    // ... existing fallback to env var ...
}
```

## Decision

**Implement Option A (primary fix) + Option B (fallback).**

- Option A fixes the root cause in onboarding
- Option B provides defense in depth for edge cases (e.g., users who added providers before joining an org)

## Security Considerations

- User-level providers should only be accessible by the user who created them
- Org-level providers are accessible by all org members
- The fallback in Option B uses `user.ID` (authenticated user), not arbitrary lookups

## Testing Strategy

1. **Unit test:** Verify `getProviderEndpoint` fallback logic
2. **Integration test:** Complete onboarding flow with API key, verify LLM calls work
3. **Manual test:** Create spec task after onboarding, confirm agent can chat

## Migration

No database migration needed. Existing user-level providers will work via the fallback. New providers created in onboarding will be org-level.