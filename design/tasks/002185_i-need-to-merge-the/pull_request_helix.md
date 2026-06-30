# Merge helix-org Role and Worker into a single Bot concept

## Summary
The `helix-org` org-chart subsystem modelled two entities — **Role** (a job
description: content + tools) and **Worker** (a live instance holding a
`role_id`, a `kind` of human/ai, and per-worker `identity_content`). This PR
collapses them into **one** aggregate, the **Bot**: a single entity with an
`id`, markdown `content` (its prompt — its only identity), a `tools` list, a
`topics` manifest, and `parent_ids` (who it reports to). There is no `kind`,
no separate identity, and no role binding — a Bot *is* its own job
description. Creating a bot is now one step (was create-role + hire-worker);
deleting a bot is one step (was fire-worker + delete-role).

helix-org is pre-release, so there is no data backfill: migration `0005`
drops the changed tables and GORM AutoMigrate recreates them (`org_bots`,
bot-keyed `org_subscriptions`/`org_reporting_lines`/`org_bot_runtime_state`).

## Changes
- **Domain** (`api/pkg/org/domain`): new `orgchart.Bot` struct + `BotID`;
  deleted `Role`/`Worker`/`HumanWorker`/`AIWorker`/`WorkerKind`. `channels`
  now gives every bot a transcript; `activation.Trigger.SourceKind` removed;
  `streaming.Subscription.BotID`; `store` exposes one `Bots` repo +
  `BotRuntimeState`; `tool.Worker`→`tool.Caller`.
- **Persistence**: gorm `org_bots` table (merge of `org_roles`+`org_workers`),
  `org_bot_runtime_state`, `bot_id` columns; memory store merged repos.
- **Application**: merged `roles`+`workers` services into `bots`
  (create/update/add-parent/remove-parent/reconcile); `lifecycle`
  Hire/Fire/DeleteRole → Create/Delete.
- **MCP tools**: `create_bot` (was create_role+hire_worker), `update_bot`
  (was update_role+update_identity), `list_bots`/`get_bot`, `invite_bots`,
  `configure_bot_project`/`get_bot_project`, `bot_log`. Read/comms tools
  (managers, reports, dm, subscribe, publish, read_events, topics…) retained.
- **REST**: `/orgs/{org}/bots…` with a single `BotDTO` (replaces
  `/roles`+`/workers`, `RoleDTO`+`WorkerDTO`). Swagger + TS client regenerated.
- **Frontend**: merged Roles+Workers pages into a Bots list + Bot detail;
  `NewBotDialog`; Chart renders bots as nodes wired by reporting/subscription
  edges (no role group-frames); sidebar shows Chart / Bots / Topics / Settings.
- **DB**: migration `0005_merge_roles_workers_into_bots`.
- **Tests + QA.md** updated to the Bot model.

## Testing
- `go build ./...`, `yarn build`, and `go test ./api/pkg/org/...` all pass.
- Verified live in the inner Helix UI: create bot (→ `org_bots` row + base
  tools + auto transcript + provisioned project/agent), Bots list + detail
  (inline chat / tools / subscriptions / links), child bot → reporting
  reconcile (`s-transcript`/`s-team`/`s-dm` + subscriptions), delete bot →
  full cascade teardown. Confirmed `org_roles`/`org_workers` no longer exist.

## Screenshots
![Chart with a bot](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002185_i-need-to-merge-the/screenshots/01-chart-bot-created.png)
![Bot detail](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002185_i-need-to-merge-the/screenshots/02-bot-detail.png)
![Reporting topology](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002185_i-need-to-merge-the/screenshots/03-chart-reporting.png)

🤖 Generated with [Claude Code](https://claude.com/claude-code)
