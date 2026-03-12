# Requirements: Add `--organization` flag to `helix test`

## Background

The `helix test` command deploys a temporary app, runs tests against it, then deletes it. Since organizations are now mandated, `helix test` fails because `deployApp()` creates apps without an `OrganizationID`. The `helix apply` command already has `--organization`/`-o` — `helix test` needs the same.

Additionally, to simplify demos and the common case, both `helix test` and `helix apply` should default to the user's first organization when `--organization` is not explicitly provided.

## User Stories

1. As a developer, I want to run `helix test -f helix.yaml -o my-org` so that the test app is created within my organization and tests pass.
2. As a developer, I want `helix test -f helix.yaml` (without `--organization`) to automatically use my first organization, so I don't have to specify it every time.
3. As a developer, I want `helix apply -f helix.yaml` (without `--organization`) to also default to my first organization, for consistency.

## Acceptance Criteria

- [ ] `helix test` accepts `--organization` / `-o` flag (string: org ID or name)
- [ ] The flag is resolved to an org ID via `cli.LookupOrganization()` (same as `helix apply`)
- [ ] `deployApp()` sets `app.OrganizationID` on the created app when flag is provided
- [ ] `deleteApp()` scopes `ListApps` to the organization so cleanup finds the right app
- [ ] When `--organization` is not provided, both `helix test` and `helix apply` default to the user's first organization (via `ListOrganizations()`, taking the first result)
- [ ] If the user has no organizations and `--organization` is not provided, the command proceeds without an org (backward-compatible for deployments that don't mandate orgs)
- [ ] `helix test --help` and `helix apply --help` document the new/updated flag behavior