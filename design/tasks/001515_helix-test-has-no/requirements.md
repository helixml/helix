# Requirements: Add `--organization` flag to `helix test`

## Background

The `helix test` command deploys a temporary app, runs tests against it, then deletes it. Since organizations are now mandated, `helix test` fails because `deployApp()` creates apps without an `OrganizationID`. The `helix apply` command already has `--organization`/`-o` — `helix test` needs the same.

## User Stories

1. As a developer, I want to run `helix test -f helix.yaml -o my-org` so that the test app is created within my organization and tests pass.

## Acceptance Criteria

- [ ] `helix test` accepts `--organization` / `-o` flag (string: org ID or name)
- [ ] The flag is resolved to an org ID via `cli.LookupOrganization()` (same as `helix apply`)
- [ ] `deployApp()` sets `app.OrganizationID` on the created app when flag is provided
- [ ] `deleteApp()` scopes `ListApps` to the organization so cleanup finds the right app
- [ ] Without `--organization`, behavior is unchanged (backward-compatible for deployments that don't mandate orgs)
- [ ] `helix test --help` documents the new flag