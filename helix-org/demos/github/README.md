# GitHub

A one-Worker docs engineer who lives in your GitHub repo. They
watch issues and pull requests as they land, label docs issues
on sight, and review PRs that touch documentation or get the
`docs` label. The whole engineer is one role file.

About 5 minutes (most of it GitHub webhook setup the first time).

## What this demo shows

- **GitHub events as a Stream.** Issues, PRs, comments, labels
  applied — all canonical `Message`s on `s-github`. The Worker
  doesn't know about webhooks; it sees Events. GitHub itself
  calls this an "activity feed"; the transport just makes that
  feed addressable as a Stream.
- **Inbound transport, outbound shell.** `s-github` is
  inbound-only. Webhook deliveries become Events; nothing
  published *to* `s-github` goes anywhere. Acting on Events —
  labelling, reviewing, commenting, opening PRs — is `gh` in the
  Worker's Environment. GitHub isn't a messaging protocol with
  one outbound shape; it's a structured-action surface, and the
  shell already speaks it.
- **Routing by label, not by identity.** This engineer has no
  GitHub user account of its own. The Worker watches the firehose
  and picks out work via the `docs` label and changed-file paths
  — both visible in the standard GitHub UI, both easy to apply by
  hand or with `gh`. (Promoting the bot to a real GitHub user so
  you can `--assignee` and `--add-reviewer` it natively is a
  one-time setup; see "What this doesn't cover".)
- **One curated file.** [`roles/doc-engineer.md`](roles/doc-engineer.md)
  is the only thing on disk. The streams, the worker identity,
  the grants, the webhook wiring — all generated at hire time
  from one chat prompt.
- **Role drives behaviour.** What counts as a docs issue, when
  to approve vs request changes, how to phrase a review — all in
  the role text. Edit the role, run `update_role`, the next
  webhook lands on the new behaviour.

## Prerequisites

- `gh` already authenticated as **you** with access to the
  repos you want to wire up (`gh auth status` is green; `gh
  repo list` shows what you'd expect). The chat session below
  reuses this auth — your existing token is what the Worker
  uses to act on the repo. Labels, comments, and reviews will
  be authored as your user. Fine for solo work; for a shared
  repo you'll want a separate machine user (see
  [`design/github-transport.md`](../../design/github-transport.md)).
- A public URL for your local helix-org so GitHub can reach the
  webhook. `cloudflared tunnel --url http://localhost:8080` or
  `ngrok http 8080`.
- `helix-org`, `claude`, and `gh` on PATH.

## Setup

```bash
cd /home/phil/helix/helix-org
make build
rm -rf /tmp/github-envs /tmp/github.db
```

## 1. Start the server (terminal 1)

```bash
cd demos/github
../../bin/helix-org serve --db /tmp/github.db --envs-dir /tmp/github-envs
```

## 2. Expose helix-org publicly (terminal 2)

```bash
cloudflared tunnel --url http://localhost:8080
```

Note the public URL it prints.

## 3. Bootstrap and open a chat (terminal 3)

```bash
cd demos/github
../../bin/helix-org bootstrap --db /tmp/github.db --envs-dir /tmp/github-envs
../../bin/helix-org chat --new
```

## 4. Pick a repo and wire up the transport

In chat — substitute `<owner>/<repo>` and the tunnel URL from
step 2:

> Wire up the github transport for repo `<owner>/<repo>` on
> public URL `<tunnel-url>`.
>
> 1. Read the owner's gh token: `gh auth token`. (You're running
>    on the same host as the owner; their gh is already
>    authenticated.)
> 2. Generate a webhook secret: `openssl rand -hex 32`.
> 3. Run `helix-org config set transport.github` with both
>    values as JSON.
> 4. Register the webhook on the repo: `gh api -X POST
>    /repos/<owner>/<repo>/hooks` with name=web, active=true,
>    events `["issues", "issue_comment", "pull_request",
>    "pull_request_review", "pull_request_review_comment"]`,
>    config `{url: "<tunnel-url>/github/webhook", content_type:
>    "json", secret: "<the secret>"}`.
> 5. Confirm with `gh api /repos/<owner>/<repo>/hooks` that the
>    hook is active. Print the hook ID.

The chat agent does it all with the owner's existing `gh`.
Nothing about the github token ever appears in your shell
history; it lives only in `transport.github` (operational
config, never sent to the LLM after this) and on disk in `gh`'s
own config.

If you have many repos and want to pick interactively, ask
`gh repo list --limit 50` first and decide before you start
this prompt.

## 5. Hire the docs engineer

> Set up the documentation engineer for `<owner>/<repo>`. Read
> `./roles/doc-engineer.md` and create role `r-doc-engineer`
> from its body. Create stream `s-github` with `transport:
> github` and config `{"repo": "<owner>/<repo>", "events":
> ["issues", "issue_comment", "pull_request",
> "pull_request_review", "pull_request_review_comment"]}`.
> Create stream `s-tick-daily` (`transport: local`). Create
> position `p-doc-engineer` under `p-root` with that role. Hire
> AI worker `w-doc-engineer`; identity is "You are the docs
> engineer for `<owner>/<repo>`." Grant the tools listed in the
> role's `Tools (MCP)` section. Then `worker_log` on
> `w-doc-engineer` with `wait=60` until you see
> `=== exit: ok ===`.

## 6. Open an issue and watch the engineer triage it

```bash
gh issue create \
  --repo <owner>/<repo> \
  --title "README: setup steps mention an env var that no longer exists" \
  --body "Step 3 references HELIX_FOO; the code reads HELIX_BAR now."
```

In chat:

> Subscribe me to `s-github` and `read_events` with `wait=60` until
> the docs engineer reacts. Show me the events as they land.

GitHub fires `issues.opened` → the transport posts to `s-github` →
the engineer wakes, reads the issue, decides it's a docs issue,
and runs `gh issue edit <n> --add-label docs`. Refresh the issue
in the GitHub UI: the `docs` label is on it.

## 7. Pull the engineer into a PR

Open a PR (any PR) and label it `docs`:

```bash
gh pr edit <pr-number> --add-label docs
```

GitHub fires `pull_request.labeled` with `label.name == "docs"` →
the engineer wakes, runs `gh pr view <n> --json files`, runs
`gh pr diff <n>`, and posts a review with `gh pr review`. If the
prose is clear and the commands run, they approve; otherwise they
request changes with line-specific asks.

PRs that touch `*.md`, `README*`, or `docs/**` get a review
automatically — no label needed. The label is for "code-only PRs
where I still want docs eyes on the change".

## 8. Watch the daily sweep

Wait until 9am, or trigger the tick manually in chat:

> Publish to `s-tick-daily`: `"sweep"`.

The engineer pulls every open issue without a `docs` label
(`gh issue list --search "-label:docs"`), classifies each, and
labels what fits. Useful for backfilling on a repo that's been
running without them.

## 9. Live-edit the role

Edit `roles/doc-engineer.md` — soften the threshold for the
`docs` label, or tighten it, or add a new event type to handle.
Then in chat:

> Update the `r-doc-engineer` role: replace its content with
> the current contents of `./roles/doc-engineer.md`.

The next webhook activation reads the new content; behaviour
shifts immediately.

## 10. Tear it down

Cleanup is a chat prompt that mirrors step 4:

> Tear down the github transport for `<owner>/<repo>`.
>
> 1. List webhooks on the repo
>    (`gh api /repos/<owner>/<repo>/hooks`).
> 2. Find the one whose `config.url` starts with
>    `<tunnel-url>/github/webhook`.
> 3. Delete it: `gh api -X DELETE
>    /repos/<owner>/<repo>/hooks/<id>`.
> 4. Run `helix-org config delete transport.github`.

The token isn't deleted because we didn't create one — it's
your existing `gh auth token`, which you go on using.

Then Ctrl-C terminals 1 and 2.

## What this shows

- **Inbound is the transport's job; outbound is the shell's.**
  GitHub events become Events on a Stream. Acting on those Events
  — labelling, reviewing, commenting — is `gh` in the Worker's
  Environment. There's no MCP tool per github action and there
  doesn't need to be: the role describes the `gh` invocation, and
  if the workflow changes, only the role changes.
- **One Stream, many event types.** `s-github` carries every
  webhook delivery the stream config opts into. The role branches
  on `Message.Extra.event` + `Message.Extra.action` (e.g.
  `pull_request` / `labeled`); `Subject` and `Body` are the
  upstream title and user text used verbatim. Adding a new event
  type is a role edit plus a stream config update, not a code
  change.
- **Labels are the routing mechanism.** Without a github identity
  for the engineer, we can't lean on `--assignee` or
  `--add-reviewer`. A label is the next-best primitive: visible
  in the standard UI, applicable from the CLI in one flag, and
  fires its own webhook event. The role's "is this for me?" check
  reduces to "does this PR/issue have the `docs` label, or touch
  a docs path?".
- **Review-of-record for docs.** A PR that touches `README.md` or
  `docs/**` always gets a real review from someone who reads docs
  for a living. Code-only PRs don't, unless you opt in with the
  label.

## What this doesn't cover (yet)

- **Native GitHub identity.** Promoting the engineer to a real
  GitHub user (a "machine user") makes `--assignee`,
  `--add-reviewer`, and `@docs-bot` autocomplete work the way you
  expect. Five minutes of one-time signup. The transport doesn't
  change; the role gains an `actor` matcher. See the design doc
  for the full setup.
- **Rate-limiting and backoff.** A burst of webhook deliveries
  could fan out to a burst of activations. Production wants a
  per-Worker rate limit and a `gh` retry policy on 403s; this
  demo doesn't show either.
- **Multiple repos.** One Stream per repo, one Worker per Stream
  is fine for now. A docs engineer who covers an org would need
  either fan-in to one Stream with the repo in `Extra`, or a
  Worker per repo — both are role edits, not code changes.
- **Drafting docs PRs.** This engineer reviews; they don't yet
  open PRs of their own to fix typos or stale commands. The role
  has `gh` and `git`, so adding a "fix-it" trigger is a role
  edit; left out today to keep the demo tight.
- **Anything outside docs.** A code reviewer, a triage bot, a
  release-notes writer — all the same pattern, different role
  text.
