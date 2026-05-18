# Implementation Tasks: Honour Explicit Unlimited for Max Concurrent Desktops

- [ ] Add `EffectiveMaxConcurrentDesktops(freeTierDefault int) int` helper on `SystemSettings` in `api/pkg/types/system_settings.go` — returns `-1` for negative input (unlimited), `freeTierDefault` for `0` (unset), and the input value otherwise
- [ ] Replace the fallback branch in `api/pkg/server/handlers.go:117-123` with a single call to `systemSettings.EffectiveMaxConcurrentDesktops(apiServer.Cfg.SubscriptionQuotas.Projects.Free.MaxConcurrentDesktops)` and delete the old `if == 0` branch
- [ ] Verify `SystemSettings.ToResponseWithSource` passes the raw stored `MaxConcurrentDesktops` through unchanged (admin UI needs to see the raw `-1` / `0` / `N` to render the three states distinctly)
- [ ] Confirm `SystemSettingsRequest.MaxConcurrentDesktops` (already `*int`) and the store update path correctly accept and persist `-1` — no DB migration; column stays `int NOT NULL DEFAULT 0`
- [ ] Verify `api/pkg/quota/quota.go:268` `limit < 0` guard already handles the new `-1` value (it does); grep for any other desktop-quota enforcement path that reads `ServerConfigForFrontend.MaxConcurrentDesktops` and add a `< 0` short-circuit if missing
- [ ] Verify `helix-org/helix/helixclient/client.go:560` `HasDesktopRoom` treats `Max <= 0` as unlimited; leave as-is if so (it's harmlessly tolerant)
- [ ] Update `frontend/src/components/dashboard/SystemSettingsTable.tsx` `handleSaveMaxDesktops` (line 113-132) so it accepts `-1` (the new unlimited sentinel) — current guard rejects negatives. Preferred UX: replace the free-form number input with a mode picker (Unlimited / Use server default / Custom cap N) that writes `-1` / `0` / `N` respectively
- [ ] Update chip display in `SystemSettingsTable.tsx` (line 738-742) to render three distinct states: `Unlimited` for `< 0`, `Default` for `0`, `Limit: N` for `> 0`
- [ ] Update the help text in `SystemSettingsTable.tsx` (line 733-734) from `0 = unlimited` to reflect the new semantics (or remove if the mode-picker UX makes it redundant)
- [ ] Add Go table-test for `EffectiveMaxConcurrentDesktops` covering: `-1` + default 2 → -1, `-1` + default 10 → -1, `0` + default 2 → 2, `0` + default 10 → 10, `5` + default 2 → 5, `5` + default 10 → 5
- [ ] Add a handler-level test that `GET /api/v1/config` returns the correct `max_concurrent_desktops` for each of: unset (`0` stored), unlimited (`-1` stored), finite cap (`N` stored)
- [ ] End-to-end test in the inner Helix at `http://localhost:8080` per `helix/CLAUDE.md`: log in, save Unlimited → confirm DB stores `-1` and `/api/v1/config` returns `-1`; save `1` → confirm 2nd desktop session is rejected; save `0` / "Use server default" → confirm cap reverts to free-tier default
- [ ] Run `go build ./pkg/server/ ./pkg/store/ ./pkg/types/` and `cd frontend && yarn build`; push and check CI green via `gh pr checks` / Drone MCP tools
- [ ] Open PR with full URL format per `helix/CLAUDE.md` (`https://github.com/helixml/helix/pull/<n>`) and reference this spec task
