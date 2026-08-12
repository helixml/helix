# Design: Persist Chrome Profile Across Desktop Container Restarts

## Overview

Two small, surgical changes:

1. Add `--user-data-dir=/home/retro/work/.chrome-state` to the `chrome-devtools` MCP
   server args, moving the agent's Chrome profile off the ephemeral overlay onto the
   persistent workspace bind mount.
2. Delete the GUI Chrome auto-launch + heartbeat block from both desktop startup scripts,
   so nothing else can claim `SingletonLock` on that profile.

No new files, no new abstractions, no migration code.

## Filesystem facts this design rests on

```
overlay                2.0T  ...  /                     <- ~/.cache lives here (EPHEMERAL)
prod/helix-workspaces  2.5T  ...  /home/retro/work      <- ZFS bind mount (PERSISTENT)
```

`/home/retro/work` is the only persistent path in the desktop container — see
`api/pkg/sandbox/controller_provision.go`. Everything else dies on recreate.

## Change 1 — persistent `--user-data-dir`

**File:** `api/pkg/external-agent/zed_config.go` (~line 309, section "5. Add Chrome
DevTools MCP server")

```go
Args: []string{
    // Persist the profile on the workspace volume. chrome-devtools-mcp defaults
    // to $HOME/.cache/chrome-devtools-mcp/chrome-profile, which is on the
    // container's overlay fs and dies on every recreate — costing an interactive
    // re-login (LinkedIn 2FA) each time. Only /home/retro/work is bind-mounted.
    // NOTE: the ~/.config/google-chrome symlink in helix-workspace-setup.sh does
    // NOT cover this — an explicit --user-data-dir bypasses the default path
    // entirely, which is precisely why .chrome-state has never held a real profile.
    "--user-data-dir=/home/retro/work/.chrome-state",
    "--viewport", "1280x800",
    "--chrome-arg=--disable-blink-features=AutomationControlled",
    "--chrome-arg=--no-first-run",
    "--chrome-arg=--disable-infobars",
    "--chrome-arg=--disable-extensions",
},
```

Flag verified against the installed v0.25.0 (`Dockerfile.ubuntu-helix:958` pins
`chrome-devtools-mcp@0.25.0`): `--userDataDir  Path to the user data directory for Chrome.`
yargs accepts the kebab-case `--user-data-dir` form.

`helix-workspace-setup.sh` already `mkdir -p`s `.chrome-state` and seeds the first-run
sentinels before any agent starts, so the directory exists when the MCP launches.

## Change 2 — delete the GUI Chrome auto-launch

**Files:** `desktop/ubuntu-config/startup-app.sh` (~328-363),
`desktop/sway-config/startup-app.sh` (~600-635)

Delete the marker check, the `Singleton*` cleanup, the `google-chrome-stable` launch and
the 30s heartbeat loop. In the ubuntu script the block lives inside a heredoc (note the
escaped `\$` / `\$(...)`); delete the whole parenthesised subshell including its trailing
`) >/dev/null 2>&1 &`. In the sway script the launch sits inline under the
`SERVICES_STARTED` gate with the heartbeat as a separate subshell — remove both, leave
`SERVICES_STARTED=true` and the surrounding portal/settings-sync startup intact.

### Why delete rather than repair

Once the MCP owns `/home/retro/work/.chrome-state`, the block is no longer merely broken
— it is dangerous. If the GUI Chrome ever wins the race and takes that profile's
`SingletonLock` first, the MCP's subsequent launch detects the singleton, hands its
command line to the running instance and exits. Puppeteer's `--remote-debugging-pipe`
connect then fails and the entire chrome MCP breaks with no browser at all. Because
`.was-running` is effectively always set (the heartbeat's `pgrep -x chrome` matches the
MCP's own Chrome), that collision would be the normal case, not the edge case.

Repairing the readiness loop — e.g. copying the Zed launcher's GNOME-Shell D-Bus check —
would make it *more* likely to fire successfully, which is the wrong direction.

Losing the feature costs nothing. A human clicking the Chrome icon runs
`google-chrome-stable` with the default profile path, which the workspace-setup symlink
points at `.chrome-state` — the same profile the MCP is driving — so Chrome's singleton
hand-off just opens a window/tab in the MCP-controlled browser. That is the behaviour we
actually want: **one browser, one profile, one login**, visible on the streamed desktop
and drivable by the agent.

## Change 3 — keep the symlinks, update the comment

**File:** `desktop/shared/helix-workspace-setup.sh:645-682`

The symlinks are what makes the unification above work — keep them. Update the comment
block to say the chrome-devtools MCP is now the primary consumer of `$CHROME_STATE_DIR`
(via an explicit `--user-data-dir` from `zed_config.go`), and that the symlinks exist so
a human-launched GUI Chrome lands on the *same* profile.

## Rollout behaviour

- **New desktop sessions** get the flag at spawn. Nothing else needed.
- **Already-running desktops** keep the old flags until their agent restarts.
  `chrome-devtools` is in `HELIX_OWNED_CONTEXT_SERVERS`
  (`api/cmd/settings-sync-daemon/main.go:1549-1553`), so the daemon's next sync overwrites
  the stale on-disk `settings.json` entry wholesale rather than deep-merging it as a
  "user override" — that path is confirmed. Claude Code's `--mcp-config` is materialised
  at agent spawn, so it lags until restart. Confirm this in code during implementation and
  note it in the PR; **do not add a workaround.**
- **Existing ephemeral profiles are lost.** Accepted. No migration.
- **Disk.** A real profile is ~170MB including `Default/Cache` and `Default/Code Cache`,
  now landing on `prod/helix-workspaces` per spec task instead of the container overlay.
  Note in the PR; build no pruning mechanism unless it proves to be a problem.

## Testing strategy

### Unit (necessary, nowhere near sufficient)

`api/pkg/external-agent/zed_config_test.go` — assert the `chrome-devtools` context server
args contain `--user-data-dir=/home/retro/work/.chrome-state`. Follow the existing table
style in that file. Plus `cd api && go build ./...`.

### End-to-end (this is the part that matters)

CLAUDE.md is explicit: test the NEXT operation, not just the state change. "A flag is
present in a config struct" is not evidence the feature works.

In the inner Helix at `http://localhost:8080` — register `test@helix.ml` / `helixtest`,
complete onboarding, run `./stack build-ubuntu` (both changed desktop files ship in the
image), create a spec task, then on a live desktop session:

1. `ps aux | grep chrome` → running Chrome carries
   `--user-data-dir=/home/retro/work/.chrome-state`, not the `.cache` path.
2. Drive the chrome MCP to a site that sets a cookie → confirm
   `/home/retro/work/.chrome-state/Default/Cookies` exists and is non-empty on the
   persistent mount.
3. **Recreate the desktop container** → cookie still present, MCP still works
   (`list_pages` / `navigate_page` succeed). Skipping this verifies nothing.
4. No GUI Chrome auto-launch fires; no `/tmp/chrome-autolaunch.log`; no `SingletonLock`
   collision in the logs.

Anything not achievable goes in the PR as `NOT tested: <what and why>`. Do not write
"covered by unit tests" and do not reason by analogy to `.claude-state` or other
persistence paths — those work via the default-path symlink, the exact mechanism this bug
proves does not apply to an explicit `--user-data-dir`.

## Notes for future agents

- **The symlink trap.** `helix-workspace-setup.sh` persists tool state by symlinking a
  default config path into `$WORK_DIR`. That pattern silently fails for any tool that
  passes an explicit path override. If you add persistence for a new tool, check whether
  it takes a `--*-dir` flag before assuming the symlink covers it.
- **Diagnosing "persisted but empty".** `.chrome-state` looked alive (crashpad + metrics
  files) while holding zero session state, because crashpad resolves via
  `--database=~/.config/google-chrome/Crash Reports` rather than `--user-data-dir`.
  Check for `Cookies` / `Login Data` / `Local State` specifically, not just mtimes on the
  directory.
- **Two Chromes is the bug.** The desktop image can produce a GUI Chrome and an
  MCP-driven Chrome. They must share one profile and one process, mediated by Chrome's
  own singleton hand-off — not run as two instances fighting over `SingletonLock`.
- `HELIX_OWNED_CONTEXT_SERVERS` in `api/cmd/settings-sync-daemon/main.go` is how
  Helix-managed MCP config changes reach existing desktops; user-configured MCPs are
  deliberately not in that set. See its comment block for the PR #2418 history.
