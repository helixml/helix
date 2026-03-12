# Design: Add `--organization` flag to `helix test` + default-to-first-org for both commands

## Overview

Mirror the `--organization`/`-o` flag from `helix apply` into `helix test`. Additionally, introduce a shared helper that defaults to the user's first organization when the flag is omitted, applied to both `helix test` and `helix apply`. This simplifies demos and the common case where orgs are mandated.

## Key Code Locations

| File | Role |
|------|------|
| `api/cmd/helix/test.go` — `NewTestCmd()` (L680-705) | Flag registration for `helix test` |
| `api/cmd/helix/test.go` — `runTest()` (L707-930) | Orchestrator that calls `deployApp` / `deleteApp` |
| `api/cmd/helix/test.go` — `deployApp()` (L1443-1474) | Creates the temporary test app (missing org) |
| `api/cmd/helix/test.go` — `deleteApp()` (L1476-1508) | Deletes the test app by name lookup (unscoped) |
| `api/pkg/cli/app/apply.go` — `createApp()` (L198-225) | Reference implementation — resolves org and sets `OrganizationID` |
| `api/pkg/cli/organization.go` — `LookupOrganization()` | Shared helper: resolves org name or ID to `*types.Organization` |
| `api/pkg/client/organizations.go` — `ListOrganizations()` | Client method: returns all orgs the user is a member of |

## Design

### 1. New shared helper: `ResolveOrganization()`

Add to `api/pkg/cli/organization.go`:

```go
func ResolveOrganization(ctx context.Context, apiClient client.Client, orgFlag string) (string, error) {
    // If explicitly provided, look it up
    if orgFlag != "" {
        org, err := LookupOrganization(ctx, apiClient, orgFlag)
        if err != nil {
            return "", err
        }
        return org.ID, nil
    }

    // Default to first org
    orgs, err := apiClient.ListOrganizations(ctx)
    if err != nil {
        return "", nil // Swallow error, proceed without org
    }
    if len(orgs) > 0 {
        return orgs[0].ID, nil
    }

    return "", nil // No orgs, proceed without
}
```

This centralizes the "resolve explicit flag or default to first org" logic so both commands (and future ones) use the same behavior.

### 2. Add `--organization`/`-o` flag to `NewTestCmd()`

Add a `string` var and register it:

```go
cmd.Flags().StringVarP(&organization, "organization", "o", "", "Organization ID or name (defaults to first org)")
```

This matches `helix apply` exactly.

### 3. Thread through `runTest()`

Pass the `organization` string into `runTest()`, which calls `ResolveOrganization()` once early, then passes the resolved org ID to `deployApp()` and `deleteApp()`.

### 4. Update `deployApp()` to accept org ID

Before `apiClient.CreateApp()`, set the org:

```go
if orgID != "" {
    app.OrganizationID = orgID
}
```

No need to call `LookupOrganization` again here — `runTest()` already resolved it.

### 5. Scope `deleteApp()` to the organization

Add `organizationID` parameter; pass it into the `AppFilter`:

```go
filter := &client.AppFilter{}
if organizationID != "" {
    filter.OrganizationID = organizationID
}
existingApps, err := apiClient.ListApps(ctx, filter)
```

Without this, the name-based lookup may fail because `ListApps` without org scope may not return org-scoped apps.

### 6. Update `helix apply` to use `ResolveOrganization()` too

In `api/pkg/cli/app/apply.go`, replace the existing org resolution in `createApp()`:

```go
// Before (only resolves explicit flag):
if orgID != "" {
    org, err := cli.LookupOrganization(ctx, apiClient, orgID)
    ...
}

// After (resolves explicit flag or defaults to first org):
resolvedOrgID, err := cli.ResolveOrganization(ctx, apiClient, orgID)
if err != nil {
    return "", err
}
if resolvedOrgID != "" {
    app.OrganizationID = resolvedOrgID
}
```

### 7. What NOT to change

- No changes to the API server — `createApp` handler already accepts `OrganizationID` on the request body.
- No changes to the Go client library — `CreateApp` already sends whatever `OrganizationID` is set on the `types.App`.
- `ListOrganizations` already exists and returns orgs the user belongs to.

## Patterns Found in Codebase

- **Org resolution**: Always use `cli.LookupOrganization()` from `api/pkg/cli/organization.go` for explicit lookups. It handles both `org_xxx` IDs and org names. Don't duplicate this logic.
- **Flag naming**: The convention is `--organization` / `-o` (see `helix apply`, `helix agent`, `helix member`).
- **`deployApp` uses direct client calls**, not shelling out to `helix apply`. So the org must be threaded in-process.
- **`deleteApp` does name-based lookup** via `ListApps` — this must be org-scoped when org is provided, otherwise the app won't be found.
- **`ListOrganizations()`** in `api/pkg/client/organizations.go` returns all orgs the user is a member of. Taking `[0]` gives a sensible default.