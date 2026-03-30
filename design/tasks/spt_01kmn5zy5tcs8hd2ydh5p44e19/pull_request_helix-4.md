# Fix endpoint type switching: update owner when changing user → global

## Summary

When an admin changed a provider endpoint's type from `user` to `global`, the `owner` field stayed as the user's ID instead of being updated to `"system"`. This caused the system query (which filters by `owner = "system"`) to miss the endpoint, making it invisible to all other users.

## Changes

- In `api/pkg/server/provider_handlers.go`, `updateProviderEndpoint`: apply the new `endpoint_type` from the request and derive correct ownership:
  - `global` → `owner = "system"`, `owner_type = "system"`
  - `user` → `owner = user.ID`, `owner_type = "user"`

No frontend changes needed — `EditProviderEndpointDialog.tsx` already sends the correct `endpoint_type` in the update payload.
