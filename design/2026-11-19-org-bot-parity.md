# Org bot parity with spec tasks: PRs, Keep Alive, embed workspace

Date: 2026-11-19. Branch: `feature/org-bot-parity`.

## Why

Org bots maintain their own craft (playbooks, skills) in git, but cannot ship it.
Seen live: luke's recruiter bot committed a day of playbook rules to
`helix-sales-skills` on `feature/recruiter-seeds-eursap-16nov`, pushed it, and
could only hand the user a GitHub compare link — `propose_pull_request` refuses
any session without a spec task. Separately, the page HelixOS embeds for an org
bot (`/embed/project`) is a bare chat + desktop: no model indicator, no
workspace tabs, no status, no Keep Alive — so a bot idle for an hour (e.g. while
its human finishes a 2FA login) has its desktop shut down.

## 1. PR proposals for org bots (reuse everything)

`SpecTaskPRProposal` gains `OrgBotID` and `OrganizationID` (indexed). A bot
proposal has `SpecTaskID = ""` (the column stays NOT NULL; no destructive
migration). Exactly one of the two is set.

`PRProposalService` resolves a **proposal owner** for every call:

| Concern | Spec task | Org bot |
|---|---|---|
| Key / project | task.ID, task.ProjectID | bot id, bot's project |
| May propose | implementation statuses | always |
| Default head | task.BranchName | none — required |
| Default base | `TaskTargetBranch` | repo default branch |
| Default title | task.Name | none — required |
| Existing head branch | must be granted | allowed unless another task/bot owns it (bots push freely) |
| Auto-approve | task fields | bot fields (`AutoApprovePullRequests`, `…By`) |
| Fallback approver | task.CreatedBy | hiring user |
| PR attach | task.RepoPullRequests + status | none (proposal row is the record) |
| Attention events | yes | none (no spec task); the card in the bot's chat is the prompt |
| Agent notification | spec-task message enqueuer | bot message enqueuer (new, wired from helix-org: sends to the bot's session) |

Shared unchanged: decide/approve/edit/reject, OAuth check, open-on-provider,
push upstream, footer, retry, `OnBranchPushed`.

Git: org-bot session keys are not branch-restricted today and stay that way.

API: `GET /orgs/{org}/bots/{id}/pr-proposals`,
`POST /orgs/{org}/bots/{id}/pr-proposals/{proposal_id}/decide`,
`PUT /orgs/{org}/bots/{id}/pr-auto-approve`, authorized with the bot-edit check.

MCP: `propose_pull_request` / `list_pull_request_proposals` resolve the owner
from the session (`OrgWorkerID` → bot). Bot instructions say to use it instead
of compare links.

UI: `PRProposalCard` and the auto-approve control are parameterised by owner
(spec task | org bot) and rendered in the org bot's chat workspace and the embed.

## 2. Keep Alive for org bots

Bot field `KeepAlive`; `ListIdleDesktops` excludes sessions whose
`org_worker_id` bot has it set. Padlock toggle (same control as the spec-task
toolbar) in the bot's workspace.

## 3. Embed parity

`EmbedProjectPage` resolves the bot from the session (`org_worker_id`) and
renders the org-bot session workspace instead of a bare `AgentChat`: model /
harness / effort chip (read-only first — composer edits write only the session
and would diverge from the bot row), bot identity, status / restart-required
banner, Diff / Files / Browser / Usage / terminal tabs, Start / Stop / Restart,
Keep Alive, PR proposal cards.
