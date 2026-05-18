# Implementation Tasks: Honour "0 = Unlimited" for Max Concurrent Desktops

- [ ] Change `SystemSettings.MaxConcurrentDesktops` from `int` to `*int` in `api/pkg/types/system_settings.go`
- [ ] Add `EffectiveMaxConcurrentDesktops(freeTierDefault int) int` helper on `SystemSettings` (return `-1` for unlimited, `freeTierDefault` when pointer is nil, else the explicit value)
- [ ] Update `SystemSettings.ToResponseWithSource` so `SystemSettingsResponse.MaxConcurrentDesktops` reflects the resolved effective value (decide and document: return `-1` for unlimited and adjust the frontend chip, OR return `0` for unlimited so the existing `chip ? 'Limit' : 'Unlimited'` logic still works — pick smaller diff)
- [ ] Update GORM column tag to allow NULL and run AutoMigrate; verify the store read/write paths in `api/pkg/store/store_system_settings.go` correctly distinguish "explicit 0" from "unset"
- [ ] Confirm migration policy with user: should existing rows with `0` be migrated to `NULL` (preserve current capped-at-2 behaviour, recommended) or left as `0` (flip silently to unlimited)? Apply chosen approach in the migration
- [ ] Replace the fallback branch in `api/pkg/server/handlers.go:117-123` with a single call to `systemSettings.EffectiveMaxConcurrentDesktops(apiServer.Cfg.SubscriptionQuotas.Projects.Free.MaxConcurrentDesktops)`
- [ ] Verify `helix-org/helix/helixclient/client.go:560` `HasDesktopRoom` already treats `Max <= 0` as unlimited; if it doesn't, update it
- [ ] Verify `StartDesktop` / quota enforcement path treats `-1` as unlimited end-to-end (cross-check against `api/pkg/quota/quota.go:268`)
- [ ] Add table-test for `EffectiveMaxConcurrentDesktops` covering: nil + default 2, nil + default 10, ptr(0) → -1, ptr(5) → 5, ptr(5) with default 10 → 5
- [ ] Add a handler-level test that `GET /api/v1/config` returns the correct `max_concurrent_desktops` for: never-set, set-to-0, set-to-finite
- [ ] Update frontend `SystemSettingsTable.tsx` chip / display logic if the response semantics changed (only if the resolver returns `-1` for unlimited)
- [ ] End-to-end test in the inner Helix at `http://localhost:8080` per `helix/CLAUDE.md`: register/log in, save Max Concurrent Desktops = 0 in the admin UI, hit `curl /api/v1/config`, then actually open more than 2 desktop sessions and confirm none are rejected for quota reasons
- [ ] Run `go build ./pkg/server/ ./pkg/store/ ./pkg/types/` and `cd frontend && yarn build`; push and check CI green via `gh pr checks` / Drone MCP tools
- [ ] Open the PR using full URL format per `helix/CLAUDE.md` (`https://github.com/helixml/helix/pull/<n>`) and reference this spec task
