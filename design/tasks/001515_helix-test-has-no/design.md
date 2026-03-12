# Design: Add `--organization` flag to `helix test`

## Overview

Mirror the `--organization`/`-o` flag from `helix apply` into `helix test`. This is a small, mechanical change — no new APIs, no new packages, no architectural decisions.

## Key Code Locations

| File | Role |
|------|------|
| `api/cmd/helix/test.go` — `NewTestCmd()` (L680-705) | Flag registration |
| `api/cmd/helix/test.go` — `runTest()` (L707-930) | Orchestrator that calls `deployApp` / `deleteApp` |
| `api/cmd/helix/test.go` — `deployApp()` (L1443-1474) | Creates the temporary test app (missing org) |
| `api/cmd/helix/test.go` — `deleteApp()` (L1476-1508) | Deletes the test app by name lookup (unscoped) |
| `api/pkg/cli/app/apply.go` — `createApp()` (L198-225) | Reference implementation — resolves org and sets `OrganizationID` |
| `api/pkg/cli/organization.go` — `LookupOrganization()` | Shared helper: resolves org name or ID to `*types.Organization` |

## Design

### 1. Add the flag to `NewTestCmd()`

Add a `string` var and register it:

```
cmd.Flags().StringVarP(&organization, "organization", "o", "", "Organization ID or name")
```

This matches `helix apply` exactly.

### 2. Thread through `runTest()`

Pass the `organization` string into `runTest()`, which passes it to `deployApp()` and `deleteApp()`.

### 3. Resolve org in `deployApp()`

Before `apiClient.CreateApp()`, if `organization != ""`:

```go
org, err := cli.LookupOrganization(ctx, apiClient, organization)
if err != nil {
    return "", fmt.Errorf("failed to lookup organization: %w", err)
}
app.OrganizationID = org.ID
```

This reuses the existing shared `cli.LookupOrganization` helper (accepts both org name and `org_xxx` ID).

### 4. Scope `deleteApp()` to the organization

Currently `deleteApp` calls `ListApps` with an empty filter. When org is provided, pass the resolved org ID into the filter:

```go
filter := &client.AppFilter{}
if orgID != "" {
    filter.OrganizationID = orgID
}
existingApps, err := apiClient.ListApps(ctx, filter)
```

Without this, the name-based lookup may fail because `ListApps` without org scope may not return org-scoped apps.

### 5. What NOT to change

- No changes to the API server — `createApp` handler already accepts `OrganizationID` on the request body.
- No changes to the Go client library — `CreateApp` already sends whatever `OrganizationID` is set on the `types.App`.
- No new packages or helpers needed — `cli.LookupOrganization` already exists.
- Backward compatible — the flag defaults to `""`, preserving existing behavior for deployments that don't mandate orgs.

## Patterns Found in Codebase

- **Org resolution**: Always use `cli.LookupOrganization()` from `api/pkg/cli/organization.go`. It handles both `org_xxx` IDs and org names. Don't duplicate this logic.
- **Flag naming**: The convention is `--organization` / `-o` (see `helix apply`, `helix agent`, `helix member`).
- **`deployApp` uses direct client calls**, not shelling out to `helix apply`. So the org must be threaded in-process.
- **`deleteApp` does name-based lookup** via `ListApps` — this must be org-scoped when org is provided, otherwise the app won't be found.