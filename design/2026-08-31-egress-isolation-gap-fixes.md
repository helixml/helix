# Egress isolation — gap fixes (follow-up to PR 3148)

Context: PR 3148 (`fix/hydra-sandbox-egress-isolation`) isolates sandbox egress
via a dedicated `helix-sandboxes` bridge + iptables + a Hydra-owned API proxy at
`192.0.2.1:18080`. A code review + the live validation in helixml/infra#70 found
gaps that undermine the isolation or add avoidable complexity. This doc tracks
the fixes applied on top of the branch.

## Fixes in this change

1. **Fail-open guard (security-critical).** `sandbox/06-setup-network-policy.sh`
   `helix_apply_sandbox_network_policy` ignored the return status of
   `helix_ensure_sandbox_network` and `helix_configure_ipv6_policy`. On a bridge
   mismatch or missing `ip6tables` it still removed the fail-closed bootstrap
   guard and published the ready marker → sessions ran with NO egress policy
   while reporting healthy. Now both calls `|| return 1`, so the dockerd restart
   loop (04-start-dockerd.sh:188) retries with the guard still installed and the
   ready marker unpublished.

2. **N1 — settings-sync-daemon wildcard bind (cross-session).** The daemon bound
   `*:9877`; any peer on the shared bridge could `GET/PUT /settings` and inject
   model-provider config into another tenant's agent. Now binds `127.0.0.1`
   (same fix PR 3147 applied to the desktop-bridge on :9876). In-container
   consumers (Zed, desktop-bridge, agents) reach it over loopback.

3. **N2 — proxy over-forwards `/debug/pprof` through the isolated route.** The
   Hydra sandbox API proxy is a blind pass-through, so `192.0.2.1:18080/debug/pprof/`
   (and via the 8080→18080 REDIRECT, `:8080/debug/pprof/`) returned 200 unauth —
   re-exposing H7 through the "isolated" path. The proxy now rejects `/debug/`
   with 404. (`/api/v1/config` disclosure is a separate API-auth/H3 concern, out
   of egress scope — sandboxes legitimately need `/api/v1/*`.)

4. **Legacy-container bypass (security-critical).** Persistent web-service
   containers (`RestartPolicy: unless-stopped`, gated on `req.Persistent`)
   created before the upgrade are restored *running* by dockerd on the legacy
   bridge, a lifecycle event that never calls `CreateDevContainer`, so migration
   never fires and they keep unrestricted egress forever. Fix: on hydra recovery
   (`RecoverDevContainersFromDocker`), any recovered container NOT attached to
   `helix-sandboxes` is live-migrated by connecting it to the isolated network
   and disconnecting the legacy one — non-destructive (writable layer preserved,
   unlike force-recreate). Failures are non-fatal so recovery still completes.
   Only persistent web-services hit this (desktop spec-task sessions don't get
   `unless-stopped`, so dockerd never auto-restores them).

5. **Proxy TLS trust (enterprise regression).** The proxy used the default
   transport (full verification) while every sibling sandbox→API client
   (RevDial, heartbeat, settings-daemon) sets `InsecureSkipVerify` for private-CA
   installs. A private-CA enterprise deployment would 502 all in-session traffic
   while looking healthy. The proxy transport now matches the siblings.

6. **`log.Fatal` crash-loop on scheme-less `HELIX_API_URL`.** `api:8080` (host:port,
   no scheme) made `newSandboxAPIProxyHandler` reject and hydra `log.Fatal` at
   boot, crash-looping the whole sandbox. Now a scheme-less upstream is coerced
   to `http://`, and a truly unparseable one logs a loud error and skips the
   proxy (hydra lifecycle stays up and diagnosable) instead of crash-looping.

7. **URL-classifier credential leak (N-review).** `isHelixMCPURL` in the daemon
   classified by URL *path* only (`/api/v1/mcp`), so a third-party MCP server
   whose path starts with that prefix was rewritten to the Helix proxy — sending
   its Authorization credential to the Helix API. Now it also requires a
   Helix-owned host.

## API-emits-canonical-URL refactor (DONE — the root-cause simplification)

Replaced the entire downstream address-rewrite layer by having the **control
plane emit the canonical sandbox proxy URL directly** at config-generation time.

- New single constant `hydra.SandboxAPIProxyURL = http://helix-api.internal:18080`.
- `buildEnvVars` (external-agent) stamps it into `HELIX_API_URL`,
  `HELIX_API_BASE_URL`, `ANTHROPIC_BASE_URL`, `OPENAI_BASE_URL`, `ZED_HELIX_URL`.
  Subscription mode still overrides `ANTHROPIC_BASE_URL` to api.anthropic.com
  afterwards (external egress is allowed).
- `zed_config_handlers.go` passes it into `GenerateZedMCPConfig` /
  `buildCodeAgentConfig` at all three resolution sites, so every MCP URL,
  language-model `api_url`, the code-agent `BaseURL`, and the Zed WebSocket sync
  `external_url` are canonical at the source.
- **Deleted** the downstream rewriters that existed only to patch the API's own
  address: hydra `rewriteHelixEndpointEnv` + `isHelixEndpointURL` + the buildEnv
  override block; daemon `rewriteHelixConfigURLs` + `rewriteHelixAPIURL` +
  `isHelixMCPURL` + their call sites and tests. Also removed the dead
  `GetZedConfigForSession` (the legacy `outer-api` rewrite) and the now-unused
  `HydraExecutorConfig.HelixAPIURL` field.
- **Eliminates two confirmed bugs by construction:** the `isHelixMCPURL`
  credential leak (couldn't be safely patched before) and the `isHelixEndpointURL`
  split-horizon miss. There is nothing left to classify or guess.

Trade-off: this couples the API and sandbox versions to the proxy being present
(a newer API emitting `helix-api.internal` to a pre-3148 sandbox with no proxy
would break). Acceptable now that the isolated bridge + proxy are universal;
noted for the PR.
- **Legacy-migration end-to-end test.** A fresh dev stack has no pre-upgrade
  persistent containers, so fix #4's migration branch cannot be exercised here;
  only the no-op path (already-isolated containers) is verified live. Needs a
  seeded legacy container to test the migration branch.

## Manual test log (dev stack, localhost:8080) — 2026-08-31

Hydra binary + network-policy script hot-deployed into `helix-sandbox-nvidia-1`
(`docker cp` + `pkill -TERM hydra`); hydra came back up clean (RevDial connected,
proxy on `192.0.2.1:18080`, upstream `http://api:8080`). Tests run from a
throwaway `alpine` container on the `helix-sandboxes` bridge.

- **N2 /debug block — PASS.** `:18080/debug/pprof/` → 404 (body "not found", no
  goroutine dump); `:18080/debug/pprof/goroutine` → 404; `:8080/debug/pprof/`
  (via the `-i helix-sbx0` REDIRECT) → 404. So H7 is closed through both the
  proxy port and the redirected gateway port.
- **API forwarding still works — PASS.** `:18080/api/v1/config` → 200;
  `:8080/api/v1/config` (REDIRECT) → 200. Confirms the new InsecureSkipVerify
  transport (#5) doesn't break the http path and the `/debug` guard doesn't
  touch normal routes.
- **Egress isolation intact — PASS.** Postgres `192.0.2.1:5432` → connection
  refused (curl exit 7 / 000); public `https://github.com` → 200.
- **Fail-closed guard (#1) — PASS.** With `helix_ensure_sandbox_network` forced
  to fail: `helix_apply_sandbox_network_policy` rc=1, ready marker ABSENT. Same
  with `helix_configure_ipv6_policy` forced to fail: rc=1, marker ABSENT. Real
  apply afterwards: rc=0, marker PRESENT. Live network re-verified healthy after
  the poking (API 200, Postgres blocked, public 200).
- **N1 loopback bind — PASS.** Rebuilt `settings-sync-daemon` binary run with a
  dummy session: `ss -tlnp` shows `127.0.0.1:9877` (local address only), not
  `0.0.0.0:9877`. Peer containers on the bridge can no longer reach it.
- **Scheme-less coercion (#6) — PASS.** Unit test
  `TestSandboxAPIProxyCoercesSchemelessUpstream` (`api:8080` accepted, coerced to
  http); `TestSandboxAPIProxyRejectsInvalidUpstream` still rejects `""` and
  `ftp://…`.
- **Legacy migration (#4) — NOT TESTED (no legacy containers on a fresh stack).**
  Code builds; no-op path (all containers already on `helix-sandboxes`) verified
  by the stack staying healthy after the new hydra recovered on restart.

Unit tests: `go test ./api/pkg/hydra/` green except pre-existing ZFS-environment
failures in `TestDiskPressureSuite` (this host has real ZFS; those assertions
predate and are unrelated to this change). `go test ./api/cmd/settings-sync-daemon/`
green.

## Canonical-URL refactor — live validation (dev stack, 2026-08-31)

API (Air) + hydra redeployed with the refactor.

- **API emits canonical URLs — PASS (live).** `GET /api/v1/sessions/<id>/zed-config`
  returned `helix-api.internal:18080` for EVERY URL: `code_agent_config.base_url`
  (`/v1`), both `language_models` (`anthropic`, `openai/v1`), all three
  `context_servers` MCP URLs, and `external_sync.websocket_sync.external_url`. No
  rewriting anywhere downstream.
- **Canonical URLs forward through the proxy — PASS (live, from a bridge
  container).** MCP session endpoint → 200; **WebSocket sync endpoint → 101
  Switching Protocols** (Zed's real-time sync upgrades cleanly through the proxy —
  the highest-risk path); chat `/v1/models` → 200.
- **`buildEnvVars` canonical env — PASS (unit)**
  `TestBuildEnvVarsEmitsCanonicalSandboxAPIURL`.
- **hydra passes env through / daemon uses config as-is — PASS (unit + suites).**
- **NOT run:** a single brand-new spec-task agent turn with the freshly-built
  daemon binary. The outer dev stack was at capacity (10/10 implementation, 3/3
  planning) with real tasks I must not disrupt. Every integration seam is proven
  live individually (emission + MCP/WS/chat forwarding), and the daemon change is
  "stop rewriting, use config verbatim" against config proven canonical live.

## Rebuilt-image E2E — fresh spec tasks, both runtimes (2026-08-31)

Images rebuilt from the working tree and deployed: `helix-ubuntu:caac18`
(`./stack build-ubuntu`, one retry after a transient rust-lang.org DNS failure)
and `helix-sandbox:latest` (`./stack build-sandbox`, recreates the sandbox
container — safe, no live task containers existed). New sandbox boot logs showed
the fail-closed policy applying (`Isolated sandbox network ready`) and the proxy
on `192.0.2.1:18080`.

Two fresh spec tasks created via `helix spectask start` on project helix-next:
`--runtime headless-ubuntu` (spt_01m1bfzpwys8sm8jv0vbnd8knt /
ses_01m1bg76dwgvwj61bh0x8zj0q2) and `--runtime ubuntu-desktop`
(spt_01m1bfzq018n9x8b2wcjdyc7xt / ses_01m1bg76dnkyqz66jtxwrenap5). To free the
project's planning WIP slots, three zombie `spec_generation` tasks from
2026-08-16/22 (sandboxes long gone) were set to `archived`.

Results — ALL PASS on both runtimes:

- Containers provisioned on `helix-sandboxes` (192.0.2.2 / .3).
- Canonical env in both: HELIX_API_URL / HELIX_API_BASE_URL / ANTHROPIC_BASE_URL
  = `http://helix-api.internal:18080`, OPENAI_BASE_URL `…/v1`,
  ZED_HELIX_URL `helix-api.internal:18080`, TLS=false.
- From inside each container: canonical API → 200; `/debug/pprof` → 404 (N2);
  Postgres :5432 and 172.19.0.2 → blocked; public HTTPS → 200.
- settings-sync-daemon (baked image) listening on `127.0.0.1:9877` (N1).
- **Two-session lateral test (the gap infra#70 flagged): desktop→headless:9877
  and headless→desktop:9877 both blocked.**
- Zed WS sync connected in both (~10s; `zed_thread_id` set) through the proxy.
- Live agent turns completed in both: 6 (headless) + 11 (desktop) `llm_calls`
  rows, ZERO errors; agents renamed their tasks, wrote+pushed three spec docs
  each (git-over-HTTP via the proxy) → both tasks reached `spec_review`.
- Desktop screenshot captured: GNOME streaming, Zed agent thread showing
  `git push origin helix-specs`, the `helix-session_task_completed` MCP tool
  call, and `hello-desktop.txt` = "desktop egress isolation works" in the
  editor. Nested docker builds also visible running in the desktop terminal.

## Persistence note

The hot-`docker cp` deploy is ephemeral — a sandbox container rebuild reverts to
the baked binary/script. To persist: `./stack build-sandbox` (hydra + network
policy) and `./stack build-ubuntu` (settings-sync-daemon), then start a new
session. Source files are edited, so CI/image builds will bake them.
</content>
</invoke>
