# Org Bot browser resume on the configured profile

Date: 2026-11-19. Baseline e7d78e2a5 (+ durable bot homes, 004250).
Branch feature/004251-resume-org-bot-chrome-on.

## Problem

An idle stop removes the desktop container; the bot home (`/home/retro/work`)
survives (004250). Chrome does not. The shared browser came back lazily, but
always on `.chrome-state`, because `zed_config.go` hard-codes that path and
the MCP wrapper's own fallback was `~/.cache/...`. Birding Promoter logs in on
a custom `chrome-profile`, so it ran a guard script that watched the port and
launched its own Chrome. Two owners, two profiles, and "the port answers" was
taken to mean "the right account is logged in".

## Design

`desktop/shared/helix-browser.sh` (installed as `/usr/local/bin/helix-browser`)
is the only thing that starts the sandbox Chrome. Not a daemon: it extends the
existing lazy relay (e62cc1724). Every path goes through it:

| Caller | Command |
|---|---|
| chrome-devtools-mcp wrapper (every MCP server start) | `relay`: socat listener on 9222, **no Chrome** |
| first connection to 9222 (first browser tool call) | `connect`: ensure, then relay to 9223 |
| desktop Chrome icon, http(s) links (`google-chrome.desktop`/`chromium.desktop` Exec), Sway bar and key | `open`: ensure, then hand the URL to the running Chrome |
| bot or operator | `ensure`, `status`, `login-check`, `declare`, `stop` |

### Configured profile

`~/work/.helix/browser.json` (in the durable home, non-secret):

```json
{"profile": "chrome-profile", "profile_id": "<uuid>",
 "login_check": {"url": "https://site/home", "logged_out_url_contains": ["/login"],
                 "logged_in_title_contains": "Inbox"}}
```

`helix-browser declare chrome-profile [--login-url URL --logged-out-match TEXT --logged-in-title TEXT [--logged-in-match TEXT]]`
writes it, and `chrome-profile/.helix-profile.json` with the same id. Declare
refuses a directory that is missing, outside the workspace (after resolving
every symlink), or not a Chrome
profile (no valid `Local State`), and switching to another profile needs
`--replace`. No declaration → `.chrome-state` (the old behaviour). A declared
profile wins over the MCP server's `--user-data-dir` (logged).

### Single owner

- `flock /tmp/helix-chrome.lock` serialises every decision inside a container
  (concurrent MCP clients, relay connections, desktop clicks).
- Chrome's `SingletonLock` (`<host>-<pid>`) decides ownership. It is a **live
  owner** only if the host is this container and the PID is a non-zombie
  process whose `comm` is chrome/chromium. The hostname
  (`ubuntu-external-<session>`) is stable across container recreation, so
  hostname + "PID alive" was not proof. Chrome rewrites its cmdline, so
  `comm` is used.
- Live owner with Helix's DevTools endpoint → reused. Live owner without it
  (Chrome started by hand) → fail, naming the PID; never a second browser.
- Lock from another host, or dead/reused PID → stale. Only `Singleton*` files
  are removed, with the reason logged; the profile is never touched. Other
  containers are excluded by Hydra's one-container-per-home fence.
- A browser on 9223 that does not own the configured profile (declaration
  switched while it ran) → fail until `helix-browser stop`.
- Only Helix's relay may hold 9222. Anything else there (a guard script's
  Chrome) fails the MCP server start with the PID instead of silently
  attaching to an unknown profile.

### Refusals (no logged-out replacement)

| Condition | Result |
|---|---|
| declared profile missing | `missing`; nothing created |
| profile (declared or default) resolves, through any symlink, outside the workspace | `outside_workspace`, checked at declare and every status/launch/setup |
| directory without the declared identity / with another id | `wrong_identity` |
| no or unparseable `Local State` | `inconsistent` |
| workspace not its own mount (container overlay) | refused for a declared profile; warning for the default |
| invalid declaration | refused, profile untouched |

Workspace setup links `~/.config/google-chrome` to the configured profile
only when it is usable, and never creates or seeds a declared profile.
Browser death or a refusal never deletes anything.

### Separate results

`helix-browser status [--json]` never starts Chrome and never connects to
9222:

- `profile`: state, source (declared/default), identity, `Local State` present, cookie DB present (stat only)
- `mount`: mounted / not_mounted
- `process`: running / stopped / stopped_with_stale_lock (+ PID, reason)
- `cdp`: ready / not_answering / not_running / other_profile; debug-port listener relay / foreign / absent
- `login`: not_configured / not_checked

`helix-browser login-check` opens the declared URL in a new tab through
DevTools' HTTP API, waits for redirects to settle and closes the tab. It reads
only the tab's URL and title. `logged_in` needs positive evidence: an http(s)
page at the declared URL (or a declared `--logged-in-match` URL) **and** a
title containing the declared `--logged-in-title`. The URL alone is not
evidence, because Chrome's network-error pages keep the requested URL and are
titled with the bare host. A declared logged-out URL gives `logged_out`.
Error pages (`chrome-error://`, or a title equal to the host), other
destinations and a missing title give `unknown`. Only `logged_in` exits 0.
The output is the state, the landing origin and path (query and fragment
stripped) and a reason. No cookie or login database is read.

Hydra's bot-home bind inventory adds `browser_profile {path, state}` (same
checks, symlinks confined to the home, warning log when not ok). It only
reports: a bot without a usable profile still starts and runs non-browser
work; its browser requests fail with the reason.

## Verification (2026-11-19)

**Unit:** `desktop/shared/test-helix-browser.sh` (66 checks, fake Chrome with
singleton hand-off and DevTools HTTP API; runs in CI `shell-script-tests`,
also passes on alpine:3.21). `TestDeclaredBrowserProfile` (hydra).

**Live, real Chrome 156 in a desktop container** (temporary workspace, ports
9322/9323, local login site setting a persistent non-secret cookie):

| Case | Result |
|---|---|
| Custom `chrome-profile` made outside Helix (guard-script style), declared | identity written, declaration as above |
| MCP servers start | relay only, no Chrome process |
| 2 concurrent real chrome-devtools-mcp clients, `list_pages` | both answered (`## Pages`), one browser process, on `chrome-profile`; `.chrome-state` not created |
| Login inside the managed browser | `logged_in` |
| SIGKILL (idle-stop style) 40 s after login, next request | resumed on the same profile, new PID, `logged_in` |
| Lock naming another container, new run dir (session replacement) | stale lock cleared with reason, resumed |
| After kills/stops: `PRAGMA integrity_check` on Cookies, History, Login Data | `ok` (read-only, no rows read) |
| Profile moved away | relay request refused, `ensure` exit 1 with reason, nothing created |
| Copy without identity in its place | refused; real profile restored → resumes |
| Desktop `open` twice (headful, Wayland) | first starts Chrome, second "Opening in existing browser session"; same owner, one process |
| Workspace-setup block: none / declared / declared-missing | link to `.chrome-state` / to `chrome-profile` / no link, nothing created |
| Review fix: profile reached through a symlink to other storage | `declare` refused; swapped-in symlink to a full copy → `ensure` exit 1, status `outside_workspace`, no Chrome |
| Review fix: login URL with nothing listening (real Chrome error page, URL unchanged, title `127.0.0.1`) | `unknown`, exit 1 (was `logged_in`) |
| Review fix: logged in after stop/resume, declared title present / wrong title declared | `logged_in` exit 0 / `unknown` exit 1 |

Chrome writes new cookies to disk on its ~30 s commit timer. A login less than
about 30 s old is lost if Chrome is killed, and SIGTERM did not flush
(headless or headful) in this test. That is Chrome's behaviour and applies to
idle stops as well.

## Not covered / limits

- **NOT tested: a built desktop image.** Neither `./stack build-ubuntu` nor
  the dev stack were run. Not tested end to end: the Dockerfile `ADD` and the
  `.desktop` Exec rewrite, and a real Org Bot through idle stop → restart →
  image rebuild → session replacement. Each building block was run as listed
  above.
- Tabs, drafts and form contents are not durable (journal work, OBR-04/05).
- Birding Promoter is **not** migrated. That needs an operator, inside its
  desktop: `helix-browser status`, then
  `helix-browser declare chrome-profile --login-url <url> --logged-out-match <text> --logged-in-title <text>`,
  then retire the guard script (once the relay holds 9222, the guard sees a
  live port, and its probes start the configured browser). This is a live
  profile mutation, so it is not done here.
- Runner loss and off-runner profile backup remain separate opt-in scope.
- A Chrome started from a terminal as plain `google-chrome` (not
  `helix-browser open`) still uses the `~/.config` symlink. While the managed
  browser runs it hands off to it. Started first, it owns the profile without
  DevTools and browser requests fail naming it.
