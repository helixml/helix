# Implementation Tasks: Fix Org Agent App Listing After Bot Table Rename

- [ ] In `api/pkg/server/app_handlers.go`, change `Table("org_worker_runtime_state")` to `Table("org_bot_runtime_state")` in `markHelixOrgAgents`.
- [ ] Update the accompanying comment to say "org_bot_runtime_state" / "(org, bot, backend, key)" so it matches the new schema.
- [ ] Verify no other non-migration Go code references `org_worker_runtime_state` (`grep -rn org_worker_runtime_state api/ --include=*.go`).
- [ ] Build the API: `cd api && CGO_ENABLED=0 go build ./...` (or at least `go build ./pkg/server/`).
- [ ] QA in the local Helix instance: list org apps with helix-org enabled and confirm a 200 response with no `42P01` error in the API logs.
