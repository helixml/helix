# Org Bot durable home, non-destructive Restart, explicit Reset

Date: 2026-11-19. Baseline e7d78e2a5. Branch feature/004250-preserve-org-bot-data.

## Why

Birding Promoter lost its custom Chrome profile, scripts and backups after an
authenticated Restart on 2026-11-17 replaced its session. In source (since
591c712bd, 2026-10-30) Bot Restart ran `ResetSession` → `deleteSession` →
`DestroyDesktop`, which deletes `/data/workspaces/sessions/<ses>`: the only place
the bot's `/home/retro/work` lived. Whether production ran that commit on
Nov 17 was not verified here. The Nov 19 idle stop is a separate event: it
removes the container and keeps the workspace.

## State inventory

| State | Where | Owner / retention |
|---|---|---|
| Bot home: `/home/retro/work` incl. `.chrome-state`, custom profiles (e.g. `chrome-profile`), scripts, backups | `/data/workspaces/org-bots/<org>/<bot>/home` (was `sessions/<ses>`) | **(org, bot)**. Survives stop, idle stop, restart, session replacement, sandbox recreation. Removed only by Reset `session_and_home` (retired) or bot delete (purged). |
| Home owner record | `org-bots/<org>/<bot>/owner.json` (not mounted) | Hydra. org, bot, generation, origin |
| Home record | `org_bot_runtime_state` keys `home_state/_generation/_sandbox_id/_detail` | Bot runtime state; deleted with the bot |
| Container, Docker image | runner Docker | Disposable |
| Inner Docker data | `docker-data-<ses>` / `/container-docker/sessions/...` | Session; disposable cache |
| `/data/sessions/<ses>` (PipeWire, crash dumps), paused screenshot | runner | Session; destroyed with the session |
| Anything outside `/home/retro/work` (e.g. `~/.config`) | container layer | Lost on every container recreate (unchanged) |
| Bot instances (`org_bot_instance`) | own instance disk per session | Unchanged: session-owned by design |
| Conversation, prompts, activations, audit | Postgres / NATS | Session / bot queue |

Chrome: the desktop and Chrome DevTools MCP share `.chrome-state`; a custom
profile is just another directory in the home. Hydra logs an inventory on bind
(entry count; per profile: `Local State`/`Preferences` parse, cookie DB
present, `SingletonLock`). Cookie and login databases are stat'ed, never read.

## Binding rules (Hydra `bot_home.go`, API `external-agent/bot_home.go`)

Only a bot's main session (exploratory, not an instance, matching the runtime
session pointer) binds the home. The API reserves `pending(generation, runner)`
before calling Hydra and records `established` after; an established home pins
placement to its runner.

- `existing`: owner record must match generation and the data dir must exist;
  otherwise **fail** ("refusing to seed a fresh home").
- `new`: adopt the session's legacy `sessions/<ses>` by journaled atomic rename
  (`migration.json`, resumable at every step), or create an empty home.
- Both legacy and home populated, home without owner record, owner of another
  bot/org, journal for another generation: **fail**, touch neither side.
- One running container per home (label `helix.bot_home`); a stopped container
  still bound to the legacy path is recreated; after start Hydra verifies the
  container really mounts the home at `/home/retro/work`.
- An older Hydra that ignores `bot_home` → the API stops the desktop and fails.

## Operations

| Operation | Surface | Effect |
|---|---|---|
| Restart | `POST /bots/{id}/runtime-restart`, `restart_bot`, `helix org bots restart`, every UI control labelled Restart (bot menu, sidebar, chart, chat panel, session workspace, config banners) | Same session. Stop container (fence) → settle waiting turns as `External agent stopped…` → recreate. Queue untouched; the interrupted activation is redelivered and SendMessage recovers the **same prompt id**. No session yet → first activation. |
| Reset `session` | `POST /bots/{id}/reset`, `reset_bot`, `helix org bots reset`, Bot settings › Advanced › Danger zone | Purge queue + in-flight, close their open activation rows, delete session (+ desktop, session data), new session. Home kept. Refused (409) while the home still lives in the session. |
| Reset `session_and_home` | same | Also retire the home to `org-bots-retired/<org>/<bot>/<gen>-<ns>`; reaper purges after 7 days. Next start gets a new generation. |
| Legacy `POST /bots/{id}/restart` | deprecated, `Deprecation: true` | Keeps its documented contract (fresh session, queue purged) as an audited session-scope Reset; never erases the home. No first-party caller. |
| Stop | unchanged | Purges queue; not a recovery tool. |
| Bot delete | lifecycle | Purges the home after destroying desktops; host gone → reaper. |

`GET /bots/{id}/reset` returns each scope's `erases`/`keeps` and availability;
the dialog renders it and requires typing the bot's name. Reset needs the same
bot-edit authorization as delete and writes `org_audit_logs` rows
(`event_type=bot_lifecycle`, `action=reset_bot`, attempted + outcome, scope,
previous session, retired path, error).

Failed erase: record `erase_failed` with "the home may still exist" detail and
audit `failed`; starts refuse with that message (surfaced as the bot's
activation error) until a retried Reset succeeds. Purge failures leave data in
`org-bots-trash`, which the reaper retries.

GC: the reaper sends the set of existing bots (`BotHomesAuthoritative`);
homes of deleted bots are purged after the grace period, never while mounted;
retired homes after retention; trash always. Older APIs omit the flag and
nothing is reaped.

## Verification (local dev stack, 2026-11-19)

Canaries: random file in home, random file + real Chrome profile at custom
`chrome-profile` with a cookie from a local test login page; login checked by
page title through DevTools `/json` (cookie contents never read).

| Case | Result |
|---|---|
| Runtime restart (CLI) | new container, same session/generation; canaries, modes 750/600, profile, LOGGED-IN kept |
| Idle-stop path then start | container removed; all kept |
| Sandbox container recreated (stack restart) | homes on volume; all kept on both bots |
| In-flight + queued (cron `e-68e7` in flight on black-holed LLM, `e-0c53` queued) → Restart | old container gone before settle; turn settled; same event and prompt id replayed → ok; queued event ran next |
| Legacy layout (data in `sessions/<ses>`, no record) | session reset and legacy `/restart` refused 409; Restart migrated (`origin: migrated`), all kept |
| Established home missing | start fails visibly, nothing seeded |
| Legacy and home both populated | start fails, both unchanged; works after operator moves one aside |
| Reset `session` (CLI and UI) | new session, same generation, all kept; old session data and container gone; audited with user and scope |
| Reset `session_and_home` | home retired with canaries intact; new empty generation; other bots and other tenant's same-id bot unchanged |
| Erase failure after session deletion | 500, `erase_failed` persisted, data intact, audit failed, start refused visibly; retry succeeds |
| Cross-tenant (non-admin user, other org; slug and id) | 403 on every verb; state, containers, audit unchanged |
| UI | three-dot and sidebar: Restart only → `runtime-restart`; Reset only under Advanced › Danger zone; dialog lists scope from server; disabled until exact name; submit → `POST reset` scope session |

The dev stack runs `ADMIN_USER_IDS=all`; the cross-tenant run used a
temporary override so the second user was not a platform admin.

## Not covered / limits

- **Runner loss is unsupported.** The home lives on one runner; a start
  elsewhere fails rather than seeding. Off-runner recovery is separate opt-in
  work (recovery point, RTO, encryption, ownership).
- Image rebuild was not run live; it takes the same container-recreate path as
  Restart with a different image.
- No durable draft/external-operation journal (OBR-04/05 scope). Restart keeps
  the conversation and home files; open browser tabs are not durable.
- Reset/Restart serialisation is per API process; the home's single-mount
  fence in Hydra is the cross-process guard.
- Stop still purges queued work; an API crash mid-activation followed by Stop
  can leave an open activation row (Reset now closes these).
- Rolling back to an API without bot homes would start bots on an empty
  `sessions/<ses>` workspace; their homes stay in `org-bots/` untouched.
