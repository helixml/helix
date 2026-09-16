---
name: pr-review-pipeline
description: Stand up an automated GitHub PR-review pipeline in any Helix org - a GitHub pull_request webhook trigger feeds a JS summarizing processor that starts a coordinator Bot, which dispatches one bare spec-task reviewer per (PR, head SHA) and posts formal reviews to GitHub. Use when creating, replicating to another org/repo, or repairing PR-review automation, wiring the trigger/processor/coordinator, granting the GitHub token secret, dispatching review tasks, or posting GitHub reviews.
---

# PR Review Pipeline

A pipeline where a Helix org Bot reviews nothing itself: it detects PR activity and farms each review out to a parallel **bare spec task** that runs in its own sandbox and posts a formal GitHub review. An approval means "ready to ship".

```
GitHub repo (pull_request events)
   └─ trigger    REPLACE_TRIGGER   (kind=github, webhook on the repo — NO bot subscribers)
        └─ processor REPLACE_PROCESSOR (kind=js, filters + summarizes)
             └─ coordinator Bot REPLACE_BOT (detection + dispatch only)
                  └─ spec tasks pr-review-<N>-<sha8>, one per (PR, head SHA)
                       └─ posts the review via scripts/post_review.sh, moves itself to done
```

## Files in this skill

| File | What it is |
|---|---|
| `coordinator-instructions.md` | The coordinator Bot's `content` — apply verbatim to `REPLACE_BOT` (the only source of truth; edit here, not in the bot). |
| `reviewer-brief.md` | The per-review-task brief template the coordinator pastes into each dispatched `create_spectask`. |
| `auth-ops.md` | GitHub token doctrine — read it before touching auth, and put it on any bot that posts reviews. |
| `scripts/post_review.sh` | Runnable asset for review tasks: posts `review.json` (event APPROVE or COMMENT — REQUEST_CHANGES is rejected; inline `comments` pass through) to GitHub and verifies via the reviews API that exactly one review exists at the commit. |
| `scripts/update_pr_summary.sh` | Runnable asset for review tasks: after the verified review post, refreshes the PR's `<!-- CURSOR_SUMMARY -->` description block (risk line + factual Overview, footer = reviewer login + reviewed sha) — GET → replace/insert ONLY between the markers → PATCH → GET-verify, with a re-GET right before the PATCH so surrounding description text can never be clobbered. |

## Placeholders (REPLACE before use — the skill is repo-agnostic)

| Placeholder | Meaning | Reference install (`helix` org) |
|---|---|---|
| `REPLACE_ORG` | Helix org that owns the pipeline | `helix` |
| `REPLACE_REPO` | GitHub repo, `owner/repo` | `helixml/helix` |
| `REPLACE_TRIGGER` | Trigger id for the repo's PR webhook | `s-github-pr` |
| `REPLACE_PROCESSOR` | Summarizing processor name | `p-gh-helix` |
| `REPLACE_BOT` | Coordinator Bot id / slug | `b-pr-coordinator` |
| `REPLACE_PROJECT` | Helix project the review tasks run in (must have `REPLACE_REPO` attached) | the `REPLACE_REPO` repo's project |
| `REVIEWER_LOGIN` | GitHub login the reviews are posted as | the GitHub App's bot login |

All commands below use the `helix` CLI (`helix api` escape hatch; env setup in the `helix-cli` skill; org endpoints live under `/api/v1/orgs/<org>/...`).

## Setup (top to bottom, once per repo)

### 0. Prereqs

- The org's control plane is reachable from GitHub (webhooks need a public URL) and the Helix GitHub App is connected in org settings (the UI's Connect-to-GitHub panel / app-manifest flow).
- `REPLACE_PROJECT` exists and has the `REPLACE_REPO` git repository attached — review-task sandboxes clone it. (Bot/project repo attachment: `list_bot_repositories` / `attach_repository` for the coordinator; any org owner can attach repos to the project.)

### 1. Webhook trigger — raw, and it stays free of bot subscribers

```bash
helix api -X POST /orgs/REPLACE_ORG/triggers --input '{
  "id": "REPLACE_TRIGGER",
  "name": "Github PR firehose",
  "kind": "github",
  "config": { "repo": "REPLACE_REPO", "events": ["pull_request"], "branches": ["*"] }
}'
helix api -X POST /orgs/REPLACE_ORG/triggers/REPLACE_TRIGGER/github/install-webhook
helix api /orgs/REPLACE_ORG/triggers/REPLACE_TRIGGER/github/webhook-status
```

**Never attach a Bot directly to the raw trigger** — every `pull_request.*` action would wake it. The coordinator only ever listens to the processor output below; the self-loop breaker in its instructions cannot make up for waking on noise.

### 2. Summarizing processor (filters + one readable line per event)

```bash
helix api -X POST /orgs/REPLACE_ORG/processors --input '{
  "data": { "type": "processors", "attributes": {
    "name": "REPLACE_PROCESSOR",
    "kind": "js",
    "input_source": "trigger:REPLACE_TRIGGER",
    "config": { "code": "<CODE BELOW>" }
  }}
}'
```

The code — keep it `js`, never `template`: `Message.extra` is `json.RawMessage` on the wire, Go templates cannot walk into it, and a failing template runner silently DROPS the event.

```js
// REPLACE_PROCESSOR: forward review-triggering PR events to the PR Review Coordinator.
function process(event, ctx) {
  var x = event.extra;
  if (!x || typeof x !== "object") return null;
  if (x.event !== "pull_request") return null;
  if (x.action !== "opened" && x.action !== "reopened" && x.action !== "synchronize") return null;
  var pr = x.pull_request;
  if (!pr || typeof pr !== "object") return null;
  var repo = (x.repository && x.repository.full_name) || "";
  var actor = (x.sender && x.sender.login) || event.from || "someone";
  event.subject = "[" + (repo || "github") + "] pull_request." + x.action;
  event.body =
    actor + " - pull request #" + pr.number + " " + x.action +
    ' : "' + pr.title + '"' + (repo ? " [" + repo + "]" : "") +
    (pr.html_url ? "\n" + pr.html_url : "");
  event.body_content_type = "text/plain";
  return event;
}
```

Capture the auto-generated output topic id it hands back (`po-topic-...`):

```bash
helix api /orgs/REPLACE_ORG/processors | jq -r '.data[] | select(.attributes.name=="REPLACE_PROCESSOR") | .id as $pid | .attributes.outputs[].id | "\($pid) \(.)"'
```

### 3. Coordinator Bot

Content = `coordinator-instructions.md`, with `REPLACE_REPO` (and `REPLACE_*` ids it references) substituted. Tools (additive to the standard worker set) from the reference install:

```
create_spectask start_spectask_planning update_spectask list_spectasks get_spectask
list_spectask_agent_messages send_spectask_agent_message stop_spectask_agent
list_secrets get_secret list_triggers get_trigger list_trigger_events
list_processors get_processor read_events list_bots get_bot bot_log chat dm managers reports
```

```bash
sed 's#REPLACE_REPO#owner/repo#' coordinator-instructions.md > /tmp/coordinator.md
jq -Rs '{id:"REPLACE_BOT", name:"PR Review Coordinator", content:., tools:[...]}' /tmp/coordinator.md \
  | helix api -X POST /orgs/REPLACE_ORG/bots --input -
```

Attach it to the processor output (NOT the raw trigger):

```bash
helix api -X POST /orgs/REPLACE_ORG/bots/REPLACE_BOT/attachments --input '{
  "source": { "kind": "processor_output", "processor_id": "<proc id>", "output_id": "<po-topic-...>" }
}'
```

### 4. Grant the GitHub token secret

The reviewer writes (reviews, comments) need `Pull requests: Read & write` + `Contents: Read` on `REPLACE_REPO`. With the org's GitHub App connected, grant the installation-token binding rather than a static PAT (no expiry to rot):

```bash
helix api /orgs/REPLACE_ORG/bots/REPLACE_BOT/available-secrets   # find the connected GitHub account
helix api -X PUT /orgs/REPLACE_ORG/bots/REPLACE_BOT/secrets/GH_TOKEN --input '{
  "source_kind": "connected_account",
  "account_id": "<connected account id>",
  "export_key": "github_app/installation_token",
  "usage": "export GH_TOKEN"
}'
```

Without a connected App, an owner creates a fine-grained PAT secret instead (`helix secret create`) and binds it the same way. Same grant goes to the project used for dispatch if review tasks need `GH_TOKEN` in their sandbox. `get_secret` rules for actually using it: [auth-ops.md](auth-ops.md).

### 5. Verify end to end

1. Open a test PR — then `helix api /orgs/REPLACE_ORG/triggers/REPLACE_TRIGGER/events` shows the delivery and `bot_log REPLACE_BOT` shows the activation.
2. Coordinator dispatches exactly one `pr-review-<N>-<short>` task; it posts one review and moves itself to `done`.
3. Reviews API confirms: latest review by `REVIEWER_LOGIN` has `commit_id == head SHA`.
4. PR description carries the `<!-- CURSOR_SUMMARY -->` block stamped for the reviewed sha; human-written description text outside the markers is untouched.
5. Push a new commit to the test PR: coordinator reviews again for the new SHA (summary block regenerated in place, footer sha updated) and ignores the same SHA on re-events.

## Upkeep — updates ship as PRs, not bot edits

The files in this skill directory are the source of truth. To change pipeline behavior: PR the edit here → after merge, push the text to the live object:

```bash
sed 's#REPLACE_REPO#owner/repo#' coordinator-instructions.md | jq -Rs '{content:.}' \
  | helix api -X PATCH /orgs/REPLACE_ORG/bots/REPLACE_BOT --input -
```

Never hand-edit bot content or processor code on the box — that silently forks from the repo. Reviewer-brief and processor changes follow the same rule (processor: `PUT /processors/<id>` with the new JSON:API document).

## Hard rules (why the pipeline is shaped this way)

- **Every verified review post is followed by a PR-summary refresh:** the reviewer runs `scripts/update_pr_summary.sh` to swap the `<!-- CURSOR_SUMMARY -->` block (risk line + factual Overview, footer `Reviewed by <REVIEWER_LOGIN> for commit <sha>`), replacing ONLY content between the markers and never claiming an unverified summary; risk is descriptive and never changes the verdict policy or blocks anything.
- **Verdicts: APPROVE or COMMENT only.** Never REQUEST_CHANGES — a bot review must never block a merge. Findings ship as ≤5 short inline comments anchored to exact diff lines; the body carries only ≤2 verified-state lines + `Reviewed: <sha>` (banned: opener verdict, "Must fix:"/"Also:" sections, numbered file:line body findings, effort estimates — see reviewer-brief.md §9). Tone: zero chatiness.
- **One review per (PR, SHA), always verified.** `scripts/post_review.sh` POSTs once and refuses to report success unless the reviews API shows exactly one review at the commit. Never claim a review you cannot verify.
- **Self-loop discipline.** Events caused by `REVIEWER_LOGIN` never start work; the coordinator never reviews itself.
- **Auth failures follow [auth-ops.md](auth-ops.md)** — one re-mint max, stop-with-verbatim-headers, and the salvage pattern so a finished review dies nowhere near a broken token.
- **Noise ban.** No comments on commit-message convention, commit splitting, PR-title wording, or taste-only style; findings carry correctness/security/data/test/CI/maintainability weight. Genuine blockers stay — block freely on real issues.
