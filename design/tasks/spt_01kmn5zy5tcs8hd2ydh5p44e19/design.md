# Design: Fix Endpoint Type Switching (user → global)

## Root Cause

In `api/pkg/server/provider_handlers.go`, the `updateProviderEndpoint` handler (around line 489) has a comment: _"Preserve ID and ownership information"_. The handler patches many fields from `updatedEndpoint` onto `existingEndpoint`, but **never applies `EndpointType`, `Owner`, or `OwnerType`**. This means:

1. The stored `endpoint_type` never changes on update.
2. The stored `owner` never changes on update.

When a global endpoint is loaded, the system refresh query filters by `Owner = "system"`. If the owner is still a user ID, the endpoint is invisible to that query.

## Key Files

- `api/pkg/types/provider.go` — `UpdateProviderEndpoint` struct (has `EndpointType` field, but no `Owner`/`OwnerType`)
- `api/pkg/server/provider_handlers.go` — `updateProviderEndpoint` handler

## Fix: Backend Only

The fix is entirely in `updateProviderEndpoint` in `provider_handlers.go`. After the admin check at line ~478, add logic to apply the type change and derive the correct ownership:

```go
// Apply endpoint type change and update ownership accordingly
if updatedEndpoint.EndpointType != "" && updatedEndpoint.EndpointType != existingEndpoint.EndpointType {
    existingEndpoint.EndpointType = updatedEndpoint.EndpointType
    switch updatedEndpoint.EndpointType {
    case types.ProviderEndpointTypeGlobal:
        existingEndpoint.Owner = string(types.OwnerTypeSystem)
        existingEndpoint.OwnerType = types.OwnerTypeSystem
    case types.ProviderEndpointTypeUser:
        existingEndpoint.Owner = user.ID
        existingEndpoint.OwnerType = types.OwnerTypeUser
    }
}
```

The frontend already sends `endpoint_type` in the update payload (`UpdateProviderEndpoint` struct includes it). No frontend changes are needed.

## Why No Frontend Changes

The `UpdateProviderEndpoint` struct already has `EndpointType ProviderEndpointType`. The frontend `AddProviderDialog.tsx` sends the form's selected type. The backend just ignores it today. Fixing the backend is sufficient.

## Edge Cases

- Switching `global → user`: owner set to the requesting admin's user ID.
- `endpoint_type` unchanged in the update request: no ownership change (preserve existing).
- Non-admin attempting to switch to global: already blocked at line ~478 — no change needed.
- Org endpoints: not in scope; org type logic is separate and unchanged.
