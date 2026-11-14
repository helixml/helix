# Per-task sandbox seal (#3622)

Issue: https://github.com/helixml/helix/issues/3622. Builds on https://github.com/helixml/helix/pull/3611
(`confine_agent_key`) and https://github.com/helixml/helix/pull/3613 (turn provenance).

## Status: parked (2026-11-14)

The owner has parked this design. Nothing is implemented, and no further review rounds are planned
until it is picked up again.

- **Scope.** The seal gates **customer-mode (R2) planner runs only**: the PTaaS verifier and finder
  lanes. Other spec tasks are unaffected.
- **Current estimate.** About **41-44 engineer-days**, as of round 10:
  - slice 1: 25-27 days;
  - slice 2: 5-6 days;
  - slice 3: 5 days;
  - tests: 6 days.

  See §5.
- **Open items:**
  - **Owner decisions.** Whether to add break-glass (§4), and whether org owners get a stop for
    cost control (§3.5, §3.12). Either way, both are recorded distinctly from K's actions (§4).
  - **Cyber sign-off.** Pending on round 10 (head `fd1306561`). Cyber expected to sign off once the
    two round-10 blockers (NB5 choke points, N9-1 LLM call logs) were closed; that hasn't been
    confirmed yet.
  - **Still to confirm during implementation:**
    - every completion path of a `just_do_it_mode` sealed task reaches `done` without opening a PR
      or merging (§3.1);
    - Kodit, the wiki and semantic search register only shared repos, never a sealed copy
      (§3.10, §3.11);
    - the raw-SQL allowlist covers every `Raw` / `Exec` that reads lane-content tables, and no
      content-carrying aggregate escapes the default scope (§3.10);
    - git's `receive.autogc` and auto-maintenance honour the refcount-derived `gc.pruneExpire`, and
      the backup runbook snapshots `sealed/` together with the shared repos (§3.11);
    - the e2e test server doesn't depend on `POST /external-agents/{sid}/command` before it is
      deleted (§5);
    - every † reference is re-read, and all citations are re-pinned against `main` at implementation
      start.

Status: design only, nothing implemented. PR: https://github.com/helixml/helix/pull/3624.

**Pinned to `origin/main` at `2007b960c`** (round 9). Between `44a97b4f2` and `2007b960c` only
https://github.com/helixml/helix/pull/3625 touched cited files: `authz.go`,
`prompt_history_handlers.go` and `spec_task_design_review_handlers.go`. Those citations were
re-pinned, and every round-9 citation was taken at `2007b960c`. All other files cited by earlier
rounds are byte-identical, so their line numbers hold. Re-pin again before implementation starts.

Paths are relative to `api/pkg/server/` unless they say otherwise. A † marks a reference that came
from a delegated code search and was not re-read line by line.

## Round 10 changes (cyber delta review of `dff8cb86`)

The verdict was two blockers, with everything else signed off. Main is unchanged at `2007b960c`, so
the round-9 pins hold. Every round-10 citation was taken at `2007b960c`.

| finding | answer | section |
|---|---|---|
| **Blocker 1: NB5 partial.** Org delete, admin user delete, org-runtime teardown and project cleanup bypass the four guarded handlers. | The `sealedTaskLive` check moves into the choke points `DeleteRepository` (`services/git_repository_service.go:1084`), `purgeProject` (`project_handlers.go:1075`) and `Store.DeleteProject` (`store/store_projects.go:244`). Bulk callers pre-check before deleting anything. A no-bypass AST test is added. Attach is recorded; detach, move and set-primary return 409. | §3.12 |
| **Blocker 2: N9-1.** LLM call logs, and the app interactions and step-info routes, expose the lane through query filters. | DENY for sealed sessions on `/apps/{id}/llm-calls`, `/admin/llm_calls`, `/apps/{id}/interactions` and `/apps/{id}/step-info`. The default scope hides `llm_calls`. K's `/usage` strips the bodies. Route-policy rule 7 covers session / interaction / task query filters. | §3.14, §3.3 |
| **SF3 partial.** `UnifiedSearch`, `ListSessionsByOwner`, `ListPromptHistoryBySpecTask`, `ListSpecTaskGitPushEvents` | A GORM default scope: a create-only `sandbox_sealed` column on every lane-content table, and a query/row/count callback with explicit K and platform opt-ins. A raw-SQL allowlist test covers `Raw` / `Exec`. A callback rather than a view, with the reason given. | §3.10 |
| **NB2 gap.** The sealed repo row must have no upstream or deploy hook | `getRepoPath` returns a synthetic row: `ExternalURL = ""`, no deploy hook. So the sync (`:703`), the upstream push (`:867`) and `onDefaultBranchPush` (`:795-801`) are skipped by construction. | §3.11 |
| **S9-1** routing fails closed | 503 on error with no fallback; 403 for a lane key on the shared path; the path taken from `spec_task_sealed_repos.path` only; the K route checks key and row; pending → ready creation with a reconciler. | §3.11 |
| **S9-2** gc and backup | Refcount-derived `gc.pruneExpire` under the lock; the `MaintainRepo` helper plus an AST ban on direct prune/repack; relative alternates; filesystem-level backups including `sealed/`. | §3.11 |
| **S9-3** disk exhaustion | `receive.maxInputSize` and a per-task quota; archive as a `base..work` bundle with a TTL; a shared-repo growth alert. | §3.11 |
| **S9-4** shared-repo branch readers | The six sites are listed and skipped for sealed tasks, with a no-error-loop test. K's git reads are fetch-only; the read API returns not-found by design. | §3.13, §3.5 |
| **S9-5** network egress | The seal stops API and git exfiltration only; a prompt-injected lane can still use the network. Stated in "What the seal is not". | Goal |
| **Nit:** repo lock | Keyed by path; the base refresh takes shared then sealed. | §3.11 |
| **Nit:** sealed route | `/git/sealed/{T}/{R}` registered explicitly; repo id `sealed` reserved. | §3.11 |
| **Nit:** filestore | A traversal and symlink test. | §3.11, §7 |
| **Confirmations folded in** | NB4: `isPersonalAPIKey` (`handlers.go:1277`) already excludes the new type. NB1: closed by cyber's experiment. | §3.7, §3.11 |

## Round 9 changes (cyber delta review of `1fe263bf`)

The verdict was five new blockers. The structural answer to NB1 is a **per-sealed-task bare
repository with one-way alternates** (§3.11). It also closes NB2, SF1 and SF2. The interim minimum
stays in slice 1 as defence in depth.

| finding | answer | section |
|---|---|---|
| **NB1** protocol v2 serves sealed commits by SHA; the shared object store lets anyone ref a sealed SHA | The sealed lane's objects never enter the shared repo. Each sealed task gets its own bare repo per repo, with `objects/info/alternates` pointing **one way** at the shared repo. `getRepoPath` routes SELF to it, and K to a separate fetch URL. **Interim, kept in slice 1:** force protocol v0 for non-K/non-SELF fetches on repos with a sealed dependent, refuse unreachable SHAs, and a v2-by-SHA test. | §3.11, §3.10, §7 |
| **NB2** user and platform push/pull paths bypass `sealOfRef`; `ExternalURL` can exfiltrate | Every `/push`, `/pull`, `/push-pull` call and every platform pusher addresses the shared repo by id, so it cannot see sealed refs, and a sealed repo has no upstream. Defence in depth: guards in `PushBranchToRemote`, `PushToOrigin`, `PushToHelixBare`, `PullFromRemote`, `SyncAllBranches`, `CreatePullRequest` and merge; negative fetch refspecs; an env-independent hook. | §3.11, §3.10 |
| **NB3** the lane opens PRs itself through auto-approved proposals | Sealed create forces `AutoApprovePullRequests = false`. `auto_approve_future` is refused. `propose_pull_request` and `pr-proposals*` are DENY for sealed tasks. `pr_auto_approve` is on the gate-3 deny list. The lane's git key can only reach its sealed repos. | §3.1, §3.10, §1e, §3.3 |
| **NB4** Kodit clones with the creator's personal key, which can be K | K is a dedicated `APIkeytypeSealing` key, excluded from `isPersonalAPIKey`, from `getOrCreateUserAPIKey` and from the no-`types` `GET /api_keys` branch. Kodit moves to a service credential. Sealed repos are never registered with Kodit, and the Kodit read routes are listed. | §3.7, §3.10 |
| **NB5** project and repo lifecycle acts on sealed lanes with no record | `409 sealed_task_live` on project delete, repo delete, detach and move. If org-owner stop is adopted, it is recorded as `org_owner_stop`. The tombstone is written before any sealed-task deletion. | §3.12 |
| **SF1** one reserved prefix | `refs/heads/helix-sealed/<task>/…` for the feature branch and the specs. It is refused in the shared repo for everyone, regardless of env; negative refspecs; `helix-specs-sealed` refused. | §3.10, §3.11 |
| **SF2** `GitRepository.Branches` leaks sealed names | Structurally gone: the sealed repo has no `GitRepository` row. A defensive prefix filter is added as well. | §3.11, §3.10 |
| **SF3** the listing filter is opt-in per handler | Sealed rows are excluded by default in the store; `IncludeSealedForKeyID` / `withPlatformCaller` opt in. This covers all 33 call sites, including tasks-progress and tasks-usage. | §3.10 |
| **SF4** check before the upstream sync | Receive-pack commands are parsed in Go and checked before any `SyncAllBranches`; the hook is the second layer. This also makes the S3 dry run side-effect-free. | §3.11 |
| **SF5** version during the run | `cluster_min_seal_version`, plus `min_cluster_version_since_seal` and `cluster_gap_seen_at` on the seal row; a deploy-tool rollback guard; the mixed-old-replica limitation stated. | §3.8 |
| **SF6** three guideline composers | One `guidelinesFor(task)`, with an AST test that guidelines are read nowhere else. | §3.4 |
| **SF7** ref/SHA parameters | An AST assertion: any `ref` / `branch` / `sha` / `commit` parameter reaching `GetGitRepository` must go through `sealOfRef`. | §3.3 |
| **Prior: B1 count** | 13 per-task tools, plus `List` and `Create`. | §3.9 |
| **Prior: S1 hook layer** | Env-independent hook rules, tested with an empty env. | §3.3, §3.11 |
| **Prior: S3 dry run** | Fixed by SF4. | §3.11 |
| **Prior: S4 no-types branch** | The sealing key type is excluded. | §3.7 |
| **Prior: S6 precedence** | Broken-seal recording wins over the unsealed non-read rule. | §4 |
| **Prior: S8 other composers** | SF6. | §3.4 |
| **Nit: clone preconditions** | Every create precondition applies to a sealing clone. | §3.7 |
| **Nit: re-pin** | `2007b960c` (see above). | header |

## Round 8 changes (cyber review of #3624)

The verdict was two blockers, with the overall shape sound. This table maps each finding to the
section that answers it.

| finding | answer | section |
|---|---|---|
| **B1** The org-runtime spec-task tools (`helix-tasks` MCP, helix-org bots) bypass all three gates | Seal check inside `SpecTasks.ownedTask` (`org/infrastructure/runtime/helix/spectasks.go:188`), which all 13 per-task tools call (plus `List` and `Create`, 15 in all). Sealed tasks are filtered out of `List` (`:314`). Each refusal is recorded with route `mcp:<server>:<tool>`, listed in seal-policy, and tested per tool. | §3.9, §1e, §7 |
| **B2** Lane state addressed by repo, PR or artifact is ungated | A fourth gate: `sealOfRef(repo, ref)` and `sealOfArtifact(artifact)`, called from the git pre-receive hook, upload-pack and info-refs, `/contents`, PR merge and PATCH on both APIs, the git read API, and artifact reads and writes. Git write protection moves to slice 1. | §3.10, §5 |
| **B2 portal decision: option (a)** | Only K and the lane's own agent may read a sealed lane. Sealed tasks and sessions are filtered from listings for everyone but K. Sealed refs are hidden from upload-pack. The lane's specs go to a per-task ref; upstream mirroring and PRs are withheld. K gets an explicit read set. | §3.5, §3.10 |
| **S1** "No route in ctx" fails open | Platform callers are marked explicitly with `withPlatformCaller(ctx)`. A request with `user != nil` and no route key is denied (`reason: no_route`). | §3.3 step 3 |
| **S2** The route-policy test doesn't enforce the claim | Every `noAuthzHelper` and `handlerAuth` route must declare `.seal(...)`. Route keys include `{server}` and the tool name. Inline-auth handlers need explicit seal calls. The AST walk extends to the org runtime and `services.GitHTTPServer`. | §3.3, §7 |
| **S3** The probe can't run as specified | seal-policy publishes `id_in` per route. Dry-run mode `X-Helix-Seal-Probe: 1` returns `probe_ok` after the authz and seal checks without running the handler; it is honoured only for K on tasks K created (`CreatedByKeyID`). `dataCallBeforeHelper` is extended so the seal check comes before any side effect. | §3.7, §7 |
| **S4** K is not separable from K's user | A sealing key becomes non-listable (prefix only, for every caller). Access records carry a request fingerprint. Revoking a live sealing key is refused, with a recovery path for `SealedByUserID`. Keys minted by K are not K. | §3.7 |
| **S5** An unsupported build looks like the other states | `GET /api/v1/seal-policy` returns `seal_version`. The create response echoes the seal. Sealed creates are refused until every replica reports seal support. The note documents the exact field values for each state. | §3.8 |
| **S6** The break-glass trail can be erased | The seal header and the access events survive task deletion (a tombstone K can read). Deletion is itself an event. Every access after a break is recorded with `reason: seal_broken`. | §4 |
| **S7** Races | One insert creates the task, seal row and `SealedConfig`. The session's task link and its confined key exist from the start. A null `SealedConfig` fails closed. Re-minted keys keep the sealed type. Prompt delivery re-checks gate 3. | §3.1, §3.4, §3.3 gate 3 |
| **S8** System turns embed live guidelines | Guidelines are frozen into `SealedConfig` and turns are composed from that copy. `ci_status` is status-only for sealed lanes. | §3.4, §3.3 gate 3 |
| **N1** Clone contradicts default-deny | The sentence is dropped. Cloning a sealed task is denied; the portal creates a fresh sealed task instead. | §3.1 |
| **N2** The share-* vhost bypasses the helpers | Seal check in `dispatchSandboxPreview` (`vhost_middleware.go:195`). | §3.3 gate 2 |
| **N3** Hand-written allow-list keys | The keys are generated from the router; the code refers to them by handler, not by template string. | §3.3 |
| **N4** Re-pin | Done at `44a97b4f2`; superseded by the round-9 re-pin to `2007b960c`. | header |
| **N5** Dedupe | Denied rows and K's rows are never deduplicated. | §4 |
| **N6** Broken-flag caching | The broken flag is read uncached, and a failed read denies on every replica. | §3.3 step 2, §4 |
| **Cyber requirement:** owner-stop and break-glass records | If the owner adopts either, they are recorded as `outcome: allowed` with `actor_class` `org_owner_stop` / `admin_break_glass`, which is distinct from `sealing_key`. Break-glass sets `seal_broken_at`. | §4 |

Round-7 history: default-deny, the central guard, §1f, the `outcome` field, K, and task-level scope.
The break-glass option section (§4) is unchanged apart from the cyber recording requirement. The
owner decision on break-glass and org-owner stop is still open.

## Goal

PTaaS verifier and finder lanes need a sandbox that nobody can act on out of band. A sealed task
is **default-deny**: every route that resolves its id, its sessions' ids or its sandbox row refuses
every caller, except an explicit allow-list.

| allowed caller | allowed routes |
|---|---|
| **The sealing key K** (§3.7): the credential that created the task | stop, archive (the delete precondition), delete; reading the seal state and the access record; **reading the lane** under the portal's option (a): the explicit read set in §3.5 |
| **The sandbox's own identity**: its session key, and a per-runner token bound to its host | its agent channel (sync, RevDial, config, credentials, daemon callbacks) and its own drive operations |
| **Platform internals**: in-process callers explicitly marked with `withPlatformCaller(ctx)` (orchestrator, pollers, hydra, reapers, credit exhaustion; §3.3) | everything they do today, except the gate-3 and gate-4 limits on what reaches the lane |

**Nobody else may read the lane either** (option (a)): not the finder lane, the org owner or an
admin acting through the API. That covers its interactions, branch, specs, artifacts and listing
entries.

Everyone else is refused on every such route. That includes org owners, project members, platform
admins acting through the API, and K on any route not in its allow-list. A route added later is
refused until someone classifies it (§3.3).

**What the seal is not.** It does not protect against the platform operator. Anyone with database,
host, hydra or `docker exec` access on the sandbox host can always reach the container. The seal
protects against tenant-side actors and against admins acting through the Helix API. The portal and
the verifier must not claim more than that.

**Nor does it stop the lane itself from leaking (S9-5).** The lane reads tenant-controlled base
content: the base branch, the project repos, fetched dependencies. All of that is a prompt-injection
surface, and the lane's container has outbound network access. The seal stops reads and writes
**through the Helix API and git server**. It does not stop a prompt-injected lane from sending its
own work to an arbitrary host over the network. Egress control (an allowlist proxy for the sandbox)
is a separate control, and the portal must not describe the seal as exfiltration-proof without it.

---

## 1. Inventory

### Who passes `authorizeUserToSession` today

`authorizeUserToSession` is at `authz.go:589`. "S(U) set" below means every caller it admits:

1. **The session owner, with any of their credentials.** That includes a browser session, their
   personal key, a key an admin mints for them (`/admin/users/{id}/api-keys`†), and the session's
   own key. The session key is minted with `Owner = session.Owner`
   (`services/spec_driven_task_service.go:2076`), so it authenticates *as* the creator.
2. **Any org owner** (`authz.go:623`).
3. **Any platform admin.** `AuthorizeOrgMember` returns a synthetic owner membership for
   `user.Admin` (`store/orgstore/authz.go:101-106`).
4. **Project members with the requested action.** When the project has `OrgMembersAccess`,
   `ActionUpdate` is downgraded to project `ActionGet` (`authz.go:642-644`), so **every org member**
   passes.
5. **The unconfined session key of any sandbox in the same project.** `enforceKeySessionScope`
   (`authz.go:204-231`) admits it, so sibling task sandboxes pass. A task-confined key reaches only
   its own task (#3611).

S(G) is the same set with `ActionGet`.

### Seal class (last column of every table)

Default-deny applies: anything not marked `SELF` or `K: …` is refused to every non-platform caller
of a sealed task.

| class | meaning on a sealed task |
|---|---|
| **SELF** | Only the sandbox's own identity may call it (its session key, or a per-runner token bound to its host). Everyone else, K included, is refused. |
| **K: stop / archive / delete / read seal** | Allowed to the sealing key K, and to SELF. Everyone else is refused. |
| **DENY** | Refused to everyone except platform internals. "(candidate: …)" records what an owner could later choose to add to the allow-list (a read view, a provenance-recorded turn). Nothing is added by default. |
| **freeze** | Not a route on the sealed task: a project- or app-level edit whose effect is held back from the sealed sandbox (§3.4). |
| **n/a** | Cannot reach the sandbox. |

Every refusal and every allowed call on a sealed task writes an access record (§4).

### 1a. Desktop, container and agent channel (`/external-agents/*`, `/sessions/*`)

| route / path | handler file:line | what it can do to the sandbox or agent | authz today | who passes today | creates interaction / provenance? | seal |
|---|---|---|---|---|---|---|
| `POST /external-agents/{sid}/command` | `external_agent_handlers.go:218` | Sends any `ExternalAgentCommand` verbatim to Zed: chat_message, open_thread, cancel, respond_question, …. A chat_message that reuses an in-flight `request_id` matches a pre-created interaction (`websocket_external_agent_sync.go:1640-1653`), so no new interaction is created. Can also auto-start the container (`websocket_external_agent_sync.go:2491`). | S(U) `:240` | S(U) set | **n** | SELF (better: delete the route, see §5) |
| `POST /external-agents/{sid}/exec` | `external_agent_handlers.go:403` | Arbitrary command in the desktop through RevDial `/exec`. | S(U) `:427` | S(U) set | n | SELF |
| `POST /external-agents/{sid}/upload` | `external_agent_handlers.go:836` | Writes files into the container. | S(U) `:855` | S(U) set | n | SELF |
| `GET /external-agents/{sid}/file` | `external_agent_handlers.go:1014` | Reads uploaded chat attachments. | S(G) `:1027` | S(G) set | n | **K: read** (option a) |
| `GET` / `POST /external-agents/{sid}/clipboard` | `external_agent_handlers.go:603` / `:714` | Reads / writes the desktop clipboard. Pasting from it is input. | S(G) `:622` / S(U) `:733` | S(G) / S(U) set | n | GET DENY; POST SELF |
| `GET /external-agents/{sid}/ws/input` | `external_agent_handlers.go:1153` | Keyboard and mouse input. Typing into Zed's agent panel creates a turn stamped `external_agent`, with no actor (`websocket_external_agent_sync.go:1101`, `:1675`). | `authorizeDesktopID(Update)` `:46` | S(U) set | turn y, actor **n** | SELF |
| `GET /external-agents/{sid}/ws/stream` | `external_agent_handlers.go:1332` | Video stream, **and input**. Input is dropped only when `X-Helix-Readonly` is set, i.e. for a non-interactive embed key (`desktop_stream_readonly.go:45`, `api/pkg/desktop/ws_stream.go:1893`, `:2128`). | `authorizeDesktopID(Get)` | S(G) set, all of whom can **drive** | n | **K: read**, stream forced read-only (§6 bug 1) |
| `GET /external-agents/{sid}/screenshot` | `external_agent_handlers.go:268` | Screenshot. | `authorizeDesktopID(Get)` | S(G) set | n | **K: read** (option a) |
| `POST /external-agents/{sid}/configure-pending-session` | `external_agent_handlers.go:1109` | No-op. | user only | anyone | n | n/a (delete it) |
| `GET /external-agents/{sid}/workspace-review`, `/workspace-review/turn/{iid}`, `/workspace-files`, `/workspace-file`, `/workspace-file/download`, `/workspace-skills`, `/workspaces` | `workspace_review_handlers.go:65`, `:253`, `:93`, `:112`, `:130`, `:219`; `external_agent_handlers.go:1600` | Reads the workspace through the desktop's HTTP API. | S(G) (`authorizeWorkspaceReviewRequest` `:17`) | S(G) set | n | **K: read** (option a) |
| `PUT /external-agents/{sid}/workspace-file` | `workspace_review_handlers.go:186` | Writes a file into the agent's working tree. | S(U) | S(U) set | n | SELF |
| `GET /sessions/{id}/terminal` (WebSocket) | `session_terminal_handlers.go:28` | Interactive tmux shell in the container, through hydra RevDial. | S(U) `:88` | S(U) set | n | SELF |
| `GET /sessions/{id}/terminal/sessions`, `DELETE …/{ts}` | `session_terminal_handlers.go:46`, `:64` | Lists / kills tmux sessions. | S(G) / S(U) | S(G) / S(U) set | n | GET **K: read**; DELETE SELF |
| `GET /mcp/desktop?session_id=` | `mcp_backend_desktop.go:57-90`†, registered `server.go:704` | Desktop MCP: `type_text`, `mouse_click`, `set_clipboard`, window control, and `save_screenshot` to **any path** (`api/pkg/desktop/mcp_server.go:66`). | S(U)† | S(U) set | n | SELF |
| `GET /sessions/{id}/zed-config` | `zed_config_handlers.go:37` | Returns the full agent config. **It embeds the session API key** (`:139`, used in MCP headers `:164`). | `authorizeSessionSandboxConfig` (`session_credentials_authz.go:54`): bound runner, or `session.Owner == user.ID` plus S(G) | per-runner token for the host; the **owner with any credential** | n | SELF (closes the key leak) |
| `POST /sessions/{id}/zed-config/user` | `zed_config_handlers.go:426` | Saves overrides. Inert inside the sandbox (§2). | same | same | n | SELF |
| `GET /sessions/{id}/zed-settings` | `zed_config_handlers.go:484` | Merged view. Also builds config with the session key (`:517`). | same, runner refused | owner | n | SELF |
| `GET` / `PUT /sessions/{id}/claude-credentials`, `/codex-credentials` | `claude_subscription_handlers.go:768`, `:851`; `codex_subscription_handlers.go:307`†, `:346` | Read / replace the decrypted agent subscription. A PUT reaches the sandbox on the next 30s poll. | inline: runner bound to the host, the owner in key scope, or the session key; then `canUseSessionAgentCredential` | owner (only for their own subscription), session key, bound runner | n | SELF |
| `GET /external-agents/sync` (agent WebSocket) | `external_agent_thread_routes.go:97` | **The agent channel itself.** `registerConnection` is last-writer-wins (`websocket_external_agent_sync.go:2703`), so a caller displaces the real Zed, receives every command, and can inject agent events. `validateExternalAgentToken` is a stub returning true (`:4981`). | runner token (shared **or** per-host, **any** session), `user.Admin`, or `ownsSessionInKeyScope` | runner-token holders, **all admins**, the **owner with any credential**, same-owner sibling project keys | n | SELF |
| `GET /revdial` (`desktop-{sid}`) | `server.go:2767`; check at `revdial_registration.go:32` | Registers the tunnel the API uses for *every* desktop operation. `connman.Set` is last-wins (`server.go:~2958`), so a caller intercepts exec, upload, file writes, input, stream and MCP traffic. | `revDialKeyMayRegisterDesktop` + `session.Owner == user.ID` | the **owner's personal key**, project keys of the owner | n | SELF (session key only) |
| `GET /ws/user?session_id=` | `websocket_server_user.go:91`† | Outbound events only; incoming frames are discarded (`:203-212`†). | S(G) | S(G) set | n | SELF (the daemon's event feed); others DENY |
| `GET /sessions/{id}/rdp-connection`, `/sandbox-state`, `/workspace-status`, `/step-info`, `/output`, `/usage`, `/toc`, `/search`, `/turns/{n}`, `/interactions*`, `GET /sessions/{id}` | `session_handlers.go:2281`, …; `agent_sandboxes_handlers.go:446` | Read-only. | S(G) | S(G) set | n | **K: read** (option a) |
| `GET /sessions/{id}/operator-access` | `session_operator_access.go:10` | Probe only (returns 204). | S(U) + project U | — | n | DENY |

### 1b. Session lifecycle and the turn path

| route | handler | effect | authz | who passes | provenance? | seal |
|---|---|---|---|---|---|---|
| `POST /sessions/{id}/messages` | `session_handlers.go:2717` | Turn through the prompt queue (`enqueueAgentMessage`). | S(U) | S(U) set | **y** (`prompt_history_handlers.go:603`) | DENY (candidate: provenance-recorded turn; owner decision) |
| `POST /sessions/chat` (existing session) | `session_handlers.go:644` (S(U) at `:894`) | Turn through `RunExternalAgent`, **bypassing the queue**. | S(U) | S(U) set | y (`:1416`, `:1470`†) | DENY (candidate: provenance-recorded turn; owner decision) |
| `POST /prompt-history/sync` | `prompt_history_handlers.go:71` | Durable prompt queue. Rows are delivered and the desktop is woken. | `authorizeUserToPromptQueue` (`:429`), now `ActionUpdate` whenever entries are queued (`:92-96`). The `ActionGet` gap was fixed in https://github.com/helixml/helix/pull/3625. | S(U) set | y | DENY (candidate: provenance-recorded turn; owner decision) |
| `POST /interactions/{iid}/questions/{rid}/respond` | `interaction_question_handlers.go:31` | Sends **caller-chosen answers** to the agent. | S(U) `:127` | S(U) set | **n** (`ResolvedQuestion` has no actor, `types/agent_question.go:29`) | DENY |
| `POST /interactions/{iid}/questions/{rid}/cancel` | `interaction_question_handlers.go:76` | Fixed-form `cancel_question`; no caller content. | S(U) | S(U) set | n | DENY (candidate once the actor is recorded, §3.6) |
| `POST /sessions/{id}/cancel` | `session_handlers.go:2600` → `prompt_history_handlers.go:716` | Fixed-form cancel of the current turn. | S(U) | S(U) set | n | DENY (candidate once the actor is recorded, §3.6) |
| `POST /sessions/{id}/clear` | `session_clear.go:185` | Cancels and closes the Zed thread, losing agent context. | S(U) | S(U) set | n | DENY |
| `POST /sessions/{id}/restart-agent` | `session_handlers.go:2873` | Restarts the container; may reset the thread. | S(U) | S(U) set | n | DENY |
| `POST /sessions/{id}/resume`, `/ensure-agent` | `session_handlers.go:2363`, `:3619` | Starts the container. | S(U) | S(U) set | n | DENY (platform internals may still start it) |
| `DELETE /sessions/{id}/stop-external-agent` | `session_handlers.go:2636` | `StopDesktop`. | S(U) | S(U) set | n | **K: stop** |
| `DELETE /sessions/{id}` | `session_handlers.go:448` | Destroys the desktop. | S(Delete) | — | n | **K: delete** |
| `PATCH /sessions/{id}/archive` | `session_handlers.go:567`† | `StopDesktop`. | S(U) | — | n | DENY |
| `PUT /sessions/{id}` | `session_handlers.go:502` | DB only: name, provider, model. Not pushed live†. | S(U) | — | n | DENY |
| `POST /sessions/{id}/foreground-thread` | `session_handlers.go:2796` | `open_thread` command; switches the agent's thread. | S(U) | — | n | DENY |
| `POST /sessions/{id}/fork` | `session_fork_handlers.go:61` | Runs commit and push **inside the parent sandbox**, stops it, repoints the task. | S(U) | — | y (`:397`, `:439`†) | DENY |
| `POST /sessions/{id}/switch-agent` | `session_switch_agent_handlers.go:62` | Hot-switches the live agent. | S(U) | — | y | DENY |
| `PATCH /sessions/{id}/execution-config` | `session_execution_config_handlers.go:118` (S(U) `:140`; spec-task branch `applySpecTaskExecutionConfig` `:175`) | Hot-switches runtime, model, provider or credential, or restarts the agent. | S(U) | S(U) set | y (`execution_config.go:328`†) | **DENY** (§3.4) |
| `POST /sessions/{id}/agent-config-applied`, `/agent-startup-error` | `session_switch_agent_handlers.go:833`, `session_agent_startup_error_handlers.go:40` | Daemon callbacks. The first delivers the waiting turn early; the second **marks a turn as errored**. | S(U) | S(U) set | n | SELF |
| `GET` / `POST` / rotate / `DELETE /sessions/{id}/preview-tokens` | `session_preview_handlers.go:61` | Mints a **public, unauthenticated** `share-*` host for **any port 1-65535** in the container (`vhost_middleware.go:122`, `hydra/server.go:754`†). The caller can then drive the agent's dev servers. | S(U) | S(U) set | n | DENY (all four verbs; §1f) |
| `POST /api_keys?type=embed&interactive=true` | `handlers.go:~965` | Embed key that can drive the session's desktop (`auth_embed_key.go:237`). | project/task authz | project members | n | DENY for sealed tasks (any embed key, interactive or not) |
| `GET /api_keys?types=all` | `handlers.go:1296` | Lists the caller's keys **in plaintext**, including session keys, which are owned by the creator. Redaction only applies to scoped callers (`:1247`). | user | the creator | n | exclude or redact sealed session keys for every caller |

### 1c. Org Sandboxes API on session-backed rows (cyber R013 point 1)

Spec-task desktops, exploratory sessions and subscription desktops also have a `sandboxes` row
(`sbx_…` id) with `SessionID` set (`types/sandbox.go:92-107`). `HydraOpsID()` returns the
**session id** for those rows (`types/sandbox.go:147-152`), so every handler below operates on the
spec-task desktop. `loadAuthorizedSandbox` (`sandboxes_api_handlers.go:1004`) checks **org
membership only**. A fix for that is in progress in a separate PR. The seal must refuse this route
family regardless, because org owners and admins pass any membership check.

| route | handler | effect on a session-backed row | authz | who passes | provenance? | seal |
|---|---|---|---|---|---|---|
| `POST …/sandboxes/{id}/commands` | `sandboxes_api_handlers.go:226` (`:262`) | Arbitrary exec in the spec-task desktop. | org member | **every org member**, admins | n | DENY (the sandbox's own key has no use for it) |
| `GET …/commands`, `…/{cmd}`, `…/{cmd}/logs` | `:290`, `:320`, `:354` | Read command output. | org member | org | n | **K: read** (option a) |
| `POST …/commands/{cmd}/kill` | `:391` (`:402`) | Signals processes in the desktop. | org member | org | n | DENY |
| `GET` / `PUT` / `DELETE …/files`, `GET …/files/list` | `:422` (`:440`, `:463`, `:470`), `:501` | Read / **write** / **delete** files. | org member | org | n | DENY (all verbs) |
| `GET …/terminal`, `…/terminal/sessions` | `:521` (`:531`), `:880` | Interactive shell. | org member | org | n | DENY |
| `GET …/screenshot` | `:683` (`desktop-{HydraOpsID}` `:694`) | Screenshot. | org member | org | n | **K: read** (option a) |
| `PATCH …/sandboxes/{id}` | `:175` | Changes the row (TTL, …). | org member | org | n | DENY (§1f) |
| `DELETE …/sandboxes/{id}` | `:202` → `sandbox/controller.go:293` (`stopSessionDesktop`) | Stops the spec-task desktop. | org member | org | n | DENY (K stops and deletes through the task and session routes; §1f) |

### 1d. Spec-task routes

| route | handler | effect | authz | provenance? | seal |
|---|---|---|---|---|---|
| `POST /spec-tasks/from-prompt`, `/clone`, the helix-org `create_spectask` tool, the PM skill | `spec_driven_task_handlers.go`, `spec_task_clone_handlers.go:33` | Creates the task. | project/task create; `denyTaskCreationToConfinedKey` | y | only from-prompt (and `/clone` of an unsealed source) can set the seal, with a dedicated `sealing` API key that becomes K (§3.7); `create_spectask` and the PM skill cannot. `/clone` of a **sealed** source is DENY (N1, §3.1). |
| `PUT /spec-tasks/{id}` | `spec_driven_task_handlers.go:1384`† | `status` drives the orchestrator (approve, start implementation), and the resulting prompt is attributed to the old `QueuedBy`†. `backlog` and `keep_alive=false` stop the desktop. **`agent_tools` hot-applies** through the MCP rev (`external-agent/zed_config.go:405-411`†). | T(U) | status path **n** (misattributed) | DENY (every field; `sandbox_sealed` immutable) |
| `PATCH /spec-tasks/{id}/execution-config`, `/interactive-embed-model` | `spec_task_execution_config_handlers.go:73`†, `spec_task_interactive_embed_model_handlers.go:54`† | Hot-switches agent, model or provider; live resource change. | T(U) | y | **DENY** (§3.4) |
| `POST /start-planning`, `/approve-specs`, `/approve-implementation`, `/pr-proposals/{p}/decide`, `/completion/decide` | `spec_driven_task_handlers.go:1209`†, `:699`†; `spec_task_workflow_handlers.go:30`†; `spec_task_pr_proposal_handlers.go:56`†, `:154`† | Workflow decisions that send templated system turns. | T(U) / project(U); agent keys refused except on approve-specs (gap noted in §6) | y | DENY (a sealed task must need no human decisions, §3.1) |
| design reviews: `/submit`, `/comments` POST, `/comments/{c}/resolve` | `spec_task_design_review_handlers.go:490`, `:696`, `:866` | `revision_requested` / `design_review_comment` turns, which interrupt the agent. | T(U) at `:517`, `:718`, `:883`. The creator shortcut was removed in https://github.com/helixml/helix/pull/3625. | y (commenter provenance `:728`, `:742`) | DENY |
| design review `/document` PUT | `spec_task_design_review_handlers.go:310` (T(U) `:333`) | Commits to helix-specs, **which the agent reads**. No turn. | T(U) | n | DENY |
| `POST /spec-tasks/{id}/attachments` | `spec_task_attachments_handlers.go:324`† | Commits a file into the repo and sends an `attachment_added` turn. | T(U) | y | DENY after creation |
| `POST /refresh-pull-request` | `spec_task_pull_request_refresh_handler.go:25`† | Forces a PR poll, which can trigger `pr_settled` / `ci_status` turns. | **T(Get)** | partial | DENY (the PR and CI pollers still run as platform internals) |
| `POST /stop-agent`, `PATCH /archive`, `DELETE` | `spec_task_workflow_handlers.go:951`†; `spec_driven_task_handlers.go:1728`†, `:1648`† | Stop the desktop; delete. | T(U) | n | **K: stop** (`/stop-agent`), **K: archive and delete** (archive is the delete precondition†) |
| `DELETE /spec-tasks/{id}/zed-instance`, `POST /work-sessions/{s}/zed-thread`, `/zed/events`, `/zed/instances/{i}/threads/{t}/events`, `/zed/threads/{t}/activity` | `zed_event_handlers.go:314`, `:566`, `:106`, `:171`, `:435`† | Legacy protocol: fake agent state events, instance shutdown. | `authorizeUserToZedSpecTask(U)` | n | SELF; everyone else DENY (§1f) |

### 1e. Git, MCP, and live config

| path | where | effect | who passes | provenance? | seal |
|---|---|---|---|---|---|
| git push to the task branch | `services` `GitHTTPServer` `handleReceivePack` (`git_http_server.go:637`); post-push hook `:1260` | Silently changes the branch the agent works on and pulls. Branch limits apply only to agent keys. | anyone with repo write | n | SELF, in the lane's own sealed repo; the shared repo refuses `refs/heads/helix-sealed/*` to everyone (§3.11) |
| `PUT /git/repositories/{id}/contents` | `git_repository_handlers.go:1172`† | Commits through the API. | repo(U) | n | same branch rule |
| `POST /pulls/{n}/request-rebase` | `git_pull_request_handlers.go:443`† | `rebase_required` turn. Has **no task-scope check** (a confined key can target another lane). | repo(U) | y | DENY (refused at the turn sink, §3.3) |
| `POST /pulls/{n}/merge`, GitHub review webhook | `:404`†; `spec_task_github_review_webhook.go:178`† | Merge; `pr_settled` / `review_comment` turns. | repo(U) / HMAC | partial (no `triggered_by`) | merge of the sealed task's PR: DENY. Webhook `review_comment` turns are dropped at the turn sink and recorded as `denied`. Poller `pr_settled` / `ci_status` turns: platform |
| MCP `session` backend agent-only tools: `propose_pull_request`, `mark_task_complete`, `task_completed` | `mcp_backend_session.go:215`, `:339`†; auto-approved opening `:249-258` | Acts **as the agent**. With auto-approval, `propose_pull_request` pushes upstream and opens a PR (`services/spec_task_pr_proposals.go:552`, `:561`) from a head branch the agent picks. | `session.Owner == user.ID` plus key scope, so the creator's personal key passes | n | `mark_task_complete`, `task_completed`: SELF. `propose_pull_request`: **DENY** for sealed tasks, even for SELF (NB3, §3.10) |
| **Org-runtime spec-task tools (B1)**: the `helix-tasks` MCP and helix-org bot tools `Get`, `List`, `ListAgentMessages` (reads interactions), `Update`, `StartPlanning`, `SendAgentMessage`, `StartAgent`, `StopAgent`, `Complete`, `RestartAgent`, `ReviewSpec`, `ApproveSpec`, `RequestChanges`, `CreatePullRequests` | `org/infrastructure/runtime/helix/spectasks.go`: `ownedTask` `:188` checks only `task.ProjectID == projectID`; it is called at `:341`, `:353`, `:407`, `:431`, `:459`, `:508`, `:530`, `:545`, `:563`, `:588`, `:609`, `:643`, `:675`. `List` `:314` does not call it. Backend `mcp_backend_spectask.go:58` (refuses confined keys at `:69`); tool catalogue `org/interfaces/mcptools/defaults.go:180`. | Read, steer, stop, restart, complete and approve **another** task in the same project. The one HTTP route `/api/v1/mcp/{server}` (`server.go:1876`) has no object helper. | any **unconfined** session key in the project (sibling lanes), and org bots | `SendAgentMessage` only | **DENY** in `ownedTask`; `List` filters sealed tasks out (§3.9) |
| **Repo- and PR-addressed lane state (B2)**: `git-receive-pack` / `git-upload-pack` / info-refs; `PUT /git/repositories/{id}/contents`; the git read API (`/tree`, `/contents` GET, `/file-content`, `/files`, `/grep`, `/commits`, `/branches`); PR `PATCH`, `merge` | `services/git_http_server.go:637` (pre-receive env `HELIX_ALLOWED_BRANCHES` set around `:746-751`), `:531`, `:457`; `git_repository_handlers.go:1172`; `server.go:1817-1842`; `git_pull_request_handlers.go:365`, `:404` | Write or read the lane's branch and specs; merge or retarget its PR. | repo readers / writers | n | **sealOfRef** (§3.10): writes SELF-only; reads K and SELF only |
| **GitHub-compatible API (B2)** at `/api/github/api/v3` (`github_compat.go:36`) | `github_compat.go:42` (create PR), `:44` → `:376` (edit), `:47` → `:452` (merge), `:45` / `:46` (files, comments) | Same as above, through the go-github code path. | repo readers / writers | n | **sealOfRef** (§3.10) |
| **Artifacts carrying the lane (B2)**: `GET` / `PUT` / `DELETE /artifacts/{artifact_id}`, `/versions`, `/download`, `/embed*`, `/document`, the insecure `/public/artifacts/{id}` | `server.go:1728-1732`, `:1641-1649`; `artifact_handlers.go:185` (inline auth `:222-227`), `:205` (public viewer), `:281` / `:400` / `:437` via `requireArtifactAccess` `:450`, `:946`, `:1080`, `:1106`; lane binding `types/artifact.go` `SpecTaskID` / `SourceSpecTaskID` | Overwrite or delete the verifier's deliverables; read them early; publish them publicly. | project members (writes need project update); anyone if `visibility=public` | n | **sealOfArtifact** (§3.10) |
| **Listings (B2)**: `GET /spec-tasks`, `GET /sessions`, `/clone-groups/{groupId}/progress`, `/projects/{id}/artifacts`, org-runtime `List` | `spec_driven_task_handlers.go:441`; `session_handlers.go:192`; `spec_task_clone_handlers.go:403`; `artifact_handlers.go:62`; `spectasks.go:314` | Reveal that the lane exists, and its content. | project readers | n | filtered in the **store query** for everyone but K (§3.10) |
| helix-org bot tools that steer spec tasks | `mcp_backend_helix_org.go`† | Bot messages a task's agent. | org-bot session key | y (`org_bot_spec_task_message`) | DENY at the turn sink when the target is sealed |
| **Live config: project skills and MCPs** | `getZedConfig` `zed_config_handlers.go:123-130`, `:164`; project MCPs `external-agent/zed_config.go:389-393`, stdio `command` `:445-486`; daemon poll `cmd/settings-sync-daemon/main.go:35` | A project member adds a stdio MCP or changes an MCP URL; Zed runs it **inside the running sandbox** within about 30s. This is **remote code execution with no record.** | project(U) (`project_handlers.go:683`, `:845`†) | n | **freeze** (§3.4) |
| Live config: app/agent MCPs, provider endpoints, `agent_tools` | `getZedConfig` → `GenerateZedMCPConfig` | Changing the agent's MCPs or the provider base URL (lets the editor intercept LLM traffic) reaches the running sandbox. | app editors, org owners | n | **freeze** (§3.4) |
| Start-time inputs: project secrets (they can set `HELIX_SKILLS_REPO`, `_REF` and the like†), `.helix/startup.sh`, skills ref | `hydra_executor.go:925-936`†; `helix-workspace-setup.sh:301-320`† | Apply on the next start or resume. | project(U) | n | snapshot hashes at seal time; flag drift on every start (§3.4) |
| hydra registry and admin debug routes | `sandbox_handlers.go:27-35`†; `/admin/agent-sandboxes/*`, `/admin/runners/{id}/logs` | Read-only, or runner/admin only. No path into a desktop. | admin / runner | n | n/a |
| `/ws/external-agent-runner` | `server.go:~2167`† | Runner token only. | runner | n | n/a (platform) |
| Skills routes, `/skills/reload` | admin† | Start-time only. | admin | n | n/a |

There is no dedicated admin desktop viewer. Admins use the normal session page through the routes
above (`frontend/src/components/admin/Runners.tsx` only stops sessions†), so sealing those routes
covers admins too.

### 1f. Routes missed at `66c7cdd` (round-7 list), and how the guard reaches each

Each row gives the route registration (`server.go`), the handler, and the line where today's authz
helper resolves the sealed object. The last two columns say how the guard reaches the route. "Path
guard" is the router middleware (§3.3, gate 1). "Resolver gate" is the seal check inside the shared
authz helper (§3.3, gate 2). Because the resolver gate sits in the helper, a handler that resolves
the id from its body is still covered.

| route | registered | handler | object resolved at | id comes from | covered by | seal |
|---|---|---|---|---|---|---|
| `PATCH /spec-tasks/{taskId}/execution-config` | `server.go:1756` | `spec_task_execution_config_handlers.go:73` | `authorizeUserToSpecTask` `:111` | path `taskId` | path guard + resolver gate | DENY |
| `POST /sessions/{id}/cancel` | `server.go:1295` | `session_handlers.go:2600` | `authorizeUserToSession` `:2611` | path `id` | path guard + resolver gate | DENY (candidate once the actor is recorded, §3.6) |
| `POST /sessions/{id}/foreground-thread` | `server.go:1297` | `session_handlers.go:2796` | `authorizeUserToSession` `:2813` | path `id` | path guard + resolver gate | DENY |
| `PATCH /organizations/{org_id}/sandboxes/{id}` | `server.go:1498` | `sandboxes_api_handlers.go:175` | `loadAuthorizedSandbox` `:176` (`:1004`) | path `id` → row `SessionID` / `SpecTaskID` (`types/sandbox.go:102`, `:107`) | path guard + resolver gate (in `loadAuthorizedSandbox`) | DENY |
| `DELETE /organizations/{org_id}/sandboxes/{id}` | `server.go:1499` | `sandboxes_api_handlers.go:202` | `loadAuthorizedSandbox` `:203` | same | same | DENY (K stops and deletes through the task and session routes) |
| `POST /organizations/{org_id}/sandboxes/{id}/commands/{cmd_id}/kill` | `server.go:1504` | `sandboxes_api_handlers.go:391` | `loadAuthorizedSandbox` `:392` | same | same | DENY |
| `POST /zed/events` | `server.go:1797` | `zed_event_handlers.go:106` | `authorizeZedSpecTask` `:129` → `authorizeUserToZedSpecTask` `:76` | **body** `spec_task_id` (decoded `:110`, checked `:125`) | resolver gate only (no id in the path) | SELF; everyone else DENY |
| `POST /zed/instances/{instanceId}/threads/{threadId}/events` | `server.go:1798` | `zed_event_handlers.go:171` | `authorizeZedSpecTask` `:216` | **body** `spec_task_id`, else the thread's record (`:208-214`) | resolver gate only | SELF; everyone else DENY |
| `POST /zed/instances/{instanceId}/heartbeat` (same family) | `server.go:1799` | `zed_event_handlers.go:501` | `authorizeZedSpecTask` `:527` | body | resolver gate only | SELF; everyone else DENY |
| `POST /zed/threads/{threadId}/activity` (same family) | `server.go:1800` | `zed_event_handlers.go:435` | `authorizeZedSpecTask` `:458` | the thread's record | resolver gate only | SELF; everyone else DENY |
| `GET /spec-tasks/{taskId}/zed-instance`, `GET …/zed-threads` (same family) | `server.go:1801`, `:1803` | `zed_event_handlers.go:264`, `:371` | `authorizeUserToZedSpecTask` `:286`; `authorizeUserToSpecTask` `:393` | path `taskId` | path guard + resolver gate | DENY |
| `POST /work-sessions/{sessionId}/zed-thread` | `server.go:1804` | `zed_event_handlers.go:566` | `GetSpecTaskWorkSession` `:588` → `GetSpecTask` `:595` → `authorizeUserToZedSpecTask` `:602` | path `sessionId` is a **work-session** id, not a session id | path guard (resolves through the work session) + resolver gate | DENY |
| `DELETE /spec-tasks/{taskId}/zed-instance` | `server.go:1802` | `zed_event_handlers.go:314` | `authorizeUserToZedSpecTask` `:336` | path `taskId` | path guard + resolver gate | DENY |
| `GET /sessions/{id}/preview-tokens` | `server.go:1305` | `session_preview_handlers.go:30` | `authorizeUserToSession` `:37` | path `id` | path guard + resolver gate | DENY |
| `POST /sessions/{id}/preview-tokens` | `server.go:1306` | `session_preview_handlers.go:61` | `authorizeUserToSession` `:68` | path `id` | same | DENY |
| `POST /sessions/{id}/preview-tokens/{token_id}/rotate` | `server.go:1307` | `session_preview_handlers.go:114` | `authorizeUserToSession` `:123` | path `id` | same | DENY |
| `DELETE /sessions/{id}/preview-tokens/{token_id}` | `server.go:1308` | `session_preview_handlers.go:186` | `authorizeUserToSession` `:195` | path `id` | same | DENY |

`authorizeUserToZedSpecTask` has a non-project branch (`zed_event_handlers.go:80-89`) that does not
call `authorizeUserToSpecTask`. The resolver gate must therefore go into `authorizeUserToZedSpecTask`
itself, not only into `authorizeUserToSpecTask`.

---

## 2. How `/sessions/{id}/zed-config/user` overrides are applied

Short answer: **they aren't applied to a running sandbox.** The route is a write-only store. The
sandbox's own settings-sync daemon uses it to report local `settings.json` edits.

1. **Authorization.** `authorizeSessionSandboxConfig` (`session_credentials_authz.go:54-71`) admits:
   - a per-runner token bound to the session's host (`canRunnerReadSession`,
     `runner_identity.go:177`);
   - the session owner with any credential, followed by S(G).

   Bot-instance keys are allowlisted for it (`auth_bot_instance_key.go:35-38`).
2. **Storage.** The body is any JSON object, unvalidated (`zed_config_handlers.go:449-457`). It is
   upserted into `zed_settings_overrides(session_id, overrides jsonb)` via
   `external_agent.SaveUserZedOverrides` (`api/pkg/external-agent/zed_config.go:974`).
3. **Readers.** The only reader is `GetUserZedOverrides` (`external-agent/zed_config.go:989`), and
   its only caller is `getMergedZedSettings` (`zed_config_handlers.go:555`). That handler serves
   `MergeContextServers(helix, user)` to the UI's "MCP Tools" panel
   (`frontend/src/components/session/ZedSettingsViewer.tsx`†). **`getZedConfig` never reads the
   overrides.**
4. **The daemon** (`api/cmd/settings-sync-daemon/main.go`):
   - It authenticates with `USER_API_TOKEN`, the session key.
   - It fetches `GET /zed-config` at start (with retries), polls it every 30s (`:35`), and reacts to
     `config_changed` on `/ws/user`. `field=agent_restart` runs `pkill -x zed` (`:1721`).
   - It writes `/home/retro/.config/zed/settings.json` (`:28`), the qwen settings (`:31`),
     `/etc/claude-code/managed-settings.json` (`:945`), and the codex, goose and claude credential
     and config files.
   - It watches `settings.json` with fsnotify and POSTs the user-edited part back to
     `/zed-config/user` (`syncToHelix`, `:~2342-2372`). **That push-back is the route's main
     caller.**
5. **Merge rules inside the sandbox** (`mergeSettings`, `:1970`):
   - Helix-owned keys win: `agent_servers`, `telemetry`, `default_agent` and `external_sync`
     (`:1845`). Inside `agent`, the default and side models and `sandbox_permissions` are also
     Helix-owned. So are the context servers `chrome-devtools`, `helix-session`, `helix-desktop`,
     `helix-viz` and `helix-tasks` (`:1899`).
   - Any other key the user edits locally wins until the next full `syncFromHelix` resets the
     overrides. That includes `context_servers` (arbitrary stdio commands), `language_models`,
     `assistant`, and `agent.tool_permissions`.
   - Zed hot-reloads the file. Only someone who can already write `settings.json` *inside* the
     sandbox can use this, i.e. the exec/terminal/input/upload/workspace-file routes.

**Consequences for the seal:**

- The POST cannot change agent behaviour by itself. Under the seal it becomes SELF, so the panel
  cannot show fabricated entries.
- The real behavioural channel is `GET /zed-config`. It carries project and app MCPs, including
  stdio commands, plus provider endpoints and `agent_tools`, and re-pulls every 30 seconds. That is
  what §3.4 freezes.
- `GET /zed-config` and `/zed-settings` also hand the creator the session key, so they must be SELF.

---

## 3. Design

### 3.1 Model

**`spec_task_seals`, a separate table, not columns on `spec_tasks`.** It has to outlive the task as
a tombstone (§4, S6), and keeping it separate keeps the "omit from updates" rule in one place.

| column | notes |
|---|---|
| `spec_task_id` (PK) | no foreign-key cascade; survives task deletion |
| `sealed_by_key_id`, `sealed_by_user_id`, `sealed_at` | K's non-secret `key_…` id (`types.APIKeyID`, `types/api_key_summary.go:40`) |
| `sealed_config` jsonb | §3.4 |
| `seal_version` | the seal contract this task was created under (§3.8) |
| `min_cluster_version_since_seal`, `cluster_gap_seen_at` | SF5, written by the replica heartbeat (§3.8) |
| `broken_at`, `broken_by`, `broken_reason` | break-glass only (§4) |
| `task_deleted_at`, `task_deleted_by_key_id` | written by the delete path; the tombstone |

`SpecTask.SandboxSealed bool` stays on `spec_tasks` (`not null default false`) as the cheap flag
listings filter on.

**Every task also gets `SpecTask.CreatedByKeyID`.** It is the non-secret id of the API key that
created the task, empty for browser and other credentials. The dry-run probe needs it on the
unsealed control (§3.7).

**Creating a sealed task:**

- It can be set only through the HTTP API, on `POST /spec-tasks/from-prompt` and on `/clone` of an
  unsealed task. helix-org `create_spectask` and the PM skill `CreateSpecTask` run on bot or agent
  keys, which are scoped, so they can never seal: a `sandbox_sealed: true` argument from either
  returns an error naming the HTTP route.
- **Clone is not a sealing path (N1).** `POST /spec-tasks/{taskId}/clone` resolves the sealed
  source, so default-deny refuses it to everyone, K included. To get another sealed lane, the portal
  creates a fresh sealed task. The round-7 sentence "a clone of a sealed task is always sealed" is
  dropped. Cloning an *unsealed* task with `sandbox_sealed: true` is allowed, and makes the cloning
  key K.
- The seal is immutable, as in #3611. `PUT /spec-tasks/{id}` returns 400 on a differing value, and
  the store never updates `spec_task_seals` except for the write-once break and tombstone columns.
- `sandbox_sealed` implies `confine_agent_key`. The create forces confinement on, and returns 400 if
  the caller explicitly sent `confine_agent_key: false`.
- **The caller must be a `sealing` key** (`APIkeytypeSealing`, §3.7), so that K is one credential
  that never doubles as a personal key. A browser session, a runner token, an ordinary personal or
  org key, or a session/project/task-scoped key gets 400.
- **`AutoApprovePullRequests` is forced to `false`** (NB3, §3.10), and `auto_approve_future` is
  refused for the task's lifetime.
- **The sealed repos are created in the same transaction** (§3.11), with the lane's refs under
  `refs/heads/helix-sealed/<task_id>/`.
- **The task must need no human decision.**
  - The create requires `just_do_it_mode: true` (`types/simple_spec_task.go:467`).
  - Under option (a) the lane may never open a PR or merge (§3.10), so its completion path is
    "agent marks complete; the work stays on the sealed refs". `CreatePullRequests`,
    approve-implementation and auto-merge are refused for sealed tasks.
  - Checking that every completion path can reach `done` without a PR is an implementation
    acceptance item.
- **Refused while the cluster cannot enforce it** (§3.8).

**Atomic creation (S7).** One transaction inserts the `spec_tasks` row, its `spec_task_seals` row
and the resolved `sealed_config`, before the task becomes visible to the orchestrator. A
`just_do_it_mode` task is otherwise picked up at once.

- Sessions for a sealed task are created with `Metadata.SpecTaskID` set in the same insert, and
  their confined key (§3.2) is minted before the desktop starts.
- Every key re-mint (`GetOrCreateSessionAPIKey`, `services/spec_driven_task_service.go:2076`)
  produces `APIkeytypeSandboxSession` with `TaskConfined` when the task is sealed. A cached key of
  any other type is deleted and re-minted, as #3611 already does for confinement.

**Derivation and caching.**

- `sealOf` resolves session → `Metadata.SpecTaskID` → `spec_tasks.sandbox_sealed`. A copy on
  session metadata would be writable through `PUT /sessions/{id}`.
- `sealed = true` is cached forever, because it is immutable. `broken_at` is **never** cached
  (N6, §3.3).

### 3.2 Who counts as the sandbox's own identity

`isSandboxSelf(user, session, task)` is true only in these two cases:

1. **The session key itself.**
   - `TokenType == api_key`, `user.SessionID == session.ID`, `user.SpecTaskID == task.ID` and
     `user.TaskConfined`.
   - The same session only. A sibling session of the same task is a different sandbox and does not
     count.
2. **A per-runner token bound to the session's host** (`canRunnerReadSession`). This applies only
   to the routes a host uses: `/zed-config` GET, the credential GET and PUT, and `/external-agents/sync`.
   The **shared** runner token is refused on every sealed route.

Identity is decided by the **credential, never by `user.ID`**. The session key and the creator share
a user id, so any rule based on `user.ID == session.Owner` admits the creator. Admins get no
exemption. An admin can mint a personal key for any user, but a personal key is never "self".

**Keeping the session key secret is a prerequisite.** Without it, "self" equals "anyone who can read
the key":

- Mint sealed session keys with a new type, `APIkeytypeSandboxSession`. Keep `Owner = session.Owner`
  so existing RBAC, git and billing keep working.
- `GET /api_keys` never returns that type's secret to anyone; it returns the prefix only.
- `GET /zed-config`, `/zed-settings` and the credential routes become SELF for sealed sessions.
- `DELETE /api_keys?key=` on a sealed session key is refused. Revoking happens when the desktop
  stops.
- `/revdial` `desktop-{sid}` and `/external-agents/sync` accept only SELF for sealed sessions. This
  closes the takeover paths, where today the creator, admins and any runner-token holder qualify.

### 3.3 Enforcement: default-deny, decided centrally

There is **one decision function and one allow-list**. Four gates call the function: gate 4 is new
in round 8 (§3.10), and the org-runtime check in §3.9 is a gate-2 caller. No gate has a
route-by-route opt-in, so a route that is not listed is refused.

**Route keys are generated, not hand-written (N3).** `/spec-tasks` mixes `{taskId}`,
`{spec_task_id}` and `{id}`, so hand-typed template strings would drift.

- The allow-list is declared **by handler**: `sealAllow(apiServer.stopAgentSession, sealingKey)`.
- At startup `buildRouter` walks the router and produces the key
  `METHOD + " " + mux path template` for each route.
- A route key gains a suffix when one template serves several objects:
  - `#<server>` for `/api/v1/mcp/{server}` (e.g. `POST /api/v1/mcp/{server}#desktop`);
  - `#<tool>` for org-runtime tools (`mcp:helix-tasks:<tool>`, `orgtool:<tool>`);
  - `#git:<service>` for the git HTTP routes (`#git:receive-pack`, `#git:upload-pack`,
    `#git:info-refs`).

```go
// sealGrant is who a sealed-task allow-list entry admits.
type sealGrant string // "self" | "self_runner" | "sealing_key" | "sealing_key_read"

// sealAllowList is the ONLY place a route is permitted on a sealed task. It is keyed
// by handler and expanded to route keys from the router at startup; an absent key is refused.
var sealAllowList = []sealEntry{
    sealAllow(h.stopAgentSession, sealingKey),                    // POST /spec-tasks/{spec_task_id}/stop-agent
    sealAllow(h.stopExternalAgentSession, sealingKey),            // DELETE /sessions/{id}/stop-external-agent
    sealAllow(h.archiveSpecTask, sealingKey),                     // PATCH /spec-tasks/{taskId}/archive (delete precondition)
    sealAllow(h.deleteSpecTask, sealingKey),                      // DELETE /spec-tasks/{taskId}
    sealAllow(h.deleteSession, sealingKey),                       // DELETE /sessions/{id}
    sealAllow(h.getSpecTaskSeal, sealingKey),                     // GET /spec-tasks/{taskId}/seal (§3.7)
    sealAllow(h.listSpecTaskSandboxAccess, sealingKey),           // GET /spec-tasks/{taskId}/sandbox-access (§4)
    // option (a) read set (§3.5): sealingKeyRead, GET/HEAD only, listed one by one
    sealAllow(h.getZedConfig, self, selfRunner),
    sealAllow(h.updateZedUserSettings, self),
    sealAllow(h.handleExternalAgentSync, self, selfRunner),
    sealAllow(h.handleRevDial, self),
    // … every other SELF row of §1 (credentials, daemon callbacks, zed events,
    // the agent's own drive routes, git push to its own refs) …
}

// sealDecision is called by all three gates. It returns the outcome and the reason it records.
func (s *HelixAPIServer) sealDecision(ctx context.Context, user *types.User,
    taskID, sessionID string) (outcome sealOutcome, reason string, err error)
```

`sealDecision` works in this order:

1. If the object belongs to no task, or its task is not sealed, return allowed and record nothing
   for the seal. Normal authz has already run, or runs next.
2. **Read `broken_at` from `spec_task_seals`, uncached, on every decision (N6).**
   - If the read fails, deny (`reason: seal_state_unavailable`) on that replica.
   - If the seal is broken, continue to step 6 and record the call as `outcome: allowed`,
     `reason: seal_broken`. Never return early (S6).
   - The cost is one primary-key lookup per decision, on sealed tasks only.
3. **Classify the caller (S1).**
   - **Platform internals are marked explicitly.** Orchestrator, pollers, reapers, hydra callbacks
     and credit exhaustion wrap their context with `withPlatformCaller(ctx, "<component>")`. Only
     those get `reason: platform`, and only with `user == nil`.
   - Otherwise read the route key the router middleware stamped from `mux.CurrentRoute(r)`, the
     same way `stampRequestAuditRoute` does at `request_audit.go:132`. The MCP gateway and the org
     runtime stamp their `#<server>` / `#<tool>` suffix before dispatching.
   - **A context with `user != nil` and no route key is denied, with `reason: no_route`.** This
     catches MCP tool goroutines, detached contexts and future code paths that forgot to stamp. The
     absence of a route never means "internal".
   - A context with neither a user nor the platform mark is also denied (`reason: no_route`).
4. Look the route key up in `sealAllowList`. Allow only if the caller matches one of that entry's
   grants:
   - `self`: the session key (§3.2);
   - `self_runner`: a per-runner token bound to the session's host;
   - `sealing_key`: `types.APIKeyID(user.Token) == seal.sealed_by_key_id`, with the extra
     binding checks in §3.7;
   - `sealing_key_read`: as `sealing_key`, for the read set only (§3.5).
5. Otherwise return denied with error code `sandbox_sealed`.
6. Write the access record (§4) synchronously, for both outcomes. If the write fails, the request
   fails closed.

**Gate 1, the path guard (router middleware).** It runs on the auth, user-or-runner and runner
routers, after authentication. It resolves the sealed object from the path or query and calls
`sealDecision` before the handler runs.

| route family | how the id resolves |
|---|---|
| `/sessions/{id}` | session → `Metadata.SpecTaskID` |
| `/external-agents/{sessionID}` | session |
| `/spec-tasks/{taskId\|spec_task_id\|id}` | task |
| `/interactions/{interaction_id}` | interaction → session |
| `/organizations/{org_id}/sandboxes/{id}` | sandbox row → `SessionID` / `SpecTaskID` |
| `/work-sessions/{sessionId}` | work session → `SpecTaskID` |
| `/mcp/desktop`, `/mcp/session`, `/revdial`, `/external-agents/sync` | the session id in the query |

Gate 1 refuses before body parsing and before any side effect. So the sealed twin in a probe
(§3.7) answers `sandbox_sealed` even to an empty body.

**Gate 2, the resolver gate (inside the shared authz helpers).** This covers ids that are not in
the path, such as body-addressed routes (§1f: `/zed/events`, the thread events, heartbeat), and a
secondary object a handler loads. These helpers call `sealDecision` right after their own checks
pass:

- `authorizeUserToSession` (`authz.go:589`)
- `authorizeUserToSpecTask` (`authz.go:300`)
- `authorizeUserToZedSpecTask` (`zed_event_handlers.go:76`), whose non-project branch does not
  reach `authorizeUserToSpecTask`
- `authorizeSessionSandboxConfig` (`session_credentials_authz.go:54`)
- `authorizeDesktopID` (`external_agent_handlers.go:46`)
- `loadAuthorizedSandbox` (`sandboxes_api_handlers.go:1004`)
- `loadAuthorizedTerminalSession`, `authorizeWorkspaceReviewRequest`
- `requireArtifactAccess` (`artifact_handlers.go:450`) and the inline artifact paths. These call
  gate 4 (§3.10).
- `SpecTasks.ownedTask` (`org/infrastructure/runtime/helix/spectasks.go:188`), for the org-runtime
  tools (§3.9)
- **`VHostMiddleware.dispatchSandboxPreview`** (`vhost_middleware.go:195`, session branch `:197`;
  N2). The share-* host proxies into a session without any helper. Preview mint is already denied
  for sealed sessions (§1f), so this check is defence in depth against a token minted before the
  seal or by a bug. A sealed session's share-* host answers 403 `sandbox_sealed`.
- the desktop and session MCP backends' authorize callbacks
- the `ownsSessionInKeyScope` paths (`/external-agents/sync`, credentials)
- the `/revdial` desktop registration (`revdial_registration.go:32`)

Every handler that touches a session, task or session-backed sandbox already has to call one of
these helpers; the route-policy test enforces that today. So gate 2 makes default-deny hold even
for a route gate 1 cannot parse.

**Gate 3, the agent-input sinks.** Turns can be produced by routes that resolve a repo or PR rather
than the task (request-rebase, merge, the GitHub review webhook, `helix-tasks` and helix-org MCP
tools). These sinks refuse a sealed target:

- `sendCommandToExternalAgent`
- `enqueueAgentMessage` / `persistQueuedPrompt`
- `RunExternalAgent` for an existing session
- **delivery**, not just enqueue (S7). `sendQueuedPromptToSession` re-checks the gate when it
  dispatches a queued row. So do the startup `ReconcileStuckSendingPrompts` (`server.go:850`) and the
  reconnect resume path. A row queued before a seal decision could change, or re-delivered after a
  restart, is checked again.

Git receive-pack is gate 4 (§3.10).

The sink cannot use "no route in the context" to recognise platform internals.
`sendCommandToExternalAgent` takes no context at all (`websocket_external_agent_sync.go:2483`), and
internal turns are often produced while a request's context is live. So the sink decides on the
turn's **provenance** (#3613), which every turn already carries:

| provenance on the turn | sealed target |
|---|---|
| `system` with a kind Helix composes itself, with no caller text: `planning_prompt`, `implementation_prompt`, `pr_settled`, `push_required`, `completion_sent_back` when auto-approved. Templates that embed guidelines use the **frozen** guidelines from `sealed_config` (S8, §3.4). | allowed |
| `ci_status` **in its status-only form for sealed lanes**: check names, conclusions, and the URL. CI log text or failure excerpts would be caller-supplied (whoever controls the CI job controls the text), so sealed lanes get the status-only form (S8). | allowed |
| `ci_status` carrying log or annotation text | denied |
| `system` with a kind that relays caller-supplied text or decisions (`design_review_comment`, `revision_requested`, `review_comment`, `attachment_added`, `rebase_required`, `pr_proposal_outcome`, `pr_auto_approve` (NB3), `org_bot_*`, `cron_trigger`, chat-channel kinds) | denied |
| `user`, or `api` with an `api_key_id` other than the session's own key | denied |
| `api` with the session's own key | allowed |

Non-turn commands (`cancel`, `open_thread`, `respond_question`, `cancel_question`) come only from
HTTP routes that gates 1 and 2 already deny. The `/command` route is deleted (§5).

When the sink drops a turn, it writes a `denied` access record. Examples are a GitHub reviewer's
`review_comment`, and a sibling task's `SendSpecTaskAgentMessage`.

**Route-policy test (fails on unclassified routes; S2).** Add a test in `route_auth_policy_test.go`
that walks the production router (`buildRouter`). Today's test does not cover enough for this: 41
routes are `noAuthzHelper` and 50 are `handlerAuth`, and `reachesAuthorize`
(`route_auth_policy_test.go:897`) only follows calls inside package `server`. So the round-8 rules
are:

1. **Every** `noAuthzHelper(...)` and `handlerAuth(...)` route must declare `.seal(...)`, whether or
   not the walk finds a sealable lookup. These are exactly the routes whose authorization the test
   cannot see. They include claude/codex-credentials, `/ws/*`, `/api/v1/mcp/{server}`, the git HTTP
   routes and the GitHub-compatible API.
2. Every route with a gate-1 prefix, and every route whose handler reaches a gate-2/gate-4 helper or
   a sealable store lookup, must declare `.seal(...)`. The sealable lookups are
   `Store.Get{Session,SpecTask,Sandbox,Interaction,SpecTaskWorkSession,SpecTaskZedThreadByZedThreadID,Artifact,GitRepository}`.
   The AST walk is extended beyond package `server` to `org/infrastructure/runtime/helix` and
   `services` (for `GitHTTPServer`).
3. **Multiplexed routes declare per sub-key.**
   - `/api/v1/mcp/{server}` declares one `.seal` per registered backend (`#desktop`, `#session`,
     `#helix-tasks`, `#helix-org`, `#external`, `#kodit`, `#helix-viz`). An unregistered server
     name is denied.
   - The org runtime declares one per tool (`mcp:helix-tasks:<tool>`), generated from
     `SpecTaskAgentTools` (`org/interfaces/mcptools/defaults.go:180`).
   - A tool or backend without a declaration fails the test.
4. **Inline-auth handlers** (`noAuthzHelper` / `handlerAuth` with `sealDeny()`) must contain an
   explicit `sealCheck(...)` / `sealOfRef(...)` / `sealOfArtifact(...)` call. The AST test asserts
   it. `dataCallBeforeHelper` (`route_auth_policy_test.go:1008`) is extended to treat the seal call
   as a helper, so no store write, dial or command send may come before it (S3).
5. **Ref and SHA parameters (SF7).** Any route with a path or query parameter named `ref`, `branch`,
   `sha`, `commit`, `base`, `head` or `source_branch`, whose handler reaches `GetGitRepository`, must
   call `sealOfRef` on that parameter before its first git or store data call. The AST test finds
   the parameter names in the route template and in `r.URL.Query().Get(...)` / `mux.Vars` reads.
6. **The git hook layer (round-8 S1 follow-up).** The pre-receive hook in
   `services/gitea_git_helpers.go` (allow-all when `HELIX_ALLOWED_BRANCHES` is unset, `:442-455`)
   gains two env-independent rules: the shared repo refuses `refs/heads/helix-sealed/*` to every
   pusher, and a sealed repo refuses everything unless `HELIX_SEAL_SELF_TASK` matches (§3.11). A
   hook test runs both with an **empty** environment, the way internal pushes run.
7. **Query filters naming a session, interaction or task (round 10, N9-1).** See §3.14 for the
   parameter list and the AST rule. A query-filtered read is an id resolver like any other.

The test fails if:

- a route or sub-key that must be classified has no declaration;
- a `sealAllow` disagrees with `sealAllowList`;
- the allow-list names a handler the router does not serve.

A second test runs every declared `sealDeny()` route against a sealed fixture with K, an org owner
and an admin. It expects 403 `sandbox_sealed` and a `denied` access record, with the fake store and
connman recording no write, dial or send.

Cost: one cached lookup per request on the gate-1 prefixes. `sealed=true` is cached forever. For
`false`, cache only the session→task mapping, never the negative seal.

Internal callers are allowed only through the explicit `withPlatformCaller` mark at gates 1, 2 and
4, and through system provenance at gate 3:

- the orchestrator;
- `execCommandInDesktop` (sets git identity, `services/spec_driven_task_service.go:1721`);
- `ensureFeatureBranchInContainer`;
- workspace checkpoint capture (`interaction_code_changes.go:69`);
- prompt-queue delivery;
- the CI and PR pollers;
- hydra.

Each of these runs a fixed, server-built action.

### 3.4 Config that changes mid-run (cyber R013 point 2): freeze inherited config, refuse own config

There are two kinds of config a running sealed sandbox can receive.

| source | owner | recommendation | why |
|---|---|---|---|
| Project skills and MCPs, app/agent MCPs, provider endpoints, re-pulled by `getZedConfig` (`zed_config_handlers.go:123-130`, `:164`, `:200`) every 30s | the project, app or org, shared with other tasks | **Freeze** | Refusing a project edit because one task is sealed would block the whole project. Freezing scopes the effect to the sealed task. |
| `PATCH /sessions/{id}/execution-config` (`session_execution_config_handlers.go:118-180`), `PATCH /spec-tasks/{id}/execution-config`, `/interactive-embed-model`, `PUT agent_tools`, switch-agent | the sealed task itself | **Refuse** (not on the allow-list: `sandbox_sealed`, §3.3) | An explicit 403 is honest. Accepting a write and then silently serving the old config would mislead whoever made the change. |

**Freeze mechanics:**

- At task creation, when the seal is set, resolve and store the declarative inputs in
  `SpecTask.SealedConfig`. These are:
  - the assistant config (runtime, model, provider_ref and reasoning effort, i.e. the
    `CodeAgentConfig`);
  - the agent's MCP definitions, the project `Skills.MCPs`, and `agent_tools`;
  - the provider endpoint **base URL and id**. The API key stays by reference so it can rotate.
  - SHA-256 hashes of the project secret set (names and value hashes), `.helix/startup.sh` at the
    base commit, and the skills ref.
  - **The guidelines text (S8, SF6).** In PTaaS the guidelines carry the rules of engagement, and
    an org owner or admin can edit them mid-run. Today they are composed live in **three** places:
    - `services/agent_instruction_service.go:716` (`getGuidelinesForTask`; org `:731`, user `:738`,
      project `:742-747`);
    - `services/spec_driven_task_service.go:505-514`;
    - `services/spec_driven_task_service.go:871-880`.

    They are rendered by `services/spec_task_prompts.go` (the `Guidelines` field `:16`, template
    `:55`, set at `:344`; `BuildApprovalInstructionPrompt` takes them as a parameter at `:493`).
    Round 9 routes all three through one `guidelinesFor(ctx, task)`:
    - for a sealed task it returns `sealed_config.guidelines`;
    - otherwise it composes org/user plus project as today.

    An AST test asserts that `Organization.Guidelines`, `Project.Guidelines` and
    `UserMeta.Guidelines` are read **only** inside `guidelinesFor` in package `services`. So a
    fourth composer can't appear unnoticed, and every Helix-composed turn on a sealed lane carries
    the RoE that held at creation.
- `getZedConfig` (and every container start, which on a sealed task is platform-initiated only) build from `SealedConfig` when the
  task is sealed. Tokens and URLs are still rendered per request, so the session key and OAuth
  tokens rotate normally, but the *set* of MCP servers, their commands and URLs, the model and the
  endpoint URL are fixed.
- If a frozen dependency disappears (the endpoint is deleted, an MCP's OAuth connection is revoked),
  the sandbox fails visibly. It never falls back to live config.
- **A null `sealed_config` on a sealed task fails closed (S7).** `getZedConfig` returns 503
  `sealed_config_missing` and the desktop does not start. Atomic creation (§3.1) makes this
  unreachable in normal operation; the check guards against a partial write or a migration bug.
- Start-time inputs (secrets, startup script, skills ref) are re-hashed on every container start. A mismatch
  writes a `config_drift` access event and shows on the task. Freezing secret *values* is out of
  scope for v1: secrets are encrypted per project and rotated by design.
- `config_changed` publishes to a sealed session are dropped, except `color_scheme`
  (`user_handlers.go:564`), which is cosmetic.

Recommendation: freeze inherited config and refuse own config, as above. Freeze-only would leave the
execution-config routes reporting success on no-ops. Refuse-only cannot stop project-level edits.

### 3.5 What still works under the seal (the whole allow-list)

- **K may control the lane:**
  - stop (`POST /spec-tasks/{id}/stop-agent`, `DELETE /sessions/{id}/stop-external-agent`);
  - archive, the precondition for delete (`PATCH /spec-tasks/{id}/archive`);
  - delete (`DELETE /spec-tasks/{id}`, `DELETE /sessions/{id}`);
  - read the seal (`GET /spec-tasks/{id}/seal`) and the access record
    (`GET /spec-tasks/{id}/sandbox-access`).
- **K may read the lane (`sealing_key_read`, portal option (a)).** Every entry is a GET or HEAD
  route, listed by handler; no other method is ever in this grant.
  - task: `GET /spec-tasks/{id}`, `/specs`, `/progress`, `/usage`, `/pr-proposals` (empty for
    sealed lanes), `/design-reviews*` GET, `/attachments` GET and content, `/zed-threads`;
  - sessions: `GET /sessions/{id}`, `/interactions*`, `/turns/{n}`, `/toc`, `/search`, `/usage`
    (metadata only for sealed sessions: no LLM request or response bodies, §3.14),
    `/step-info`, `/output`, `/workspace-status`, `/sandbox-state`, `/rdp-connection` (URLs only);
  - desktop, read-only: `/external-agents/{sid}/screenshot`, `/file`, the workspace-review and
    workspace-file GET routes, and `/ws/stream` with `X-Helix-Readonly` forced. `/ws/input`,
    clipboard and terminal are **not** in the set.
  - git: **fetch only** (upload-pack and info-refs) from the lane's sealed repos through
    `/git/sealed/{T}/{repoID}` (§3.11). The shared repo holds nothing of the lane, so the git read
    API (tree, commits, contents) on `helix-sealed/T/work` returns not-found by design (§3.13).
  - artifacts (gate 4): `GET /artifacts/{id}`, `/versions`, `/download`, `/embed*`, `/document`, for
    the lane's artifacts;
  - listings: the sealed task and its sessions appear in K's own `GET /spec-tasks`,
    `GET /sessions`, `/clone-groups/{id}/progress` and `/projects/{id}/artifacts` results.
- **The sandbox itself** (SELF rows in §1): the daemon, sync, RevDial, config and credentials, its
  callbacks and zed events, the LLM proxy, git push and fetch on its own refs, its own artifacts,
  and its own MCPs.
- **Platform internals** (marked with `withPlatformCaller`): orchestrator, hydra, pollers,
  checkpoints, reapers, credit-exhaustion stop, and the system turns allowed under gate 3.

**Nobody else may act on or read the lane.** That includes the finder lane, project members, org
owners and admins acting through the API. Refused for everyone, K included:

- further turns (`/messages`, the prompt queue, approvals, comments);
- resume, restart and cancel;
- input, terminal and clipboard;
- every write to the lane's refs or artifacts;
- opening a PR or merging.

The "(candidate: …)" notes in §1 list what an owner could later add, one entry at a time. Each
addition needs a review, and its route-policy entry changes visibly.

Under the seal, nobody but the agent can type into Zed. So an interaction with origin
`external_agent` comes from the agent itself, which makes it a useful signal for the verifier.

**Open question for the owner (not decided here).** Should org owners get `stop` for cost control?
Today a runaway sealed sandbox is stopped by K, by its TTL or credit exhaustion (platform), or by
break-glass. Adding `{org_owner: stop}` is a single allow-list entry. If it is adopted, it is
recorded distinctly from K's stop (§4, cyber requirement).

### 3.6 Stop, delete and cancel (cyber R013 point 3)

**Stop and delete survive, for K only.** These routes are allow-listed for K:

- `POST /spec-tasks/{id}/stop-agent`
- `DELETE /sessions/{id}/stop-external-agent`
- `PATCH /spec-tasks/{id}/archive` (the delete precondition)
- `DELETE /spec-tasks/{id}`
- `DELETE /sessions/{id}`

None of them forwards caller-supplied content to the agent. Each writes an access record with
`outcome: allowed` and `kind: lifecycle`, and the task shows "stopped by K at T".

Every other lifecycle route is denied, by default-deny:

- `PUT status=backlog`;
- archive through `PUT`;
- the org Sandboxes `DELETE` / `PATCH`;
- resume, restart and ensure-agent.

These are equivalent to routes K already has, or they restart the agent at a time the caller
chooses. Platform internals (TTL, credit exhaustion, reapers) still stop and destroy the sandbox.

**Cancel is denied by default.** It becomes an allow-list candidate only when it is recorded and
content-free, which matches the reviewer's position.

- **Turn cancel** (`POST /sessions/{id}/cancel`, `session_handlers.go:2600` →
  `cancelActiveTurn` at `prompt_history_handlers.go:716`):
  - It sends a fixed, server-built cancel for the current request id and carries no caller content.
  - It qualifies once it records `cancelled_by` (a `TurnProvenance`) on the interaction it ended.
  - Cancelling at a strategic moment changes an outcome. An unrecorded cancel cannot be told apart
    from an agent failure.
- **Question cancel** (`POST /interactions/{iid}/questions/{rid}/cancel`,
  `interaction_question_handlers.go:76`):
  - It sends a fixed `cancel_question` for a `request_id` that is validated against the pending
    question, and carries no content.
  - It qualifies once `ResolvedQuestion` records the actor (`ResolvedBy TurnProvenance`).
- Even once qualified, cancel is added to `sealAllowList` (for K) only by an owner decision.
- **Question respond** carries caller-chosen `answers` into the agent. It stays denied.
- **`/clear`** destroys agent context. It stays denied.

### 3.7 The sealing key K, and the control/twin probe

K is the API key that created the sealed task. Its non-secret id is stored in
`spec_task_seals.sealed_by_key_id` (§3.1).

**K can set the seal and read it back.**

- **Set:** `POST /spec-tasks/from-prompt` with `sandbox_sealed: true` and `just_do_it_mode: true`,
  using K.
  - The response **echoes** `sandbox_sealed: true`, `sealed_at`, `sealed_by_key_id` (K's own `key_…`
    id, never a secret) and `seal_version`.
  - If the echo is missing, the portal must treat the lane as unsealed and delete it (§3.8).
- **Clone preconditions (nit).** Cloning an unsealed task with `sandbox_sealed: true` applies
  **every** create precondition: a `sealing`-key caller, `just_do_it_mode`, atomic `sealed_config`
  and sealed repos, the cluster gate (§3.8), and forced `AutoApprovePullRequests = false`.
- **Read back:** `GET /spec-tasks/{taskId}/seal` (new; allow-listed for K) returns:

  ```
  {spec_task_id, sandbox_sealed, seal_version, sealed_at, sealed_by_key_id, confine_agent_key,
   seal_broken_at, seal_broken_by, seal_broken_reason, task_deleted_at, blocked_attempts,
   min_cluster_version_since_seal, cluster_gap_seen_at}
  ```

  - On an **unsealed** task the same route answers anyone with normal read authz, with
    `sandbox_sealed: false`, so a probe reads both tasks the same way.
  - After the task is deleted it still answers K from the tombstone (§4).
  - The exact value of every field in each state is tabulated in §3.8.

**A sealed task refuses K on every route outside K's allow-list** (§3.5). Being K does not widen
anything else. K gets 403 with:

```json
{"error": "sandbox_sealed", "route": "<route key>", "spec_task_id": "spt_…"}
```

That is the same body an org owner or admin gets.

**K is bound to the key, not to K's user (S4).** Today `GET /api_keys` returns plaintext keys to
any unscoped credential of the owner (`handlers.go:1296`); redaction happens only in
`redactAPIKeysForScopedCaller` (`handlers.go:1247`). So K's user, by browser, another personal key,
or an admin-minted key for that user, could read K and act as K. Five rules close that:

1. **K is a dedicated key type, minted for sealing (NB4).**
   - New `APIkeytypeSealing`, minted by `POST /api_keys?type=sealing`. The plaintext is returned
     **once**, in the mint response. From then on `GET /api_keys` shows only its prefix, to every
     caller, unscoped ones included.
   - `isPersonalAPIKey` (`handlers.go:1277`) returns false for it. So the no-`types` branch of
     `GET /api_keys` (`handlers.go:1311-1320`), which returns the caller's latest personal key,
     never returns K (S4).
   - The same holds for `getOrCreateUserAPIKey`, which hands a user's personal key to Kodit at repo
     create (`git_repository_handlers.go:136`, `:367`) and re-register (`kodit_handlers.go:476`).
     So K's plaintext can never reach Kodit.
   - Only a `sealing` key can seal. An ordinary personal or org key gets 400.
   - **Kodit uses a service credential** instead of a user's personal key, so no user key (K or any
     other) is ever stored in Kodit. Until that lands, the create path refuses to seal if K's id
     appears in any Kodit registration. By construction it cannot, because K was minted only for
     sealing.
2. **Revoking a live sealing key is refused.** `DELETE /api_keys` (`controller/handlers.go:75`)
   returns 409 `sealing_key_in_use` while any task sealed by that key is not archived or deleted.
   - **Recovery when K is lost anyway** (user deleted, key purged by an operator): the lane is
     stopped by the platform.
   - `GET /seal` and `/sandbox-access` then become readable, read-only, by `sealed_by_user_id`'s
     **personal credentials**, recorded as `actor_class: sealing_user_recovery`.
   - Nothing else opens, and no write is ever re-granted.
3. **Keys minted by K are not K.** K can mint another key (`POST /api_keys`), but that key has its
   own `key_…` id, so it is not K. The grant compares key ids only.
4. **Each access record carries a request fingerprint**, so a stolen copy of K is distinguishable
   from the portal:
   - `credential_fingerprint`, the same HMAC fingerprint the request audit uses
     (`types/request_audit.go:33-37`);
   - `client_ip_hash` and `user_agent_hash`;
   - for browser credentials, the auth-session id.
5. **No other credential of K's user passes any `sealing_key` grant.** The probe checks this
   (step 6).

**The probe this design supports (S3).** K creates two tasks in the same project with identical
inputs:

- **A**, an unsealed control (`sandbox_sealed: false`), whose `CreatedByKeyID` is K;
- **B**, the sealed twin (`sandbox_sealed: true`), sealed by K.

Calling every deny route for real on A would run restart-agent, clear, fork, sandbox DELETE, merge
and preview-token mint. So the probe uses a dry run.

- **`X-Helix-Seal-Probe: 1`.**
  - Honoured only when the caller is K and the target task's `CreatedByKeyID` (A) or
    `sealed_by_key_id` (B) is K.
  - The request runs authentication, normal authz and the seal decision, then stops.
  - The handler body never runs: the gate returns `200 {"probe_ok": true, "route": "…"}` on
    allowed, or the normal `403 sandbox_sealed` on denied.
  - For anyone else the header is ignored, and the request runs normally.
- **Every route declares where its id comes from.** `GET /api/v1/seal-policy` returns
  `{seal_version, routes: [{key, method, template, id_in, grants}]}`. `id_in` is one of:
  - `path:<var>`;
  - `query:<name>`;
  - `body:<json-pointer>` (e.g. `body:/spec_task_id` for `/zed/events`);
  - `ref:<path|query var>` (gate 4);
  - `artifact:<var>`;
  - `tool:<name>` (org runtime, called through `/api/v1/mcp/helix-tasks`).
- **The dry run must stop before any side effect.**
  - Gate 1 short-circuits before the handler.
  - For gate-2 and gate-4 routes the handler itself reaches the seal call. The extended
    `dataCallBeforeHelper` test (§3.3) proves that no store write, dial, git command or send comes
    before that call, so returning at the call is side-effect-free.

**Probe steps,** all using K:

1. `GET /spec-tasks/{A}/seal` → `sandbox_sealed: false`. `GET /spec-tasks/{B}/seal` →
   `sandbox_sealed: true`, `sealed_by_key_id` = K, and `seal_version` ≥ the portal's minimum.
2. `GET /api/v1/seal-policy` → the probe set: every route whose grants for K are `deny`, plus the
   `sealing_key` and `sealing_key_read` routes for the positive checks.
3. For each deny route, build the id from `id_in` (A's or B's id, ref or artifact), and send it with
   `X-Helix-Seal-Probe: 1`:
   - A must answer `200 probe_ok`;
   - B must answer `403 sandbox_sealed`.
4. `GET /spec-tasks/{B}/sandbox-access?kind=access&outcome=denied` must show one row per probe call
   on B, flagged `probe: true`. `seal_broken_at` must still be null: denied attempts are not
   breaches (§4).
5. The `sealing_key` and `sealing_key_read` routes must answer `probe_ok` on B with K.
6. The same routes, sent with another credential of K's user, must answer `403 sandbox_sealed` on
   B. That proves the grant is bound to the key.

The control matters because a refusal on B alone could come from ordinary RBAC. `probe_ok` on A,
with the same key and route, shows that the seal is what refused B.

### 3.8 Unsupported builds and rolling deploys (S5)

**The risk.** An old build silently creates an unsealed lane. `createTaskFromPrompt` decodes with
plain `json.Decoder` and no `DisallowUnknownFields` (`spec_driven_task_handlers.go:138`), so it
drops `sandbox_sealed` and creates an **unsealed** lane. On that build `GET /seal` returns 404, the
same as a missing task. During a rolling deploy, old replicas enforce nothing.

**Five rules:**

1. **Capability discovery.** `GET /api/v1/seal-policy` returns `seal_version` (starting at `1`; it
   increments when the allow-list contract changes). **A 404 means the build does not support
   sealing.** The portal checks this before creating a lane.
2. **The create response echoes the seal.** The portal requires `sandbox_sealed: true`,
   `sealed_by_key_id` = K and `seal_version` ≥ its minimum. On anything else it deletes the task
   with K and aborts.
3. **Cluster-wide minimum version.**
   - Each API replica records `(replica_id, seal_version, seen_at)` in a small heartbeat table every
     30s. This is a table in the existing API, not a new service.
   - A sealed create is refused (`503 seal_unsupported_by_cluster`) unless every replica seen in the
     last 2 minutes reports `seal_version` ≥ the version the request needs.
   - So a lane is never created while an old replica could serve its routes. An old replica coming
     back after the lane exists is an operational fault: deploy tooling must not roll back past the
     version.
4. **Fail closed on unknown seal versions.** A replica that reads a `spec_task_seals` row with a
   `seal_version` higher than it understands denies every non-platform caller on that task, with
   `reason: seal_version_unknown`.
5. **Version during the run, not just at create (SF5).** `seal_version` ≥ 3 at create time says
   nothing about a later rollback. A v1 or v2 replica would serve live config and turns, and a
   pre-seal build ignores the seal entirely.
   - `GET /api/v1/seal-policy` reports `cluster_min_seal_version`: the minimum over replicas seen in
     the last 2 minutes.
   - The heartbeat writer, every 30s, sets `spec_task_seals.min_cluster_version_since_seal =
     min(current value, cluster_min_seal_version)` on every live sealed task. `GET /seal` returns
     it. A heartbeat gap longer than 2 minutes sets `cluster_gap_seen_at`.
   - **Cyber's rule:** any value below 3 during a run, or any `cluster_gap_seen_at`, marks the lane's
     evidence as tainted.
   - **Deploy guard.** `scripts/deploy-prod.sh` refuses to roll the API back to a build whose
     `seal_version` is below the highest `seal_version` of any live sealed task. Rollback past it
     requires archiving those lanes first, or an explicit operator override that is itself logged
     on every affected lane as `kind: config_drift`, `reason: cluster_rollback`.
   - **Limitation, stated plainly.** A pre-seal build writes no heartbeat, so if one serves traffic
     next to new replicas, the new replicas cannot see it. The deploy guard is the control for that
     case. The gap detection covers a full rollback, not a mixed one.

**Exact field values per state** (`GET /spec-tasks/{id}/seal`, as K):

| state | HTTP | `sandbox_sealed` | `seal_broken_at` | `blocked_attempts` | `task_deleted_at` |
|---|---|---|---|---|---|
| unsupported build | `GET /api/v1/seal-policy` → 404; `/seal` → 404 | — | — | — | — |
| never sealed | 200 | `false` | null | `0` | null |
| sealed, intact | 200 | `true` | null | `0` | null |
| sealed, intact, with blocked attempts | 200 | `true` | null | `> 0` (count of `outcome: denied`) | null |
| sealed, broken | 200 | `true` | timestamp | any | null |
| sealed, task deleted (tombstone) | 200 | `true` | null or timestamp | any | timestamp |

`sandbox_sealed` never flips to `false` after creation. A broken seal is `true` with
`seal_broken_at` set, never a silent `false`.

### 3.9 Org-runtime spec-task tools (B1)

**The gap.** `SpecTasks` in `org/infrastructure/runtime/helix/spectasks.go` serves the
`helix-tasks` MCP backend (`mcp_backend_spectask.go:58`) and helix-org bots.

- Its only object check is `ownedTask` (`:188`), which compares `task.ProjectID == projectID`.
- Every per-task tool calls it: `Get` `:341`, `Update` `:353`, `StartPlanning` `:407`,
  `SendAgentMessage` `:431`, `ListAgentMessages` `:459`, `StartAgent` `:508`, `StopAgent` `:530`,
  `Complete` `:545`, `RestartAgent` `:563`, `ReviewSpec` `:588`, `ApproveSpec` `:609`,
  `RequestChanges` `:643`, `CreatePullRequests` `:675`.
- `List` (`:314`) returns the project's tasks without it.
- All of this goes through the single `/api/v1/mcp/{server}` route, which has no object helper.
- The backend refuses confined keys (`:69`), so the sealed lane itself can't call it. But any
  **unconfined** sibling session key in the project can, and so can org bots.
- Only `SendAgentMessage` reached gate 3.

**The fix.**

- **The seal check goes inside `ownedTask`.** After the project match, it calls `sealDecision` with
  scope `task`, route key `mcp:helix-tasks:<tool>` (or `orgtool:<tool>` for bots), and the caller's
  identity.
  - The MCP gateway stamps that key into the context before dispatching, so `no_route` cannot
    trigger by accident.
  - No org-runtime caller is ever K or SELF: K is an API key on the HTTP API, and the sealed lane's
    confined key is refused by the backend. So the decision is always `denied`, recorded with
    `scope: task` and `reason: not_allowlisted`.
  - The tool returns an MCP error carrying `sandbox_sealed`.
- **`List` filters sealed tasks out in the store query** (`ListSpecTasks` gains
  `ExcludeSealed: true`), so the lane is not even visible to siblings or bots.
- **`Create` (`:202`) is unaffected**: it makes a new task. A bot can't create a *sealed* task,
  because sealing requires an API-key caller (§3.1).
- **seal-policy lists every tool** under `mcp:helix-tasks:<tool>` with `grants: deny` and
  `id_in: tool:<arg>`. The route-policy test generates the expected set from `SpecTaskAgentTools`
  (`org/interfaces/mcptools/defaults.go:180`).
- **Tool count (nit).** There are **13 per-task tools** that go through `ownedTask`: `Get`, `Update`,
  `StartPlanning`, `SendAgentMessage`, `ListAgentMessages`, `StartAgent`, `StopAgent`, `Complete`,
  `RestartAgent`, `ReviewSpec`, `ApproveSpec`, `RequestChanges`, `CreatePullRequests`. Then there
  is `List`, which filters, and `Create`, which can't seal.
- **Test:** for each of the 13 per-task tools, a sibling unconfined session key and an org bot
  targeting the sealed task get `sandbox_sealed` and one `denied` record per call. `List` omits the
  task. `Create` with `sandbox_sealed: true` errors.

### 3.10 Lane state addressed by repo, ref or artifact (B2), and portal read option (a)

Gates 1 and 2 resolve only session, task and sandbox ids. A lane's work also lives in its git
refs, its PR and its artifacts. **Gate 4** covers those.

**Round 9 makes the git part structural.** A ref-name check (`sealOfRef` alone) cannot keep a
sealed lane private while its objects sit in the shared object store. Cyber showed on git 2.51
that protocol v2 serves a hidden commit by SHA even with `uploadpack.hideRefs` and all three
`allow*SHA1InWant=false` set (NB1). gitnamespaces(7) SECURITY gives the same warning. So:

- **The sealed lane's objects never enter the shared repository.** Each sealed task gets its own
  bare repository for each repo it works on, with one-way alternates onto the shared repo (§3.11).
  What is not in the shared repo cannot be fetched from it, by SHA or otherwise, under any protocol
  version.
- **`sealOfRef` / `sealOfArtifact` remain as checks**, now mostly defence in depth for git. They
  are still the primary control for artifacts.

```go
// sealedRepoFor returns the sealed bare repo a caller's git request must use, or "" for the shared one.
func (s *GitHTTPServer) sealedRepoFor(ctx context.Context, user *types.User, repoID string) (path string, taskID string, err error)
// sealOfRef rejects the reserved prefix and SHAs not reachable from a non-sealed ref (defence in depth).
func (s *HelixAPIServer) sealOfRef(ctx context.Context, repoID, refOrSHA string) error
// sealOfArtifact maps an artifact to its lane via Artifact.SpecTaskID, else ActiveVersion.SourceSpecTaskID.
func (s *HelixAPIServer) sealOfArtifact(ctx context.Context, a *types.Artifact) (taskID string, sealed bool, err error)
```

**The reserved namespace (SF1).** Every sealed ref lives under `refs/heads/helix-sealed/<task_id>/`,
inside the sealed repo:

- the feature branch is `helix-sealed/<task_id>/work` (`task.BranchName` is set to this for sealed
  tasks);
- the specs and progress branch is `helix-sealed/<task_id>/specs`;
- the shared `helix-specs` branch is never written by a sealed lane.

The specs readers take the task's specs ref instead of the constant: `SpecsBranchName` users at
`services/git_http_server.go:2038`, `:2044` and `services/task_progress_parser.go:50`, `:90`, plus the
prompts and workspace setup.

In the **shared** repository the prefix is reserved for nobody:

- the pre-receive hook refuses any update under `refs/heads/helix-sealed/`, **regardless of
  environment**;
- every fetch refspec (`SyncAllBranches`, `PullFromRemote`) carries the negative glob
  `^refs/heads/helix-sealed/*`, so an upstream cannot plant a ref there;
- a branch named exactly `helix-specs-sealed` or anything under the prefix is refused on create;
- so nobody can pre-plant or collide with a lane's ref.

**Writes.**

- **The sealed lane** writes only to its sealed repos, through git push. Its session key resolves
  to them (§3.11).
  - Every shared-repo write API refuses the lane's own key: `PUT /contents`
    (`git_repository_handlers.go:1172`), branch create (`server.go:1818`), PR create, and the
    `/push`, `/pull`, `/push-pull` routes.
  - So a prompt-injected lane cannot place its work on a readable branch (NB3).
- **Everyone else** writes only to the shared repo, which holds no sealed refs or objects. The
  user routes that take an arbitrary branch therefore cannot reach the lane (NB2):
  - `POST /git/repositories/{id}/push?branch=` (`git_repository_handlers.go:1448`, force push at
    `:1482`);
  - `/pull?branch=&force=true` (`:1383` → `PullFromRemote`, `services/git_repository_service_pull.go:372`);
  - `/push-pull` (`:1514` → `PushToOrigin` / `PushToHelixBare`,
    `services/git_repository_service_contents.go:45` / `:32`).

  Changing `ExternalURL` (`PUT /git/repositories/{id}`, `:308`) only redirects the shared repo.
- **Platform pushers** also operate on the shared repo by repo id, so they cannot carry sealed refs:
  - crash-recovery push (`services/git_repository_service.go:334`);
  - external sync (`services/git_external_sync.go:109`);
  - post-push upstream push (`services/git_http_server.go:867`, `:934`, `:1481`);
  - workflow pushes (`server/spec_task_workflow_handlers.go:293`, `:597`);
  - PR proposals (`services/spec_task_pr_proposals.go:552`, then `CreatePullRequest` at `:561`);
  - the branch-PR route (`git_repository_handlers.go:1872`).
- **Choke-point guards (defence in depth, NB2).** These refuse any ref under the reserved prefix
  and any sealed repo path, for every caller, the platform included:
  - `PushBranchToRemote` (`services/git_repository_service_push.go:17`);
  - `PushToOrigin`, `PushToHelixBare`, `PullFromRemote`, `SyncAllBranches` (`:450`);
  - `CreatePullRequest` and the merge paths.

  A sealed repo has **no upstream remote configured at all**, so it has nothing to push to.
- **PRs and merges (NB3).**
  - A sealed create forces `AutoApprovePullRequests = false` at every `InitAutoApprovePullRequests`
    call site (`types/spec_task_pr_proposal.go:113`; callers `spectasks.go:291`,
    `agent/skill/project/spec_task_create_tool.go:241`, `spec_task_clone_handlers.go:254`, and
    from-prompt).
  - `auto_approve_future` (`services/spec_task_pr_proposals.go:362`) is refused for sealed tasks.
  - The session-MCP `propose_pull_request` tool (`mcp_backend_session.go`; auto-approved opening at
    `:249-258`) moves from SELF to **DENY** for sealed tasks, as does every
    `/spec-tasks/{id}/pr-proposals*` route.
  - The gate-3 sink adds `pr_auto_approve` to its deny list.
  - Native and compat PR create/edit/merge (`git_pull_request_handlers.go:365`, `:404`;
    `github_compat.go:42`, `:376`, `:452`) refuse a head or base under the reserved prefix. The
    platform auto-merge paths skip sealed tasks.
- **Artifacts.**
  - `PUT` / `DELETE /artifacts/{id}` and new versions go through `requireArtifactAccess`
    (`artifact_handlers.go:450`) → `sealOfArtifact`. They are allowed only for SELF, i.e. the lane's
    own confined key, which `enforceKeyArtifactTask` (`:494`) already binds.
  - A sealed lane's artifact can't be set to `visibility: public` (`:118`, `:303`).

**Reads (K and SELF only — option (a)).**

- **Git.**
  - The shared repo contains nothing of the lane.
  - K and SELF fetch the lane through the sealed-repo URL (§3.11), which is allow-listed for them
    alone.
  - Branch listings cannot show sealed names: `GitRepository.Branches` (`types/git_repositories.go:52`)
    is persisted by `SyncAllBranches` from the shared repo only. It is returned by repo GET and
    list, and counted in `/git/{id}/status`. A defensive filter drops the reserved prefix anyway
    (SF2).
- **The git read API with ref/SHA parameters** (`server.go:1817-1842`: branches, tree, contents GET,
  file-content, files, grep, commits) and **PR reads** on both APIs (`server.go:1845-1846`;
  `github_compat.go:41`, `:43`, `:45`, `:46`):
  - they operate on the shared repo;
  - as defence in depth, `sealOfRef` rejects the reserved prefix, and rejects SHAs not reachable
    from a non-sealed ref;
  - an AST assertion makes this mandatory (SF7, §3.3).
- **Kodit and its read routes (NB4).**
  - Kodit indexes the shared repo only. Sealed repos are never registered with Kodit.
  - So `/kodit-commits`, `/search-snippets`, `/semantic-search`, `/keyword-search`, `/wiki*` and
    `/enrichments` (`server.go:1822-1834`) cannot serve sealed content.
  - The key Kodit clones with is fixed separately (§3.7): K can never be that key.
- **Artifacts.**
  - `GET /artifacts/{id}` (`artifact_handlers.go:185`, inline auth `:222-227`), `/versions`
    (`:437`), `/download` (`:1106`), `/embed*` (`:946`) and `/document` (`:1080`) call
    `sealOfArtifact` and admit K and SELF only.
  - The insecure public viewer (`server.go:1649`, `artifact_handlers.go:205`) refuses a sealed
    lane's artifact for everyone.
- **Listings: a GORM default scope hides sealed rows on every read (SF3, round 10).** Round 9's
  per-function filter was not enough. `Store.UnifiedSearch` (`store/store_prompt_history.go:759`,
  `GET /search?q=`) runs its own owner-scoped ILIKE over task `name` / `original_prompt` and over
  sessions. `ListSessionsByOwner` (`store/store_sessions.go:589`), `ListPromptHistoryBySpecTask`
  (`store/store_prompt_history.go:307`) and `ListSpecTaskGitPushEvents`
  (`store/spec_task_design_review_store.go:242`) were also outside it. So the exclusion moves from
  functions to **models**:
  - **Filter column.** `sandbox_sealed bool not null default false`, written **on create only**
    (GORM `<-:create`, never updated), on every table that holds lane content:
    - `spec_tasks`, `sessions`, `interactions`, `llm_calls`, `prompt_history_entries`;
    - `spec_task_git_push_events`, `artifacts`, `spec_task_design_reviews` and their comments;
    - `spec_task_zed_threads`, `spec_task_work_sessions`.

    The store sets it from the owning task's seal at insert. Sealed sessions are created in the
    sealed task's own transaction (S7), so the value is right from the first write. The LLM-call
    logger sets it from the session's seal (cached; `true` is immutable). Each column has a partial
    index `WHERE sandbox_sealed`.
  - **Default scope.** A GORM callback is registered on `Query`, `Row` and the `Count` path, before
    `gorm:query`. For any statement whose schema table is in that list, it appends
    `<table>.sandbox_sealed = false`, **unless** the context carries one of:
    - `withSealVisibility(ctx, keyID)`, set only by `sealDecision` when the caller is K or SELF of
      that task. It widens to `sandbox_sealed = false OR spec_task_id IN (tasks sealed by keyID)`;
      the lane's own session sees only its task.
    - `withPlatformCaller(ctx)`: orchestrator, reapers, the provider-recovery readers
      (`websocket_external_agent_sync.go:4559`, `server/provider_recovery.go:16`), and the gate-3
      delivery path.

    `UnifiedSearch`, `ListSessionsByOwner`, the 33 `ListSpecTasks` / `ListSessions` call sites, the
    batch `/projects/{id}/tasks-progress` / `/tasks-usage` (`server.go:1709-1710`), clone-group
    progress, the org-runtime `List` (`spectasks.go:314`) and every future query are covered without
    naming them.
  - **Why a callback rather than a Postgres view.** A view used as the model's table would also
    carry inserts and updates, so every write path would need a separate base-table model. That
    reintroduces the per-call-site opt-in the review wants gone. The callback applies only to
    reads, which is the right side for a filter.
  - **Raw SQL.** Callbacks do not see `.Raw` / `.Exec`; the store has 36 of them today. A store
    test lists every `Raw` / `Exec` whose SQL names a listed table. Each must be on an allowlist
    **and** contain `sandbox_sealed`, or be a write. A new one fails the test.
  - **Primary-key reads are scoped too.** `First` / `Take` by id are scoped, so a sealed object
    looked up by id is "not found" unless the context opts in.
    - `sealOf` and the gate lookups read seal state through an internal, platform-marked context, so
      they always see the row they are deciding about.
    - After an `allowed` decision, the gates attach `withSealVisibility` to the request context, so
      the handler's own reads succeed.
    - Everyone else gets 404 from the store, which matches §3.3's "outside the tenant" answer.

Gate 4 records go into the same access record with `scope: task` and the route key: `#git:<service>`
plus the ref, or the artifact id.

### 3.11 Per-sealed-task bare repositories (NB1 structural fix; also NB2, SF1, SF2)

**Layout.** For each sealed task T and each repo R in T's project that T works on (at least the
primary repo):

```
/filestore/git-repositories/sealed/<T>/<R>.git          bare repo for T's copy of R
/filestore/git-repositories/sealed/<T>/<R>.git/objects/info/alternates
    -> ../../../../<R>/objects                           RELATIVE path (S9-2); one-way: sealed reads shared
```

- **Tracking.**
  `spec_task_sealed_repos(spec_task_id, repo_id, path, state, created_at, ready_at, detached_at)`,
  where `state` ∈ `pending | ready | failed | detached`.
  - There is **no** `GitRepository` row, so sealed repos never appear in repo listings, `Branches`,
    status counts, Kodit registration or external sync.
  - `path` is the only source of a sealed repo's location. **It is never built from URL segments**
    (S9-1).
- **Creation order (S9-1).** Disk state can't join a database transaction, so creation runs in four
  steps:
  1. The seal transaction (§3.1) inserts each `spec_task_sealed_repos` row as `pending`, with
     `path` computed by the server.
  2. After commit, `git init --bare`, then write the alternates file and the hook. Then the
     base-ref fetch.
  3. Mark the row `ready`.
  4. **The lane's desktop is not started until every row for T is `ready`.** A reconciler (on start,
     and every 5 minutes, `withPlatformCaller`) retries `pending` rows. After 3 failures it marks
     them `failed`, which stops the lane visibly and writes a `kind: lifecycle` record.
- **Base refs.** At creation, and whenever the platform refreshes the lane's base, the sealed repo
  fetches the shared repo's base branches into `refs/heads/<base>`, read-only for the lane. It runs
  `git fetch <shared> +refs/heads/<base>:refs/heads/<base>` inside the sealed repo, marked
  `withPlatformCaller`. This only reads from the shared repo; no command ever runs in the shared
  repo against the sealed one.
- **Hook.** The sealed repo's pre-receive hook allows updates only under
  `refs/heads/helix-sealed/<T>/`, only when `HELIX_SEAL_SELF_TASK=<T>` is set by the server for T's
  own session key. A missing or wrong value is a denial. This closes the round-8 S1 finding that an
  unset `HELIX_ALLOWED_BRANCHES` means allow-all (`services/gitea_git_helpers.go:442-455`).

**Routing.** `GitHTTPServer.getRepoPath` (`services/git_http_server.go:420`) is the single place a
git request becomes a filesystem path. It consults `sealedRepoFor(user, repoID)`.

**Routing fails closed (S9-1):**

- If `sealedRepoFor` returns an error, or a `ready` row's path is missing on disk, the request
  gets 503 `sealed_repo_unavailable`. It **never** falls back to the shared repo.
- A key bound to a sealed task (its session key, or K) that arrives on a shared `/git/{R}` path
  gets **403** before upload-pack or receive-pack runs.
- The K route, `/git/sealed/{T}/{R}`, checks `spec_task_seals(T).sealed_by_key_id == APIKeyID(key)`
  **and** that a `ready` `(T, R)` row exists. A forged `T`, or a `T` that K didn't seal, gets 404.
  `T` and `R` are only lookup keys; the filesystem path comes from the row.
- **Explicit registration (nit).** `/git/sealed/{T}/{R}` is registered as its own route in
  `GitHTTPServer.RegisterRoutes` (`services/git_http_server.go:195`), ahead of `/git/{repo}`. The
  repo id `sealed` is reserved: repository create and rename refuse it.
- **Repo-row shape for the sealed branch (NB2 gap).** `getRepoPath` returns the path plus a
  `*types.GitRepository` that `handleReceivePack` reads. For a sealed path it returns a synthetic
  row with `ExternalURL = ""`, `DefaultBranch = <T's base>`, no web-service deploy hook and
  `IsExternal = false`. So all three upstream paths are skipped by construction:
  - the pre-receive upstream sync (`services/git_http_server.go:703`);
  - the post-receive upstream push (`:867`);
  - `onDefaultBranchPush` (`:795-801`).

  A test asserts the upstream mock and the deploy hook see no call for a sealed push.
- **Lock key (nit).** `GetRepoLock(repoID)` (`services/git_repository_service.go:142`) is keyed by
  repo id, so a sealed push would serialize with the shared repo's operations. That is safe, but it
  needlessly couples the lane to shared-repo traffic. Round 10 keys the lock by **path**
  (`WithRepoLock(path, …)`). The base-ref refresh takes **both** locks in a fixed order (shared,
  then sealed), so a shared-repo rewind can't interleave with a sealed base fetch.
- **Filestore (nit).** The `/filestore/*` routes (`server.go:1155-1163`) serve a separate root, but a
  test asserts that no filestore path, including `..` and symlink tricks, resolves into
  `git-repositories/sealed/`.

| caller | `/git/{repoID}` resolves to |
|---|---|
| T's own session key (SELF) | T's sealed copy of R. A repo outside T's set is refused. The lane never touches the shared repo through git. |
| K, via `/git/sealed/{T}/{repoID}` (a separate URL, allow-listed for K and SELF) | T's sealed copy, **fetch only** |
| anyone else | the shared repo, which contains none of T's objects or refs |

**How this closes each finding:**

- **NB1.** A v0 or v2 fetch from the shared repo cannot return objects the shared repo doesn't
  have.
  - A non-SELF push cannot create a shared ref at a sealed SHA, because git would require the
    object to be uploaded, and only someone who already has it can upload it.
  - The read API and branch create run against the shared repo, so a sealed SHA argument resolves
    to "object not found".
- **NB2.** Every user and platform push or pull path addresses a repo id, and therefore the shared
  repo. None of them can name, read or overwrite a sealed ref, and the sealed repo has no upstream
  to exfiltrate to. A changed `ExternalURL` only affects the shared repo.
- **SF1.** The reserved prefix exists only inside sealed repos; in the shared repo it is refused
  for everyone. That gives a prefix `hideRefs`-style guarantee without a growing ref list, and no
  add-ref race: the lane's refs are created inside its own repo in the creation transaction.
- **SF2.** `GitRepository.Branches` is filled from the shared repo, so sealed names never get there.

**Garbage collection (S9-2).** Cyber verified that `gc --prune=now` on the shared repo after an
upstream rewind corrupts a sealed copy, and that `gc.pruneExpire=never` prevents it.

- **Real triggers.** Helix never calls gc itself. What actually runs it is `receive.autogc` after
  pushes and git's auto-maintenance after fetches (`SyncAllBranches`, `PullFromRemote`).
- **Rule.** A shared repo's `gc.pruneExpire` is **derived from a DB refcount**: the number of
  `spec_task_sealed_repos` rows for R in `pending` or `ready`.
  - It is set to `never` when the count goes 0 → 1, and restored when it goes back to 0.
  - Both changes happen under R's repo lock, in the same code path that inserts or detaches the row.
  - A startup reconciler rewrites the config from the refcount, so a crash between the two can't
    leave pruning enabled.
- **Forbidden on repos with sealed dependents:** `git prune`, and `git repack -a -d` without `-A` or
  `--cruft`. Helix code that runs maintenance goes through one `MaintainRepo(path)` helper, which
  checks the refcount. An AST test forbids direct `prune` / `repack` / `gc` invocations elsewhere.
- **When T is archived:** T's sealed repos run `git repack -a -d` to copy every alternate object
  locally (fsck clean, per cyber's experiment). Then they drop the alternates file, record
  `detached_at` and decrement the refcount. They stay readable by K from the tombstone (§4), subject
  to the disk rules below.
- **Backups.** They must be **filesystem-level** (snapshot or rsync of `git-repositories/`
  **including** `sealed/`, in one consistent snapshot with the shared repos). Clone- or bundle-based
  backups of a sealed repo silently drop the borrowed objects. A restore of `sealed/` without the
  matching shared repos is invalid. The backup runbook states both.

**Disk (S9-3).**

- **Push size.** Sealed repos set `receive.maxInputSize` (default 512 MiB per push). A per-task quota
  (default 5 GiB on-disk, configurable per org) is checked in the Go receive-pack pre-check (SF4). A
  lane over quota gets its push refused, with a `denied` record (`reason: quota`), and the portal
  sees it.
- **Archive form.** Instead of `repack -a -d` of the full base history into every tombstone, the
  archived lane is reduced to a **bundle of `base..work` (and `base..specs`) plus the base SHA**.
  The bundle is kept as the tombstone's git evidence, and the sealed repo directory is removed.
  - K can still verify the lane's commits by unbundling against any repo that has the base SHA.
  - The full `repack -a -d` path is kept only as a fallback when the base SHA is no longer
    reachable in the shared repo; that is recorded as `kind: lifecycle`, `reason: base_unreachable`.
- **Retention.** Tombstone bundles have a TTL, `HELIX_SEAL_TOMBSTONE_TTL`, default 365 days,
  configurable. The `spec_task_seals` row and the access events are kept indefinitely; they are
  small.
- **Shared-repo growth.** While a shared repo has sealed dependents, a metric reports its on-disk
  size. An alert fires on growth above a threshold (default +20% or 5 GiB since the first seal),
  because `pruneExpire=never` keeps upstream garbage until the last lane detaches.

**Post-push processing.** The handlers that today read the shared repo after an agent push must
take the sealed repo path for sealed tasks:

- `handlePostPushHook` (`services/git_http_server.go:1260`);
- `handleFeatureBranchPush` (`:1300`);
- `processDesignDocsForBranch` (`:1861`);
- `createDesignReviewForPush` (`:2019`);
- `checkCommentResolution` (`:2134`).

None of them may push upstream for a sealed path.

**Interim minimum (kept in slice 1 as defence in depth).** These stay even with per-task repos, so a
mistake in routing is not by itself a leak:

1. **Force protocol v0** for fetches by anyone but K and SELF on a repo that has a sealed
   dependent. The server drops `GIT_PROTOCOL` (today forwarded from `Git-Protocol` at
   `services/git_http_server.go:742-744`), so `uploadpack.hideRefs` plus `allow*SHA1InWant=false`
   apply as tested.
2. **Refuse unreachable SHAs.** Every SHA argument on the read API, branch create, PR create and
   `/contents` must be reachable from a non-reserved ref of the shared repo (`git for-each-ref
   --contains <sha> --exclude=refs/heads/helix-sealed/`); otherwise the call is denied.
3. **Run the v2 test.** A protocol-v2 fetch by SHA of a commit that exists only in a sealed repo must
   fail against the shared repo URL, and also against a mis-routed URL in a fault-injection test.

**Receive-pack ordering (SF4, and the S3 dry run).**

- Today `handleReceivePack` runs `SyncAllBranches(force=true)` from upstream **before** receive-pack
  (`services/git_http_server.go:~703`). It also lets the hook be the only ref check.
- Round 9 parses the pushed ref-update commands in Go first. It applies `sealedRepoFor`, the
  reserved-prefix rule and the seal decision, and only then runs any sync or receive-pack.
- The hook stays as a second layer.
- A sealed repo never syncs from upstream at all, so the dry-run probe (`X-Helix-Seal-Probe`)
  returns after the Go check, with no side effect.

### 3.12 Project and repository lifecycle (NB5, closed in round 10)

Round 9 guarded four handlers. The same destruction is reachable from other callers, so round 10
moves the guard into the **three choke points** every destructive path goes through, and proves
with an AST test that nothing bypasses them.

**Choke points:**

| choke point | file:line | callers today (all now guarded) |
|---|---|---|
| `GitRepositoryService.DeleteRepository` | `services/git_repository_service.go:1084` | `DELETE /git/repositories/{id}` (`git_repository_handlers.go:397`); `DELETE /organizations/{id}` (`organization_handlers.go:433` → `:469`); admin user delete (`admin_user_delete.go:80`); project delete cleanup (`project_delete_cleanup.go:84`) |
| `HelixAPIServer.purgeProject` | `project_handlers.go:1075` | `DELETE /projects/{id}` (`project_handlers.go:1019`); `DELETE /organizations/{id}`; admin user delete (`admin_user_delete.go:67`) |
| `Store.DeleteProject` | `store/store_projects.go:244` (interface `store/store.go:842`) | `purgeProject`; org-runtime teardown (`org/application/lifecycle/lifecycle.go:582` → `server/helix_org_inproc.go:859`); any future caller |

**Rule.** Each choke point first checks `sealedTaskLive(repoID | projectID)`:

- the check covers any `spec_task_seals` row whose task is not archived and whose project is the
  target, or whose `spec_task_sealed_repos` / primary repo is the target;
- it runs inside the same transaction as the delete, so a lane sealed concurrently can't slip in.

If a lane is live:

- **Without an owner-adopted org-owner stop:** return `ErrSealedTaskLive`, surfaced as **409
  `sealed_task_live`**. Bulk callers (org delete, admin user delete, org-runtime teardown) fail as a
  whole, before any deletion: they pre-check every repo and project they will touch, then start. So
  a half-deleted org can't happen. Each refused attempt writes one `outcome: denied`, `kind: lifecycle`
  record per affected lane, with the caller's `actor_class` and route.
- **With org-owner stop adopted (owner decision, §4):** the choke point stops the lane, records
  `actor_class: org_owner_stop`, writes `task_deleted_at` / `task_deleted_by_key_id` on the
  tombstone, repacks or bundles the sealed repos (§3.11), and only then deletes. Deleting the
  shared repo first would corrupt the sealed copies that borrow from it, so the order is mandatory.

**AST test (no bypass).** In packages `server`, `services`, `store` and `org/...`:

- `os.RemoveAll` / `os.Remove` on a path derived from the git-repository root;
- `Store.DeleteProject`;
- `gitRepositoryService.DeleteRepository`;
- a GORM `Delete` on `types.Project` or `types.GitRepository`.

These may appear only inside the three choke points, or in a reviewed allowlist (tests, migrations).
A new caller of the store method is fine, because the check lives inside it. A new **direct**
deletion is a test failure.

**Repository re-attachment and base changes (all classified):**

| route | handler | decision |
|---|---|---|
| `PUT /projects/{id}/repositories/{repo_id}/detach` | `project_handlers.go:1418` (`server.go:1687`) | **409** `sealed_task_live` if the repo is a live lane's repo |
| `POST /projects/{id}/move` | `project_handlers.go:2310` (`server.go:1693`) | **409** while the project has a live lane |
| `PUT /projects/{id}/repositories/{repo_id}/attach` | `server.go:1686` → `attachRepositoryToProject` | allowed. A live lane's repo set is fixed at seal time (`spec_task_sealed_repos`), so attaching a new repo doesn't change what the lane sees. Recorded as `outcome: allowed`, `kind: access` on every live lane in the project. |
| `PUT /projects/{id}/repositories/{repo_id}/primary` | `server.go:1685` → `setProjectPrimaryRepository` | **409** while the project has a live lane. The primary repo determines the lane's base and its workspace setup on any platform restart. |
| `PUT /git/repositories/{id}` (`ExternalURL`, default branch) | `git_repository_handlers.go:308` (`server.go:1814`) | allowed, and recorded on every live lane referencing the repo (round 9). With per-task repos it can't reach the lane's refs. |

**Tombstone.** Whenever a sealed task is deleted (K's delete, a choke-point delete after org-owner
stop, an operator purge), `task_deleted_at` and `task_deleted_by_key_id` are written on the
tombstone first. A delete that cannot write the tombstone fails.

### 3.13 Shared-repo readers of `task.BranchName` (S9-4)

Code that reads the **shared** repo by repo id plus `task.BranchName` fails safe for a sealed task,
because the branch doesn't exist there. But it must not error-loop. Each one is explicitly **skipped
for sealed tasks**, checked through `sealOf(task)` at entry, with the reason in a debug log:

| site | function | for a sealed task |
|---|---|---|
| `server/spec_task_workflow_handlers.go:220` | `approveImplementation` (fast-forward merge) | not reached: the route is DENY (§3.10). The function also returns early. |
| `server/spec_task_workflow_handlers.go:895-900` | `mergeInternalRepoBranch` | skip |
| `services/git_http_server.go:1467` | `tryAutoMergeAfterRebase` | skip (§3.10, no auto-merge) |
| `services/git_http_server.go:1598` | `tryAutoMergeBotRun` | skip |
| `services/spec_task_orchestrator.go:1593` | `checkTaskForExternalPRActivity` | skip. A sealed lane has no PR; the orchestrator must not retry. |
| `server/session_workspace_handlers.go:65` | `resolveExpectedBranch` | returns `helix-sealed/<T>/work`. Only the lane's own workspace calls it, and that resolves to the sealed repo. |

A test drives the orchestrator over a sealed task for N ticks and asserts these functions log one
skip each and raise no errors.

**K's git reads are fetch-only (S9-4).** K reads the lane's git state by fetching
`/git/sealed/{T}/{R}`.

- The git read API (tree, commits, contents, file-content, grep) runs against the **shared** repo.
  For `ref=helix-sealed/T/work` it returns **not-found by design**, for K too.
- A K-facing read API over sealed repos is deliberately not offered. It would be a second read path
  to keep sealed.

### 3.14 LLM call logs and app-owner session reads (N9-1, closed in round 10)

**The gap.** The LLM-call log holds the full `Request`, `Response` and `OriginalRequest` of every
model call. Rows carry the lane's `SessionID` and `AppID`; the proxy logger writes them at
`openai/logger/openai_logger.go:~411`. They are readable by session filter:

| route | handler | who passes today |
|---|---|---|
| `GET /apps/{id}/llm-calls?session=<sid>` | `agent_routes.go:23` → `llm_calls_handlers.go:79` (filter read `:113`) | the agent app's owner, or an admin |
| `GET /admin/llm_calls?session=<sid>` | `server.go:1548` → `llm_calls_handlers.go:25` (filter `:37`) | any admin |
| `GET /apps/{id}/interactions?session=<sid>` | `agent_routes.go:24` → `llm_calls_handlers.go:157` (filter `:192`) | the app owner, or an admin: the lane's interactions |
| `GET /apps/{id}/step-info?interactionId=<iid>` | `agent_routes.go:25` → `llm_calls_handlers.go:250` (`:254`) | the app owner, or an admin |

The agent app's owner is on the tenant side, so under option (a) this is a full read of the lane.
The route-policy test missed it, because the session id arrives as a **query filter**, not through
an id resolver.

**The fix:**

- **The default scope covers `llm_calls`, `interactions` and step info** (§3.10). With no seal
  visibility in the context, `ListLLMCalls` (`store/store_llm_calls.go:38`), the interaction lists
  and the step-info reads return no rows of a sealed session, whatever the filter. That alone closes
  the admin and app-owner paths.
- **The routes are explicitly DENY for sealed sessions.** When the `session` / `interactionId`
  filter resolves to a sealed session, the handler calls `sealDecision`. It answers
  `403 sandbox_sealed` (not an empty list) and writes a `denied` record, so the attempt is
  visible. That applies to K too: K reads the lane through the session routes in its read set,
  not through app-wide logs. Unfiltered listings simply omit sealed rows.
- **K's usage read strips the bodies.** `GET /sessions/{id}/usage` (`session_usage_handlers.go:40`)
  is in K's read set for token and cost totals. For a sealed session it returns the call
  metadata (model, tokens, cost, timing, status) and **omits** `Request`, `Response` and
  `OriginalRequest`. K reads the transcript through `/interactions`, which carries provenance.
- **Platform readers keep working.** `recentProviderFailure`
  (`websocket_external_agent_sync.go:4559`) and `server/provider_recovery.go:16` run with
  `withPlatformCaller`.
- **Route-policy rule 7 (query filters).** Any route that reads a query parameter naming a session,
  interaction or task must declare `.seal(...)`, and its handler must resolve the parameter through
  `sealDecision` before the first store read. The parameter names are `session`, `session_id`,
  `sessionId`, `interaction`, `interaction_id`, `interactionId`, `spec_task_id`, `specTaskId`, `task`
  and `task_id`.
  - The AST test finds `r.URL.Query().Get("<name>")` and struct-tag-bound query structs in the
    handler and everything it reaches.
  - It also flags any `ListXxxQuery{SessionID: …}` / `{SpecTaskID: …}` / `{InteractionID: …}`
    literal whose value comes from the request.

## 4. Break-glass and the access log

**Recommendation: ship both.** The access log comes first and is the more important of the two.

### Access log

Add a table, `spec_task_sandbox_access_events`, written by the server and append-only.

**Columns:**

| column | values / notes |
|---|---|
| `id`, `created` | |
| `spec_task_id` | **always set.** It is resolved from the task, session, sandbox row, interaction or work session. |
| `session_id`, `sandbox_id` | nullable. Set when the route resolved one. |
| `scope` | `task`, `session` or `sandbox`: which kind of id the route addressed |
| `outcome` | **`allowed` or `denied`**. Required on every row. |
| `kind` | `access` (a route call), `turn` (a gate-3 sink decision), `lifecycle` (an allowed stop/archive/delete, and the task deletion itself), `config_drift`, `break_glass` |
| `actor_class` | **who, in seal terms**: `sealing_key`, `self`, `self_runner`, `platform`, `org_owner_stop`, `admin_break_glass`, `sealing_user_recovery`, `other`. This is what keeps the cyber requirement satisfiable: an org-owner stop or an admin break-glass, if adopted, is an `allowed` row whose `actor_class` can never be confused with K's `sealing_key` stop or delete. |
| `reason` | `sealing_key`, `sealing_key_read`, `self`, `self_runner`, `platform`, `not_allowlisted`, `no_route`, `seal_state_unavailable`, `seal_version_unknown`, `turn_provenance_denied`, `seal_broken`, `drift`, `break_glass` |
| `actor_user_id`, `credential_kind` (browser, personal_key, org_key, session_key, runner, admin), `api_key_id` | `api_key_id` is the non-secret `key_…` id |
| `credential_fingerprint`, `client_ip_hash`, `user_agent_hash`, `auth_session_id` | S4: separates a stolen copy of K from the portal (§3.7). The fingerprint is the request audit's HMAC (`types/request_audit.go:33-37`). |
| `route` | the route key (§3.3): `METHOD template` with any `#server` / `#tool` / `#git:<service>` suffix, the sink name for gate 3, or `ref` / `artifact_id` for gate 4 |
| `probe` | `true` when sent with `X-Helix-Seal-Probe: 1` and honoured (§3.7) |
| `count`, `first_seen`, `last_seen` | used only by deduplicated rows (see Volume) |
| `status` | the HTTP status returned |

**Scope.** The record covers all three route families, not only routes that touch the sandbox:

- **task-level routes**, i.e. anything resolving `/spec-tasks/{taskId|spec_task_id|id}` (execution
  config, approvals, design reviews, attachments, PR decisions, zed-instance, the seal read, stop,
  archive, delete), plus the body-addressed task routes in §1f;
- **session-level routes:** `/sessions/{id}/…`, `/external-agents/{sessionID}/…`, `/interactions/…`,
  the MCP backends, sync and RevDial;
- **sandbox-level routes:** `/organizations/{org}/sandboxes/{id}/…` on session-backed rows;
- **ref- and artifact-addressed lane state** (gate 4, §3.10): git push and fetch on sealed refs,
  `/contents`, PR routes on both APIs, artifact reads and writes, all with `scope: task`;
- **org-runtime tools** (§3.9), `scope: task`, route `mcp:helix-tasks:<tool>`.

`sealDecision` writes one row per decision, whichever gate called it, so all three families land in
the same table keyed by `spec_task_id`.

**A denied attempt is not a breach.**

- `outcome: denied` means the seal *held*. The UI shows such rows as "blocked attempts", and they
  never change the task's seal state.
- `outcome: allowed` rows are expected, and are attributed by `reason`: K's stop or delete, the
  sandbox's own traffic, or platform work.
- The **only** event that changes the seal state is `kind: break_glass`, which sets
  `seal_broken_at` (§ Break-glass).
- **After a break, every access is still recorded (S6):** `outcome: allowed`,
  `reason: seal_broken`, with the full actor fields. `sealDecision` never returns early on a broken
  seal (§3.3 step 2).
- `GET /spec-tasks/{id}/seal` reports `seal_broken_at`; it is not derived from counting rows. So a
  verifier reading the record can tell three states apart: intact and untouched, intact with
  blocked attempts, and broken.

**Volume.**

- On sealed tasks every decision is written, `allowed` and `denied` alike.
- **Only allowed `self` / `self_runner` / `platform` rows are deduplicated:** one row per route,
  credential and hour, with `count`, `first_seen` and `last_seen`. That covers sync reconnects and
  the 30s config polls, which would otherwise fill the record.
- **Never deduplicated (N5):** any `outcome: denied` row, any row with `actor_class` `sealing_key`,
  `org_owner_stop`, `admin_break_glass`, `sealing_user_recovery` or `other`, and any
  `kind: lifecycle` / `break_glass` / `config_drift` row. Each of those is one row per event.
- On unsealed tasks, only non-read operations are written.
- **Precedence (S6).** A task with a broken seal is still a sealed task. "Record every access after
  a break" (`reason: seal_broken`, reads included) wins over "unsealed tasks log only non-reads".

- **Written** synchronously by `sealDecision` and the gate-3 sinks. A failed write fails the
  request closed.
- **Exposed** as `GET /spec-tasks/{id}/sandbox-access`: allow-listed for K on sealed tasks, T(Get)
  on unsealed ones. Filters: `outcome`, `kind`, `scope`, `since`. Also shown as a panel on the
  task. The portal and verifier read it alongside `/interactions`.
- **Kept beyond the task (S6).** Neither `spec_task_sandbox_access_events` nor `spec_task_seals`
  has a cascading foreign key to `spec_tasks`, so they are never purged by the audit retention job
  or by task deletion.
  - Deleting the task writes `task_deleted_at` / `task_deleted_by_key_id` on the seal row, and a
    `kind: lifecycle` event with the actor.
  - After deletion, K can still read `GET /spec-tasks/{id}/seal` and `/sandbox-access` from the
    tombstone. These two routes resolve from `spec_task_seals`, not from `spec_tasks`.
  - So an owner or admin who breaks the seal and then deletes the task cannot erase the trail. The
    break, every access after it, and the deletion all remain readable by K.
- **Why not reuse the request audit log:**
  - It is admin-only and global, with no session column.
  - Its recorder **drops events when its buffer is full** (`api/pkg/auditlog/recorder.go:55`).
  - Its retention is 180 days (`config.go:1379`).

  An evidentiary record must not drop rows. Keep the request audit as the second, independent
  trail.

### Break-glass

Add `POST /spec-tasks/{id}/break-seal`.

- It is on `adminRouter`, needs a reason, and is not reversible.
- It sets `SealBrokenAt`, `SealBrokenBy` and `SealBrokenReason`, writes a `break_glass` event, and
  publishes a `seal_broken` state shown on the task and in the API.
- After that the task is treated as **unsealed** under normal authz. It never becomes "sealed with
  an exception", so no reader can mistake a broken seal for an intact one.

**Why keep it:** without an API path, operators who need to debug a stuck verifier will edit the
database or `docker exec` on the host, which leaves no trace in Helix. A loud, irreversible
break-glass is strictly better than that.

**Why not an exemption:** a per-request admin bypass is exactly the out-of-band access the seal
exists to rule out, and it would be invisible to the verifier.

**What it cannot catch:** host or database access by the platform operator. State that limit in the
portal (see the Goal section).

**Owner decision: still open** for both break-glass and an org-owner stop.

**Cyber requirement: how either is recorded, if adopted.** This holds whichever way the owner
decides.

- **Break-glass** writes one row: `kind: break_glass`, `outcome: allowed`,
  `actor_class: admin_break_glass`, `reason: break_glass`, with the admin's identity, the stated
  reason and the request fingerprint.
  - In the same transaction it sets `spec_task_seals.broken_at` / `broken_by` / `broken_reason`, so
    `seal_broken_at` is set in `GET /seal`.
  - Every later access is recorded with `reason: seal_broken` (§ Access log).
- **An org-owner stop** writes `kind: lifecycle`, `outcome: allowed`,
  `actor_class: org_owner_stop`, `reason: org_owner_stop`. It does **not** set `seal_broken_at`: a
  stop gives no content access.
- K's stop and delete are `actor_class: sealing_key`. A reader filtering on `actor_class` can always
  tell the three apart, and none of them is ever deduplicated (N5).

---

## 5. Implementation plan

Three slices, each a separate PR. **`seal_version` advances with each slice** (§3.8). The portal
must not run production verifier lanes until `seal_version` ≥ 3, i.e. until every slice has
shipped, and `min_cluster_version_since_seal` must stay ≥ 3 for the whole run. Earlier versions are
for integration testing only.

**Slice 1 (`seal_version` 1): default-deny, per-task git isolation, org tools, key binding, access
record, destructive choke points, LLM-log closure.** About 25-27 days.

1. **Model** (1d).
   - Add `spec_task_seals` (no cascade), `SpecTask.SandboxSealed`, `SpecTask.CreatedByKeyID` and
     `spec_task_sealed_repos`.
   - One transaction creates the task, the seal row, `sealed_config` and the sealed repos (S7).
   - Immutability; forced confinement; `just_do_it_mode`; forced `AutoApprovePullRequests = false`
     (NB3); every create precondition applied on clone (nit); refuse clone of a sealed task (N1);
     the create echo (S5).
   - OpenAPI (`./stack update_openapi`); the frontend create option and a badge.
2. **Sealing key type** (1d, NB4, S4). `APIkeytypeSealing`, shown once and prefix-only afterwards;
   excluded from `isPersonalAPIKey`, from the no-`types` `GET /api_keys` branch and from
   `getOrCreateUserAPIKey`; refuse revoking a live sealing key; the recovery path; the Kodit
   registration check.
3. **Decision function** (1d).
   - Route-key stamping (`#server`, `#tool`, `#git:<service>`).
   - `withPlatformCaller` marking and `no_route` denial (S1).
   - The uncached `broken_at` read (N6).
   - The handler-keyed allow-list (N3).
   - `sealDecision` with the synchronous access-record write and fingerprints.
4. **Gates 1 and 2** (1.5d). The path guard; the resolver gate in every helper in §3.3, including
   the share-* vhost (N2); the stream forced read-only; delete `POST /external-agents/{sid}/command`
   and `configure-pending-session`.
5. **Org-runtime tools** (0.5d, B1). The seal check in `ownedTask` for the 13 per-task tools.
6. **Per-sealed-task bare repos** (5d, NB1/NB2/SF1/SF2, plus round 10 S9-1, the NB2 gap and the
   nits).
   - Fail-closed routing; the pending → ready creation order with its reconciler; the explicit
     `/git/sealed/{T}/{R}` route; the reserved repo id `sealed`; the synthetic repo row with
     `ExternalURL = ""` and no deploy hook; the repo lock keyed by path; the filestore traversal
     test.
   - Layout, alternates and creation; `sealedRepoFor` in `getRepoPath`; the `/git/sealed/{T}/{R}`
     fetch URL for K.
   - Base-ref refresh; the sealed-repo hook; post-push handlers taking the sealed path; the specs
     ref under `helix-sealed/<T>/specs`.
   - The gc policy (`gc.pruneExpire=never` while a sealed dependent is live) and
     repack-and-detach at archive.
7. **Shared-repo hardening and choke points** (2d, NB1 interim, NB2, SF1, SF4).
   - The reserved-prefix refusal in the shared hook, independent of env.
   - Negative fetch refspecs.
   - Guards in `PushBranchToRemote`, `PushToOrigin`, `PushToHelixBare`, `PullFromRemote`,
     `SyncAllBranches`, `CreatePullRequest` and merge.
   - The Go-side receive-pack command check before any upstream sync.
   - **Interim minimum:** force protocol v0 for non-K/non-SELF fetches on repos with a sealed
     dependent; refuse unreachable SHAs; the v2-by-SHA test.
8. **PRs** (0.5d, NB3). `propose_pull_request` and `pr-proposals*` DENY for sealed tasks;
   `auto_approve_future` refused; `pr_auto_approve` in the gate-3 deny list.
9. **Lifecycle choke points** (2d, NB5 closed in round 10).
   - The `sealedTaskLive` check inside `DeleteRepository`, `purgeProject` and `Store.DeleteProject`,
     with bulk-caller pre-checks (org delete, admin user delete, org-runtime teardown).
   - The no-bypass AST test.
   - 409 on detach, move and set-primary; recorded attach.
   - Tombstone-first deletion.
9a. **gc, backup and disk** (2d, S9-2, S9-3). Refcount-derived `gc.pruneExpire`; the
   `MaintainRepo` helper and its AST test; relative alternates; the backup runbook;
   `receive.maxInputSize` and the per-task quota; bundle-based archive with a TTL; the shared-repo
   growth alert.
9b. **LLM call logs and app-owner reads** (1.5d, N9-1). DENY for sealed sessions on
   `/apps/{id}/llm-calls`, `/admin/llm_calls`, `/apps/{id}/interactions` and `/apps/{id}/step-info`;
   body-stripped `/sessions/{id}/usage` for K; route-policy rule 7 (query filters).
9c. **Shared-repo branch readers** (0.5d, S9-4). Skip the six listed sites for sealed tasks, with
   the orchestrator no-error-loop test.
10. **Route-policy test and seal-policy** (2d, S2, S3, SF7). `.seal(...)` on every
    `noAuthzHelper` / `handlerAuth` route, every MCP backend and every tool; the extended AST walk;
    `dataCallBeforeHelper` extended to the seal call; the ref/SHA-parameter assertion; hook tests
    with an empty env; seal-policy with `seal_version`, `cluster_min_seal_version` and `id_in`; the
    dry-run probe.
11. **Seal read, access record and tombstone** (1d, S6, N5).
12. **Cluster version** (1d, S5, SF5). The heartbeat table; the create gate;
    `min_cluster_version_since_seal` and `cluster_gap_seen_at`; the `deploy-prod.sh` rollback guard.
13. **Break-glass** (0.5d), only if the owner adopts it. It is recorded as in §4 either way.

**Slice 2 (`seal_version` 2): portal read option (a), outside git.** About 5-6 days.

14. **GORM default scope** (3d, SF3 closed in round 10).
    - Add the create-only `sandbox_sealed` column on the listed tables, plus a backfill migration.
    - Register the query/row/count callback.
    - Add the `withSealVisibility` / `withPlatformCaller` opt-ins at the gates and at platform entry
      points.
    - Add the raw-SQL allowlist test.
    - Run `UnifiedSearch`, `ListSessionsByOwner`, `ListPromptHistoryBySpecTask` and
      `ListSpecTaskGitPushEvents` through the scope, with route-level tests.
15. **Artifact reads** (1d). `sealOfArtifact` on every artifact read path and on the public viewer.
16. **Kodit service credential** (1-1.5d, NB4). Kodit clones with a service credential, never a
    user key. Sealed repos are never registered with Kodit.
17. **Defensive filters** (0.5d, SF2). Drop the reserved prefix from `GitRepository.Branches`,
    repo status counts, and branch and PR listings.

**Slice 3 (`seal_version` 3): turn sink, config freeze, guidelines.** About 5 days.

18. **Gate 3** (1.5d). Provenance-based refusal at enqueue and at delivery
    (`sendQueuedPromptToSession`, `ReconcileStuckSendingPrompts` at `server.go:850`, reconnect
    resume); status-only `ci_status`; `pr_auto_approve` denied.
19. **Config freeze** (2d). `sealed_config` served by `getZedConfig` and by platform restarts; fail
    closed on a null config; drift hashing; drop `config_changed`.
20. **`guidelinesFor`** (1d, S8, SF6). One composer for all three call sites, frozen for sealed
    tasks, with the AST assertion.
21. **Provenance for cancel and questions** (0.5-1d). Makes cancel *eligible* for the allow-list
    (§3.6).

**Tests, the probe harness and live verification:** about 6 days, spread across the slices.

**Total: about 41-44 engineer-days**, up from 33-36 in round 9 (slice 1 25-27, slice 2 5-6, slice 3 5, tests 6).

- **The two blockers, about +3 days:** NB5 choke points and AST (+1d over round 9's lifecycle
  item); LLM logs and the query-filter rule (+1.5d); the S9-4 skip list (+0.5d).
- **SF3 default scope, about +1 day** over the per-function filter.
- **S9-1 to S9-3, about +3 days:** routing and creation order (+1d over round 9's bare-repo item),
  gc/backup/disk (+2d).
- **Tests:** up from 5 to about 6 days.

Round 9's breakdown follows.

- **Per-sealed-task bare repos: about 4 days.** They also remove round 8's per-task specs ref
  (2 days) and most of its hidden-read work (1.5 days), which are now folded in.
- **Other round-9 items: about 8 days net.** The interim git hardening, the NB2 choke points, the
  sealing key type, NB5, the SF3 store default, SF5 and SF6.

## 6. Independent bugs this inventory found

These need fixing whether or not a task is sealed. Two were fixed in
https://github.com/helixml/helix/pull/3625 (merged; in `2007b960c`): prompt-history sync on
`ActionGet`, and the design-review "creator always has access" shortcut.

1. **The stream WebSocket accepts input from any reader.** `/ws/stream` is authorized with
   `ActionGet`, but forwards input unless the caller holds a non-interactive embed key
   (`desktop_stream_readonly.go:55`). Fix: set read-only unless the caller passes `ActionUpdate`.
2. **The agent channel can be taken over.**
   - `/external-agents/sync` admits **all admins**, any runner-token holder for any session, and
     the owner with any credential.
   - `validateExternalAgentToken` is a stub that returns true.
   - The connection registry is last-writer-wins and never closes the displaced connection.
3. **The owner's personal key can register `/revdial` `desktop-{sid}`**, and the registration is
   last-wins. This allows interception of all API→desktop traffic.
4. **The creator can read session keys in plaintext,** from `GET /api_keys?types=all` and from
   `GET /sessions/{id}/zed-config`.
5. **Org Sandboxes routes on session-backed rows admit any org member.** A fix is in progress in a
   separate PR.
6. **Project members can run code live in running sandboxes.** A project stdio MCP reaches every
   running sandbox in the project within 30s. Even unsealed, at minimum record an access event, and
   consider requiring a restart for a new stdio command.
7. **Preview tokens can expose any port publicly.** They accept any port from 1 to 65535 on an
   unauthenticated `share-*` host. Consider a port allowlist.
8. **The desktop MCP `save_screenshot` writes to any path the caller gives.**
9. **The session MCP's agent-only tools accept the owner's personal key.**
10. **Spec-task workflow gaps:**
    - `approve-specs` has no agent-key guard.
    - `request-rebase` has no task-scope check.
    - `PUT status` attributes the resulting prompt to the old `QueuedBy`†.
11. **`POST /interactions/…/questions/…/respond` records no provenance.**

## 7. Test plan

**Route policy** (`route_auth_policy_test.go` and the `route_policy_*_test.go` files):

- Add a `Seal` field and the `.seal(sealDeny())` / `.seal(sealAllow(grants...))` builders to
  `routePolicy`.
- **Unclassified-route test (default-deny guard).** Walk the production router. A route resolves a
  sealable id if its template has a gate-1 prefix, **or** an AST walk of its handler finds a call to
  a gate-2 helper or to the `Store.Get{Session,SpecTask,Sandbox,Interaction,SpecTaskWorkSession,
  SpecTaskZedThreadByZedThreadID}` lookups (§3.3). The test fails if:
  - such a route has no `.seal(...)` declaration;
  - a `sealAllow` declaration disagrees with `sealAllowList`;
  - `sealAllowList` names a template the router does not serve.

  Verify the test by deleting one declaration (for example the one for
  `POST /api/v1/zed/events`) and watching it fail.
- **Coverage of the 66c7cdd list.** A table-driven test asserts that each §1f route is declared
  `sealDeny()` (or SELF-only for the zed event family). For those routes it also asserts that a
  sealed fixture refuses K: the body-addressed ones through gate 2, and the rest through gate 1.
- An AST test checks that every handler referencing a desktop primitive reaches `sealDecision`
  through a gate-2 helper. The primitives are:
  `connman.Dial`, `dialDesktopExec`, `callSessionDesktopJSON`, `sendCommandToExternalAgent`,
  `hydra.NewRevDialClient`, `openPersistentTerminal`, `uploadToSandbox`, and the
  `hydra` `RunSandboxCommand` / `WriteSandboxFile` / `DeleteSandboxFile` / `KillSandboxCommand`
  calls.

**`SandboxSealSuite`** (gomock store, plus a fake `connman` that counts `Dial` and `Set`):

- **Callers:**
  - session owner via browser;
  - owner's personal key;
  - the key that created the task;
  - an admin-minted key for the owner;
  - org owner;
  - platform admin;
  - project member with update;
  - an org member on an `OrgMembersAccess` project;
  - a sibling task's confined key;
  - an unconfined sibling session key;
  - the shared runner token;
  - a per-runner token bound to another host.
- **Every `sealDeny()` route** × every caller (K included):
  - assert 403 with `error: "sandbox_sealed"` for callers inside the tenant, and 404 for callers
    outside it;
  - assert **no side effect**: no store write (gomock has no expectation), no `Dial`, no `Set`, no
    `SendChan` send;
  - assert an access record with `outcome: denied` and `reason: not_allowlisted`, and that
    `seal_broken_at` is still null.
- **Allowed cases:**
  - the session's own key on the SELF routes reaches the dial;
  - a bound runner reads `/zed-config`;
  - K's stop, archive and delete succeed and write `outcome: allowed`, `kind: lifecycle`;
  - K reads `/seal` and `/sandbox-access`;
  - another credential of K's user is refused on those same routes.
- **Denied by default (regression guard):** K's `/messages`, resume, cancel, screenshot and stream
  on the sealed task all return `sandbox_sealed`.
- **Gate 3:** a `review_comment` webhook turn, a `rebase_required` from request-rebase, and a
  sibling `SendSpecTaskAgentMessage` aimed at the sealed task are dropped, each with an
  `outcome: denied`, `kind: turn` record. A `ci_status` poller turn is delivered.
- **Control/twin probe (§3.7), automated.** Using K: create the unsealed control A and the sealed
  twin B. Iterate `GET /api/v1/seal-policy` for every `deny` route.
  - A must not return `403 sandbox_sealed`.
  - B must return exactly that.
  - B's access record must gain one `outcome: denied` row per call, and B's `seal_broken_at` must
    stay null.

  Run it with `sealDecision` stubbed to "allowed" and confirm it fails, so the test proves the seal
  rather than the existing authz.
- **Immutability and K:**
  - a sealed create through a browser session or a scoped key returns 400;
  - a sealed create without `just_do_it_mode` returns 400;
  - `GET /seal` on B returns `sealed_by_key_id` equal to K's `key_…` id.
- **Immutability:**
  - PUT `sandbox_sealed` with a differing value returns 400;
  - the store update omits the column;
  - `POST /spec-tasks/{sealed}/clone` returns `sandbox_sealed` to K and everyone else (N1); cloning an unsealed task with `sandbox_sealed: true` makes the cloning key K;
  - `sandbox_sealed` with `confine_agent_key: false` returns 400.
- **Key secrecy:**
  - `GET /api_keys?types=all` as the owner shows only the prefix for the sealed session key;
  - the owner is refused `/zed-config`;
  - `/external-agents/sync` and `/revdial` reject the owner's personal key and an admin.
- **Freeze:**
  - change a project MCP and the execution config after creation; `getZedConfig` for the sealed
    session is unchanged and the execution-config PATCH returns 403;
  - an unsealed sibling task does see the change;
  - drift in secrets or the startup script on a restart writes `config_drift`.
- **Git:** a push to the sealed task's branch by the owner's personal key is rejected; a push by the
  session key is accepted.
- **Org Sandboxes:** on a session-backed `sbx_` row for a sealed session, every route (commands,
  logs, kill, files in every verb, terminal, screenshot, PATCH, DELETE) returns `sandbox_sealed` to
  K, org owners and admins. Each refusal writes a record with `scope: sandbox`.

**Round-8 additions:**

- **B1, org tools.**
  - For each of the 13 per-task `SpecTasks` tools: a sibling **unconfined** session key in the same project,
    and an org bot, target the sealed task. Each gets `sandbox_sealed` and writes one row with
    `scope: task`, `route: mcp:helix-tasks:<tool>`, `outcome: denied`.
  - `List` omits the sealed task.
  - On an unsealed twin, the same calls succeed. That is the control.
- **B2, writes.** Each of these is rejected and recorded:
  - a `git push` to `helix-sealed/<task>/work` and `helix-sealed/<task>/specs`, by K, the owner's
    personal key and an admin-minted key;
  - `PUT /contents` on a sealed ref;
  - PR `PATCH` and `merge` on the native API and on `/api/github/api/v3/.../pulls/{n}/merge`;
  - artifact `PUT` and `DELETE` on the lane's artifact by a project member.

  A push by the lane's own session key succeeds. The upstream mirror never receives the sealed ref.
- **B2, reads (option a).**
  - A non-K `git fetch` / `ls-remote` does not advertise sealed refs, and a fetch of a sealed
    commit by SHA fails.
  - K's fetch sees them.
  - Git read API, PR reads, artifact `GET` / `embed` / `download` and the public viewer return
    `sandbox_sealed` (or omit the row from listings) for org owners, admins and the finder lane's
    key; K succeeds.
  - `GET /spec-tasks`, `GET /sessions`, clone-group progress and project artifacts omit the lane for
    everyone but K. The filtering is checked in the store query, not in Go.
- **S1.** A handler goroutine with a user and no stamped route key is denied (`no_route`).
  `context.Background()` without the platform mark is denied. Each platform entry point carries
  `withPlatformCaller`, asserted by an AST test listing the entry points.
- **S3, dry-run probe.** `X-Helix-Seal-Probe: 1` from K on A returns `probe_ok` with no side effect:
  - the gomock store gets no writes;
  - the fake connman records no dial;
  - no git command runs.

  On B the same request returns `sandbox_sealed`. From any other caller the header is ignored.
- **S4, key binding.**
  - After sealing, `GET /api_keys?types=all` shows only K's prefix, even to K's user's browser
    session.
  - `DELETE /api_keys?key=K` returns 409 while the lane is live.
  - A key minted by K fails every `sealing_key` grant.
  - Access rows carry `credential_fingerprint`.
- **S5.**
  - With a replica heartbeat reporting `seal_version` 0, a sealed create returns
    `503 seal_unsupported_by_cluster`.
  - The create response contains the echo.
  - Every state in the §3.8 table returns exactly the documented field values.
- **S6.**
  - Break the seal, access the task as an admin, then delete it.
  - `GET /seal` and `/sandbox-access`, as K, still answer from the tombstone, showing the
    `break_glass` row, the `reason: seal_broken` access, and the deletion event.
- **S7.**
  - Concurrent create and orchestrator pickup never observes a sealed task without its seal row or
    `sealed_config`.
  - A null `sealed_config` makes `getZedConfig` return 503.
  - A key re-minted after restart is `APIkeytypeSandboxSession` with `TaskConfined`.
  - A queued `user`-provenance prompt left in `sending` is refused at
    `ReconcileStuckSendingPrompts`.
- **S8.**
  - Edit org and project guidelines mid-run; the sealed lane's next Helix-composed turn still
    carries the frozen text.
  - A `ci_status` with log text is reduced to status-only for the sealed lane.
- **N2.** A share-* request for a sealed session returns `sandbox_sealed`.
- **N5.** Two identical denied calls produce two rows; two identical K stops produce two rows.
- **N6.** With `spec_task_seals` unreadable (fault injection), every non-platform call on a sealed
  task is denied with `seal_state_unavailable`.
- **Cyber requirement.** If break-glass or an org-owner stop is built, its row has `actor_class`
  `admin_break_glass` / `org_owner_stop`, distinct from K's `sealing_key`. Break-glass sets
  `seal_broken_at`; an org-owner stop does not.

**Round-9 additions:**

- **NB1, structural.**
  - A commit pushed by the lane exists only in its sealed repo: `git cat-file -e <sha>` in the
    shared repo fails.
  - Protocol v0 and v2 fetches by SHA against the shared repo URL both fail.
  - A non-SELF push that tries to create a shared ref at the sealed SHA is rejected by git, because
    the object is missing.
  - The read API and branch create with the SHA return not-found.
- **NB1, interim (fault injection).** With `sealedRepoFor` forced to return the shared repo (a
  simulated routing bug), and the lane's objects copied in:
  - a v2 fetch by SHA from a non-K caller still fails, because v0 is forced;
  - the SHA is refused as unreachable on the read API and branch create;
  - `ls-remote` does not advertise the reserved prefix.
- **NB2.**
  - `/push?branch=helix-sealed/<T>/work&force=true`, `/pull?branch=…&force=true` and `/push-pull`
    by a project writer change nothing in the sealed repo.
  - `PUT /git/repositories/{id}` with a new `ExternalURL`, followed by every platform pusher
    (crash recovery, external sync, workflow, PR proposals), sends nothing under the reserved
    prefix to the new remote.
  - Each choke-point function refuses a reserved ref or a sealed path when called directly.
- **SF1.**
  - In the shared repo, creating `refs/heads/helix-sealed/x` is refused for every caller,
    including with an empty hook env.
  - An upstream that has `helix-sealed/*` branches does not sync them in.
  - A branch named exactly `helix-specs-sealed` is refused.
- **SF2.** After a lane pushes, the shared repo's `GitRepository.Branches`, repo GET/list and
  `/git/{id}/status` `branch_count` are unchanged.
- **SF4 and S3.** A sealed receive-pack is refused by the Go check before `SyncAllBranches` runs:
  the upstream mock receives no call. The dry-run probe on a git route has no side effect.
- **NB3.**
  - A sealed create from a project with auto-approve on still yields
    `AutoApprovePullRequests = false`.
  - `propose_pull_request` from the lane's own key returns `sandbox_sealed`, as do `pr-proposals*`
    and `auto_approve_future`.
  - No upstream push or PR is opened.
- **NB4.**
  - `POST /api_keys?type=sealing` returns the key once; later `GET /api_keys` with and without
    `types` shows only its prefix.
  - Creating and re-registering a repo as K's user hands Kodit a non-sealing credential (slice 1)
    or the service credential (slice 2).
  - Sealing with any non-`sealing` key returns 400.
- **NB5.** With a live sealed task, project delete, repo delete, detach and move return
  `409 sealed_task_live`. After K archives, they proceed, and the tombstone shows
  `task_deleted_at`.
- **SF3.** Every one of the 33 `ListSpecTasks` / `ListSessions` call sites, exercised through its
  route (including `/projects/{id}/tasks-progress` and `/tasks-usage`), omits the sealed task for a
  non-K caller. The store-level test asserts the default `sealed = false` predicate.
- **SF5.** Lower one heartbeat replica to `seal_version` 2 during a run:
  `min_cluster_version_since_seal` drops to 2 and stays there. A heartbeat gap sets
  `cluster_gap_seen_at`. `deploy-prod.sh` refuses a rollback below a live lane's version.
- **SF6.** The AST test fails if `Organization.Guidelines`, `Project.Guidelines` or
  `UserMeta.Guidelines` is read outside `guidelinesFor`. Editing guidelines mid-run changes no
  sealed-lane turn from any of the three composers.
- **SF7.** The AST assertion fails when a handler with a `ref` / `branch` / `sha` / `commit`
  parameter reaching `GetGitRepository` lacks `sealOfRef`. Verify it by removing one call.

**Round-10 additions:**

- **Blocker 1, NB5 choke points.** With a live sealed task:
  - `DELETE /organizations/{id}`, admin user delete, the org-runtime teardown and the project-delete
    cleanup each return `sealed_task_live` / `ErrSealedTaskLive` **before any deletion**. Afterwards
    the shared repo, project row and sealed repos are untouched, and one `denied` record exists per
    lane.
  - The direct choke-point calls (`DeleteRepository`, `purgeProject`, `Store.DeleteProject`) refuse
    in a unit test.
  - The no-bypass AST test fails when a new `os.RemoveAll` under the repo root or a direct GORM
    project delete is added outside the choke points; verify it by adding one.
  - Detach, move and set-primary return 409. Attach succeeds and is recorded.
  - After K archives, all of them proceed, and the tombstone has `task_deleted_at`.
- **Blocker 2, N9-1.**
  - For a sealed session: `GET /apps/{id}/llm-calls?session=`, `GET /admin/llm_calls?session=`,
    `GET /apps/{id}/interactions?session=` and `GET /apps/{id}/step-info?interactionId=` return
    `403 sandbox_sealed` to the app owner and an admin, with a `denied` record.
  - The same routes without a filter omit the lane's rows.
  - K's `/sessions/{id}/usage` returns token and cost metadata with no request or response bodies.
  - The provider-recovery readers still see the rows.
  - Route-policy rule 7 fails when a handler reading `session` / `interactionId` / `spec_task_id`
    from the query lacks a seal declaration; verify by removing one.
- **SF3, default scope.**
  - With no seal visibility in the context, every store read of the listed tables (Find, First,
    Count, Row, Pluck, and `UnifiedSearch`'s ILIKE) returns no sealed rows. A table-driven test
    covers each model.
  - With `withSealVisibility(K)` they return K's lane only. With `withPlatformCaller` they return
    everything.
  - The raw-SQL allowlist test fails on a new `Raw` naming a listed table without
    `sandbox_sealed`.
  - `GET /search?q=<sealed task name>` as K's user's browser session finds nothing.
- **S9-1.**
  - A `sealedRepoFor` error returns 503, and a missing sealed path returns 503, with **no** fallback.
  - The lane's own key on `/git/{R}` gets 403 before any git process starts.
  - A forged `/git/sealed/{T'}/{R}` returns 404 to K.
  - A crash between `pending` and `ready` is reconciled, and the desktop doesn't start meanwhile.
- **S9-2.**
  - The refcount drives `gc.pruneExpire` 0 → 1 → 0 under the lock. The startup reconciler fixes a
    mismatch.
  - `MaintainRepo` refuses prune / `repack -a -d` on a repo with a sealed dependent.
  - Relative alternates resolve.
  - A filesystem-snapshot restore of `git-repositories/` keeps sealed repos fsck-clean.
- **S9-3.**
  - A push over `receive.maxInputSize` or over quota is refused with `reason: quota`.
  - The archive produces a `base..work` bundle that verifies against the shared repo.
  - The TTL removes old bundles but keeps the seal row and events.
  - The growth alert fires on a synthetic shared-repo growth.
- **S9-4.** The orchestrator over a sealed task for N ticks: the six sites log one skip each and
  raise no error. The git read API with `ref=helix-sealed/T/work` returns not-found, for K too.
- **Nits.**
  - Creating a repo with id `sealed` is refused.
  - The sealed push lock is independent of the shared repo's.
  - `/filestore/*` cannot reach `git-repositories/sealed/` (traversal and symlink cases).

**Live, in the inner Helix** (mandatory, because this changes the agent-channel lifecycle):

1. Create a sealed spec task with a live Zed and confirm `zed_thread_id` is set.
2. As the owner in the browser, and as K through the CLI:
   - the stream, screenshot, terminal, upload, exec, workspace-file, messages and resume all return
     `sandbox_sealed`;
   - K's stop works;
   - the access-record panel shows the blocked attempts while the seal state stays intact.
3. Confirm the sandbox still works by itself: the daemon syncs, MCPs connect, the agent pushes its
   branch, and credentials refresh.
4. Add a project stdio MCP mid-run and confirm it does **not** appear in the sealed sandbox's
   `settings.json` after 60s, while an unsealed task in the same project picks it up.
5. After K's stop, confirm that platform-initiated restart, if any, builds from `SealedConfig`.
   Then confirm K's archive and delete remove every resource (§ lifecycle checklist in
   CLAUDE.md).
6. Break the seal, confirm the state is visible, and confirm normal access returns.
