# GitHub Engineer

A one-Worker general-purpose software engineer who watches a
GitHub Project v2 Kanban board on a single repo, picks tasks
off the `Todo` column, and drives them through `In Progress` →
`In Review` → a merged PR → `Done`. The owner reviews and
merges the PR; **the engineer never merges**.

This is the same shape as [`demos/github/`](../github/) — one
role, the github transport, no bespoke MCP tools — but the
engineer here writes code, docs, design, and architecture, not
just docs reviews. The whole engineer is one role file:
[`roles/software-engineer.md`](roles/software-engineer.md).

About 5 minutes after the github webhook setup. Once the
engineer is hired, everything runs from the helix-org UI and
the GitHub Project board.

## What this demo shows

- **A Kanban board as the surface humans use; webhooks as the
  trigger the agent reacts to.** The agent doesn't poll the
  board. It reacts to standard GitHub webhook events
  (`issues.opened`, `pull_request_review.submitted`,
  `pull_request.closed`) and drives the Project v2 `Status`
  column with `gh project item-edit`. You see a normal Kanban
  board; the agent's column moves are indistinguishable from a
  human dragging cards.
- **End-to-end task ownership in one role.** Pick up, branch,
  implement, run checks, push, open PR, react to review, move
  the card to Done after the human merges. All in
  [`roles/software-engineer.md`](roles/software-engineer.md).
  Edit the role to change behaviour — e.g. require a design
  comment before any PR, or forbid touching `*.go` files —
  and the next webhook activation reads the new behaviour.
- **The owner gates the merge, the agent gates everything
  else.** The role explicitly forbids merging. The owner is
  always the last gate before code lands on `main`. The
  agent's job is to make the merge button as cheap to press as
  possible.
- **No GitHub identity for the agent.** Same as the docs demo:
  the agent uses your `gh auth token`. Comments, PRs, branches,
  and project board moves are authored as you. Fine for solo
  or small-team work; promote to a machine user for shared
  repos (see
  [`design/github-transport.md`](../../design/github-transport.md)).

## Prerequisites

- `gh` authenticated as you with push access to the target repo
  (`gh auth status` is green; `gh repo list` shows what you'd
  expect). The token also needs the `project` scope so the
  engineer can read and move project board items —
  `gh auth refresh -s project,read:project` if your token
  predates these.
- `helix-org`, `claude`, and `gh` on PATH.
- Port `8080` free on the host, or pick another and pass
  `--addr :<port> --public-url http://localhost:<port>` to
  `helix-org serve` and tunnel that port instead.
- A public URL pointing at your local helix-org so GitHub can
  reach the webhook.
  [`cloudflared tunnel --url http://localhost:8080`](https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/install-and-setup/tunnel-guide/local/)
  or [`ngrok http 8080`](https://ngrok.com/) work.
- A GitHub repo you own (or have admin on) with a Project v2
  board attached. The board's `Status` field needs **all four**
  of these options (case-sensitive):
  - `Todo`
  - `In Progress`
  - `In Review`
  - `Done`

  GitHub's default board ships with `Todo`, `In Progress`,
  `Done`. Add `In Review` as a status option from the project
  settings (Fields → Status → Add option).

  If the repo has no project yet, create and link one:
  `gh project create --owner <you> --title "<name>"` then
  `gh project link <number> --owner <you> --repo <owner>/<repo>`.
  Note the project number it prints — you'll paste it into
  step 6.

  Recommended: in the project's settings, enable
  *Auto-archive items* and on the repo enable
  *Automatically delete head branches* after merge — the
  engineer leaves card archiving and branch cleanup to GitHub.

## Setup

### 1. Build helix-org

```bash
cd /home/phil/helix/helix-org
make build
```

### 2. Start the server (terminal 1)

```bash
cd demos/github-engineer
../../bin/helix-org serve --db /tmp/github-engineer.db --envs-dir /tmp/github-engineer-envs
```

If this is the first run against the DB, the server bootstraps
the owner Worker (`w-owner`).

### 3. Expose helix-org publicly (terminal 2)

```bash
cloudflared tunnel --url http://localhost:8080
```

Note the public `https://....trycloudflare.com` URL it prints.
You'll paste it into the next step.

### 4. Open a chat (terminal 3)

```bash
cd demos/github-engineer
../../bin/helix-org chat --new
```

Now you're driving the org through `claude`, acting as
`w-owner`. Every step from here is a chat prompt.

### 5. Wire up the github transport

Substitute `<owner>/<repo>` and the tunnel URL from step 3.
Paste this into the chat:

> Wire up the github transport for repo `<owner>/<repo>` on
> public URL `<tunnel-url>`.
>
> 1. Read the owner's gh token: `gh auth token`. (You're
>    running on the same host as the owner; their `gh` is
>    already authenticated.)
> 2. Generate a webhook secret: `openssl rand -hex 32`.
> 3. Run `helix-org config set --db /tmp/github-engineer.db
>    transport.github` with both values as JSON.
> 4. Register the webhook on the repo: `gh api -X POST
>    /repos/<owner>/<repo>/hooks` with `name=web`, `active=true`,
>    events `["issues", "issue_comment", "pull_request",
>    "pull_request_review", "pull_request_review_comment"]`,
>    config `{url: "<tunnel-url>/github/webhook",
>    content_type: "json", secret: "<the secret>"}`.
> 5. Confirm with `gh api /repos/<owner>/<repo>/hooks` that the
>    hook is active. Print the hook ID.

Nothing about the github token ever appears in your shell
history; it lives only in `transport.github` (operational
config, redacted on read) and on disk in `gh`'s own config.

### 6. Hire the software engineer

Substitute `<owner>/<repo>`, your project owner (the user or
org that owns the project), and your project number. Paste:

> Set up the software engineer for `<owner>/<repo>`. Read
> `./roles/software-engineer.md` and create role
> `r-software-engineer` from its body. Create stream
> `s-github` with `transport: github` and config
> `{"repo": "<owner>/<repo>", "events": ["issues",
> "issue_comment", "pull_request", "pull_request_review",
> "pull_request_review_comment"]}`. Create position
> `p-software-engineer` under `p-root` with that role. Hire
> AI worker `w-software-engineer`; identity is:
>
> ```
> You are the software engineer for <owner>/<repo>.
>
> Project board:
> - owner: <project-owner>
> - number: <project-number>
>
> Your role text describes how to discover the project IDs and
> move cards. Cache them in ./project-config.json on first
> activation.
> ```
>
> Grant the tools listed in the role's `Tools (MCP)` section.
> Then `worker_log` on `w-software-engineer` with `wait=60`
> until you see `=== exit: ok ===`. The on-hire activation
> runs project-board discovery; you should see some
> `gh project ...` commands in the log.

When the chat agent reports the on-hire activation finished,
the engineer is live and listening for github events.

---

**From here on the demo is UI-driven.** Open the helix-org UI
at <http://localhost:8080/ui/> and your GitHub Project board in
two side-by-side windows.

## Driving the demo

### 7. Add a task to the board

In the GitHub UI: open your Project → `Todo` column → `+ Add
item` → `Create new issue`. Title it like a normal task — for
example, "Add a /version endpoint that returns the current
commit SHA". Write a body that describes what you want.

GitHub fires `issues.opened`. The webhook hits helix-org. The
engineer wakes up.

### 8. Watch the engineer in the UI

In the helix-org UI, click `w-software-engineer`. You should
see, roughly in order:

1. The card moves from `Todo` → `In Progress` on your project
   board.
2. A comment lands on the issue: "Picked up — branching off
   `main`."
3. A branch shows up on the repo: `issue-N-<slug>`.
4. A PR opens, body says `Closes #N`.
5. The card moves to `In Review`.

If the task was genuinely ambiguous, the engineer will instead
post one clarifying question on the issue and stop. Answer in
the issue thread; the resulting `issue_comment.created` event
reactivates them.

### 9. Review and merge

Open the PR in the GitHub UI. Read the diff.

- If you want changes, leave a `request_changes` review with
  line comments. The engineer will push fixes and reply with a
  one-line summary of what changed.
- If you're happy, approve and merge.

The engineer **does not merge**. That's your gate.

### 10. After the merge

GitHub fires `pull_request.closed` with `merged: true`. The
engineer wakes, finds the issue the PR closed, and moves the
card to `Done`. They comment on the issue: "Merged — closing
the loop." They delete the branch.

If you closed the PR without merging, the engineer moves the
card back to `Todo` instead and comments "PR closed without
merge — back on the board."

### 11. Live-edit the role

Edit `roles/software-engineer.md` to change behaviour — say,
require a one-paragraph design note in every PR body, or block
on touching specific paths. Then in chat:

> Update `r-software-engineer`: replace its content with the
> current contents of `./roles/software-engineer.md`.

The next webhook activation reads the new role text and behaves
accordingly. No restarts.

## Tear it down

In chat, mirroring step 5:

> Tear down the github-engineer demo for `<owner>/<repo>`.
>
> 1. List webhooks on the repo: `gh api /repos/<owner>/<repo>/hooks`.
> 2. Find the one whose `config.url` starts with
>    `<tunnel-url>/github/webhook` and delete it:
>    `gh api -X DELETE /repos/<owner>/<repo>/hooks/<id>`.
> 3. Run `helix-org config delete --db /tmp/github-engineer.db
>    transport.github`.
> 4. Confirm both.

The token isn't deleted because the agent never created one —
it's your existing `gh auth token`, which you go on using.

Then Ctrl-C terminals 1 and 2.

## What this shows

- **Same transport, different role.** The github transport is
  unchanged from the docs demo. All the difference is in the
  role text. A code reviewer, a release-notes writer, a
  security-triage bot — same pattern, different role.
- **Project boards as the human-visible workflow.** The agent
  drives `Status` columns directly via `gh project item-edit`.
  You watch a normal GitHub Kanban board; the agent's moves
  look like any other human dragging cards.
- **PR review is the gate.** The agent does not push to `main`,
  does not merge, does not close issues directly. Every change
  ships through a PR you review. The role's "On
  `pull_request.closed && merged`" branch is the closure of
  the loop, not the start of one.

## What this doesn't cover (yet)

- **`projects_v2_item` events.** A draft card (one without a
  linked issue) added directly to the board doesn't fire
  `issues.opened` — only `projects_v2_item.created`. That
  event is organization-scoped, not repo-scoped, so the demo's
  repo webhook doesn't see it. The engineer wakes only when an
  *issue* is created. Promoting the transport to handle
  organization webhooks is a config change; the role can be
  extended to react to `projects_v2_item` without a code
  change.
- **Concurrency on one task.** If two `issues.opened` events
  arrive for the same issue (e.g. via a project automation
  rule), the engineer might branch twice. Production wants
  per-issue locking; the demo doesn't.
- **Long-running tasks.** Each activation is a one-shot claude
  invocation. A multi-day implementation that needs ten
  sequential thinking sessions has to be expressible as N
  short reactivations triggered by review feedback or comments.
  For most issue-sized work that's fine.
- **Branch hygiene on dead PRs.** The engineer deletes branches
  after merge, but doesn't garbage-collect orphan branches from
  PRs that closed without merge. Those need a sweep, not
  implemented here.
- **Multiple repos.** One Stream per repo, one engineer per
  repo. Fan-in to one Stream with the repo in `Extra` is a
  role + stream config edit, not a code change.
