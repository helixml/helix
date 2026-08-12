# Requirements: Persist Chrome Profile Across Desktop Container Restarts

## Background

Long-running bot desktops lose their logged-in browser state every time the desktop
container is recreated. For GTM bots this is expensive: LinkedIn requires interactive
2FA, so every restart costs a human round-trip (`request_human_attention` → a human
logs in by hand). The bot prompt asserts "Your session persists between daily runs" —
that is currently false.

### Root cause 1 (primary) — the MCP's Chrome uses an ephemeral profile

`chrome-devtools-mcp` launches its own Chrome and passes an explicit `--user-data-dir`.
Live evidence from a running desktop container:

```
/opt/google/chrome/chrome ... --ozone-platform=wayland \
  --user-data-dir=/home/retro/.cache/chrome-devtools-mcp/chrome-profile
```

That is the tool's documented default (`$HOME/.cache/chrome-devtools-mcp/chrome-profile`).
`~/.cache` is on the container's overlay filesystem, destroyed on every recreate. Only
`/home/retro/work` is a persistent bind mount (ZFS `prod/helix-workspaces`).

The persistence we *do* have — `desktop/shared/helix-workspace-setup.sh:645-682`
(PR #2452, commit `0f459036e`) — symlinks `~/.config/google-chrome` and
`~/.config/chromium` to `$WORK_DIR/.chrome-state`. **An explicit `--user-data-dir`
bypasses that symlink entirely**, so the persistent directory has never held a real
profile. After 4 days of a bot running, `.chrome-state` contained only a 3.5KB stub
`Default/Preferences` plus `BrowserMetrics/`, `Crash Reports/` and `Variations/` — no
`Cookies`, no `Login Data`, no `Local State`, no `History`. The crashpad/metrics files
land there only because crashpad is passed
`--database=/home/retro/.config/google-chrome/Crash Reports`, which resolves via the
config path rather than `--user-data-dir`. That is what makes the directory look "used"
while holding no session state.

### Root cause 2 (independent) — GUI Chrome auto-relaunch races the compositor

The GUI-Chrome restore path wired to `.chrome-state`
(`desktop/ubuntu-config/startup-app.sh:328-363`) is broken on its own terms:

```
[2026-08-12T05:38:26+01:00] Auto-launching Chrome (marker present)
ERROR:...wayland_connection.cc:202] Failed to connect to Wayland display: Connection refused (111)
ERROR:...ozone_platform_wayland.cc:288] Failed to initialize Wayland platform
ERROR:ui/aura/env.cc:246] The platform failed to initialize.  Exiting.
```

Its readiness loop only waits for the socket *file* to exist
(`[ -S "${XDG_RUNTIME_DIR}/wayland-0" ]`) rather than for the compositor to accept
connections — unlike the Zed launcher immediately above it, which does a real
GNOME-Shell D-Bus `ShellVersion` check. Chrome dies within a second, every boot.

The heartbeat's liveness probe is `pgrep -x chrome`, which matches the *MCP's* Chrome.
So `.was-running` is always touched and auto-launch always fires on the next boot,
regardless of whether a GUI Chrome was ever opened. Today that is harmless only because
the launch crashes.

## Scope

Forward-looking only. New desktop sessions created after this change must keep their
browser profile across container restarts. Profiles that already exist in the ephemeral
`~/.cache` location are lost, and that is fine — **no migration, import or backfill
logic.**

## User Stories

### US-1: Agent browser sessions survive a container restart

**As** an autonomous bot running in a Helix desktop,
**I want** my Chrome profile (cookies, logins, local storage) stored on the persistent
workspace volume,
**so that** a container recreate does not force a human to redo interactive 2FA.

**Acceptance criteria:**
- The `chrome-devtools` context server in `api/pkg/external-agent/zed_config.go` passes
  `--user-data-dir=/home/retro/work/.chrome-state`.
- A comment at that line explains *why*: only `/home/retro/work` is bind-mounted, and
  the `~/.config/google-chrome` symlink in `helix-workspace-setup.sh` does **not** cover
  an explicit `--user-data-dir`.
- On a live desktop, `ps aux | grep chrome` shows the running Chrome with
  `--user-data-dir=/home/retro/work/.chrome-state` and **not** the `.cache` path.
- After driving the chrome MCP to a site that sets a cookie,
  `/home/retro/work/.chrome-state/Default/Cookies` exists and is non-empty.
- After recreating the desktop container, that cookie file is still present and the MCP
  still works (Chrome launches, `list_pages` / `navigate_page` succeed).

### US-2: One browser, one profile — no singleton collision

**As** a platform maintainer,
**I want** the broken GUI Chrome auto-launch block removed rather than repaired,
**so that** the GUI Chrome cannot grab `SingletonLock` on the profile before the MCP does.

**Acceptance criteria:**
- The Chrome auto-launch + heartbeat block is deleted from
  `desktop/ubuntu-config/startup-app.sh` (~328-363) and from
  `desktop/sway-config/startup-app.sh` (~600-635).
- The `~/.config/google-chrome` and `~/.config/chromium` symlinks in
  `helix-workspace-setup.sh` are **kept**; the comment there is updated to state that the
  chrome-devtools MCP is now the primary consumer of `.chrome-state`.
- On a live desktop no GUI Chrome auto-launch fires, `/tmp/chrome-autolaunch.log` is not
  created, and no `SingletonLock` collision appears in the logs.
- A human clicking the desktop Chrome icon still reaches the same profile (via the
  symlink) and hands off to the MCP-controlled instance.

### US-3: The fix is proven by restart, not by inspection

**As** a reviewer,
**I want** end-to-end evidence from a real restart,
**so that** "a flag is present in a config struct" is not mistaken for a working feature.

**Acceptance criteria:**
- `cd api && go build ./...` passes.
- A Go unit test in `api/pkg/external-agent/zed_config_test.go` asserts the
  `chrome-devtools` context server carries the persistent `--user-data-dir`.
- End-to-end run in the inner Helix at `http://localhost:8080`: register
  `test@helix.ml` / `helixtest`, complete onboarding, `./stack build-ubuntu` (both changed
  desktop files ship in the image), create a spec task, and perform the checks in US-1
  and US-2 — **including the container recreate**. A run that skips the restart has
  verified nothing.
- Anything genuinely not achievable is stated in the PR as `NOT tested: <what and why>`.
  Not acceptable: "covered by unit tests", or reasoning by analogy to `.claude-state` and
  other persistence paths — those use the default-path symlink, the exact mechanism this
  bug proves does not apply here.
- The PR notes that already-running desktops keep the old flags until their agent is
  restarted, and that a real profile (~170MB incl. `Default/Cache` and
  `Default/Code Cache`) now lands on `prod/helix-workspaces` per spec task.
- Branch, build, test, push, open a PR against `helixml/helix` main with full PR URLs in
  the report. Do not merge.

## Non-Goals

- No migration, import or backfill of existing ephemeral `~/.cache` profiles.
- No fallback path if `.chrome-state` is unavailable — per CLAUDE.md, one approach, no
  dead code.
- No repair of the GUI Chrome auto-launch readiness loop; it is deleted.
- No profile pruning / cache-size mechanism. Note the size in the PR; build nothing
  unless it is demonstrated to be a real problem.
- No workaround for already-running desktops.

## Open Questions

1. **Claude Code MCP materialisation.** The brief states `chrome-devtools` is in
   `HELIX_OWNED_CONTEXT_SERVERS` (`api/cmd/settings-sync-daemon/main.go:1549-1553`), so the
   new arg wins over stale on-disk `settings.json` on the next daemon sync, while Claude
   Code's `--mcp-config` is materialised at agent spawn and so keeps old flags until the
   agent restarts. A grep for `--mcp-config` / `mcpServers` under `api/` and `desktop/`
   found only a passing mention in `desktop/shared/helix-npx.sh:8` — the actual
   materialisation site was not located during planning. **Assumption:** the reasoning
   holds; the implementer should locate that code path, confirm it, and state it in the
   PR. No workaround either way.
2. **Sway parity is cosmetic.** Sway is experimental and its startup script is a
   different shape (`SERVICES_STARTED` gate, `wayland-1`). Assumed the intent is deletion
   for consistency only, with no sway E2E verification expected — ubuntu is the image
   that gets `./stack build-ubuntu` and the live test.
3. **`RestoreOnStartup=1` policy.** The Dockerfile sets a restore-on-startup Chrome
   policy that the deleted auto-launch block's comments reference. Assumed it stays as-is
   (harmless, and it still applies when a human opens Chrome manually). Flag if it should
   be removed too.
