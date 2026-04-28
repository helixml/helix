# GitHub: an inbound Stream transport over the activity feed

**Status:** draft, pre-implementation. Poke holes before we touch
code.

## Goal

Add a `github` Stream transport that turns a repo's webhook
deliveries into Events on a Stream. Workers see canonical
`Message`s — same shape as email, webhook, internal DM — and act
on them via `gh` in their Environment. No new MCP tools are
introduced; the only org-graph change is `Stream.Transport.Kind
= "github"` plus a small config blob.

The first concrete user is the docs engineer demo
([`demos/github/`](../demos/github)). The transport shape is
chosen to be reused unchanged for any other "Worker that watches
a repo" use case (code reviewer, triage bot, release-notes
writer) — those are role text, not transport changes.

## Non-goals

- **Outbound.** GitHub is not a messaging protocol with one
  outbound shape; it's a structured-action surface (label,
  review, comment-on-line, request-review, merge, close,
  open-PR, …). Wrapping each action behind `publish` would
  reinvent the `gh` CLI's flag set with worse ergonomics. The
  shell does outbound. `publish` to a `github` Stream is an
  error.
- **A bespoke "GitHub identity" type.** The transport doesn't
  model GitHub users, teams, or apps. Identity matching (when
  needed) is a string compare in role text, against fields
  already in the webhook payload.
- **GitHub Apps as the install model.** Apps can't be assigned
  to issues or requested as reviewers, which kills two of the
  natural routing flows. We use repo-scoped webhooks and a PAT
  for `gh`. Apps may be added later as an *additional* identity
  option, not a replacement.
- **Polling.** Webhook-driven only. `gh api` polling from a
  Worker is allowed for one-off lookups (read a PR diff inside
  a trigger), but not as the trigger mechanism.
- **Retry / backoff / replay.** Single-delivery, fire-and-forget,
  same as the existing webhook transport. Production reliability
  is its own work.

## Identity model

Two identity setups are supported. Both run on the same
transport; the difference is entirely role-side string matching.

### Identity A: no GitHub identity (default for the demo)

The Worker has no GitHub user account of its own. The owner's
existing `gh` CLI token (whatever they got from `gh auth login`)
goes in `transport.github.token`; outbound `gh` calls in the
Worker's Environment are authored as that human.

Routing into the Worker uses **labels** and **changed-file
paths**, both visible in the standard GitHub UI:

- A docs issue is one with the `docs` label. Either the Worker
  applies it on classification, or a human applies it to summon
  the Worker.
- A docs-relevant PR is one whose changed files match docs
  conventions (`*.md`, `docs/**`, …) *or* one labelled `docs`.

GitHub fires a webhook for both label additions
(`issues.labeled`, `pull_request.labeled`) and PR opens, so the
Worker sees these triggers as Events without any per-user
routing. This is what the docs demo runs on today; zero GitHub
setup beyond `gh auth login` (which the owner has already done)
and a webhook registration that the chat session creates on
their behalf.

**Why `gh auth token`, not a fresh PAT.** GitHub deprecated the
PAT-creation API in 2020 and never replaced it; fine-grained
PATs are UI-only. The chat agent therefore can't programmatically
mint a new token. It can read the one the owner already has
(`gh auth token`), and that's enough — the owner has access to
the repos they want to wire up, so the existing token does too.

Trade-off: the token is broader than a per-repo fine-grained PAT
would be (it covers everything the owner's `gh` covers), and
outbound actions appear authored by the owner. Acceptable for
solo work and demos; awkward for shared repos where you'd like
the Worker's actions visibly attributed and the blast radius
narrowed. Production deployments should manually create a
fine-grained PAT in the GitHub UI — see Identity B for the
machine-user variant, which solves both problems at once.

### Identity B: machine user (production)

A real GitHub user dedicated to automation. Five-minute one-time
setup: signup with a `+bot` email alias, verify, set up 2FA,
invite to the repo or org with `Write`, generate a fine-grained
PAT scoped to the repo. The handle (e.g. `winder-doc-bot`) is
written into the Worker's identity stub at hire time.

This unlocks GitHub-native routing: `gh issue edit --assignee
docs-bot`, `gh pr edit --add-reviewer docs-bot`, `@docs-bot`
autocomplete in comments. The Worker compares its own handle
(cached on hire from `gh api user --jq .login`) against the
event payload to decide "is this for me?".

The transport is **identical** for Identity A and Identity B.
The only thing that changes is what string the role compares
against; the transport just delivers events. (We document a
future stream-config field — `actor` — that pre-flags
identity-match results in `Message.Extra` for role convenience,
but it isn't load-bearing.)

### Why not GitHub Apps

Tested for fit, ruled out:

- **Cannot be assigned** to issues. `gh issue edit --assignee
  some-app[bot]` rejects.
- **Cannot be a requested reviewer.** Same.
- **No autocomplete in `@`-mentions.** Apps don't show up in
  the user picker.

Apps are fine for "this bot reads webhooks and posts comments";
they fail for "this bot is a participant you can route work to".
We need the latter. (Apps may join Identity C later for
attribution-only — a tracked-but-not-required future feature.)

## Architecture

```
GitHub
  │  POST /github/webhook
  │  X-Hub-Signature-256: sha256=<hmac>
  │  X-GitHub-Event: <event_type>
  │  X-GitHub-Delivery: <uuid>
  │  body = JSON
  │
  ▼
helix-org HTTP server
  │  1. HMAC-verify against transport.github.webhook_secret
  │  2. Parse body, extract repository.full_name
  │  3. Find Streams with kind=github and config.repo == full_name
  │  4. For each: filter on config.events, build Message, append
  ▼
Stream events ────► dispatcher ────► Worker activation
```

One webhook URL per helix-org installation
(`/github/webhook`), one HMAC secret, fan-out to N Streams
matched by repo. Mirrors Postmark's "one inbound URL, route by
content" pattern.

## Stream config

```json
{
  "repo":   "owner/name",
  "events": ["issues", "issue_comment", "pull_request",
             "pull_request_review", "pull_request_review_comment"]
}
```

- `repo` — required. Matched against `repository.full_name` in
  the payload. Case-insensitive.
- `events` — required, non-empty. Whitelist of GitHub event
  types this Stream wants. Anything not listed is dropped at
  the transport. Filtering here (not in the role) keeps Workers
  from spinning up for events they'll immediately ignore.

Multiple Streams may share a `repo`. A code-review stream and a
docs-review stream on the same repo each get their own webhook
deliveries, filtered by their own `events` list — no
coordination needed.

Future, deferred: `actor` (pre-flag identity matches),
`paths` (pre-filter PR events by changed-file glob),
`labels` (pre-filter to events touching specific labels). All
deliverable as opt-in optimisations; the v1 role does these
checks itself with cheap `gh` calls.

## Webhook URL & signature verification

- **Path:** `/github/webhook`. Single URL for the installation.
  No per-stream URL — repo routing is content-based.
- **Method:** POST. Other verbs return 405.
- **Verification:** HMAC-SHA256 of the raw request body using
  `transport.github.webhook_secret`, compared in constant time
  against `X-Hub-Signature-256`. Mismatch → 401, no append.
- **Body parse:** the JSON is read once, used both for the HMAC
  body and for routing. Don't re-marshal between the two.
- **Response:** 200 with empty body on accepted (or filtered-
  out) events. 401 on bad signature, 400 on unparseable body,
  404 on no matching Stream. GitHub retries on non-2xx; we want
  to fail loudly on bad signatures and silently on "no Stream
  configured for this repo" — return 200 in the latter case so
  GitHub stops retrying.

  *Tension:* 200 on no-match means a misconfigured Stream looks
  successful. Mitigate by logging at INFO when a delivery
  arrives for a repo with zero matching Streams.

## Message envelope mapping

The mapping squeezes the upstream payload into the canonical
envelope without synthesising new prose. Pass through what's
already there; structured stuff lives in `Extra`, which is the
webhook body verbatim.

| Field                | Value |
|----------------------|-------|
| `Message.From`       | `sender.login` from payload — the GitHub user who triggered the event. Empty when the payload has no sender (system events, some pushes). |
| `Message.To`         | empty. (Future: populated from `assignees` / `requested_reviewers` once we have an Identity-B demo to motivate the shape.) |
| `Message.Subject`    | The natural "title" field of the event, used verbatim — `issue.title` for issue events, `pull_request.title` for PR events (including PR-comment and PR-review events, where the parent PR's title is what a reader wants to see), `release.name` for release events. Empty for events that have no title (push, label-only deltas, etc.). |
| `Message.Body`       | The natural "user-typed text" of the event, used verbatim — `issue.body` for `issues.opened` / `issues.edited`, `pull_request.body` for `pull_request.opened` / `pull_request.edited`, `comment.body` for issue and PR-review comments, `review.body` for `pull_request_review.submitted`. Empty when the event carries no user text (label, assigned, sync, push, …). |
| `Message.ThreadID`   | `#<number>` for any event scoped to one issue or PR (including their comments and reviews); empty for repo-level events. Lets a role read all events for one PR via `read_events` filtered by `ThreadID`. |
| `Message.MessageID`  | `X-GitHub-Delivery` — the webhook's UUID. This is GitHub's per-delivery identifier, mirroring how the email transport sets `MessageID` from the SMTP `Message-ID` header. Entity ids (`issue.id`, `pull_request.id`, `comment.id`) stay in `Extra` where they came from. |
| `Message.Extra`      | The full webhook body, **with one synthetic top-level key added**: `event: "<X-GitHub-Event header value>"`. Everything else (`action`, `repository`, `sender`, `issue`/`pull_request`/`comment`/`review`/`label`, …) is GitHub's untouched payload. |

**Roles branch on `Extra.event` and `Extra.action`**. `event`
comes from the HTTP header that GitHub itself uses to
disambiguate; `action` is the field GitHub already puts in the
body. Together they identify the trigger (e.g.
`event=pull_request, action=labeled`), and everything else the
role needs lives at the same JSON paths GitHub documents
(`Extra.label.name`, `Extra.pull_request.number`,
`Extra.comment.body`).

The transport does not generate any prose, does not pre-extract
fields from the payload, and does not invent wrappers around
it. The only thing it adds to the body is the `event` key
(because GitHub put that one piece of identifying information
in a header rather than in the body, and we can't preserve
headers in `Extra` without inventing a sub-namespace). If a
reader wants a one-line digest ("philwinder opened issue #42:
README setup steps…"), they format it themselves from `From` +
`Subject` + `Extra.event` + `Extra.action`. That's not the
transport's job.

## Inbound-only semantics

`publish` to a `github` Stream returns an error:

> `publish` is not supported on github transport streams; use
> `gh` from the Worker's Environment to act on the repo.

Same for any other tool that produces outbound on a Stream
(`reply`, future). The dispatcher's outbound path checks
`Stream.Transport.Kind == "github"` and short-circuits with an
explanatory error rather than no-op silently. Surfaces
mistakes loudly during role authoring.

`subscribe`, `read_events`, and the dispatcher's *inbound*
machinery work as normal — this is just an inbound Stream that
happens to feed events from a remote system instead of `dm` or
the inbound webhook handler.

## Operational config

```json
helix-org config set transport.github '{
  "token":          "<gh-token>",
  "webhook_secret": "<random hex>"
}'
```

- `token` — a GitHub token with at least `Issues: read/write`,
  `Pull requests: read/write`, `Contents: read` on the target
  repos. For the demo, this is the owner's own `gh auth token`;
  for production, a fine-grained PAT scoped to the target repo
  set up manually in the GitHub UI (or, eventually, the
  Identity-B machine user's PAT). Provisioned into each Worker
  Environment's `gh` config at activation time, the same way
  Postmark's token is read at outbound-send time. Never appears
  in chat or activation logs.
- `webhook_secret` — random hex, used for HMAC verification
  only. Generated by the chat agent during demo setup; rotated
  by setting a new value and updating each repo webhook's
  `config.secret`.

Both required. Server logs `github transport enabled` once both
are present at startup.

The same config covers all repos served by the installation.
Per-repo override (different token for different repos) is a
future concern handled by the secrets-management work below.

## Setup and teardown via chat

The first-time setup (token, secret, webhook registration) is
itself a prompt the owner runs in chat. The owner has already
authenticated `gh` on the host; the chat agent uses *that* `gh`
to do everything else:

```
> Set up the github transport for repo <owner>/<repo> on the
> public URL <tunnel-url>.
>
> 1. Read the owner's gh token: `gh auth token`.
> 2. Generate a webhook secret: `openssl rand -hex 32`.
> 3. helix-org config set transport.github with both values.
> 4. Register the webhook on the repo: gh api -X POST
>    /repos/<owner>/<repo>/hooks with events ["issues",
>    "issue_comment", "pull_request", "pull_request_review",
>    "pull_request_review_comment"], url
>    <tunnel-url>/github/webhook, content_type json,
>    secret <generated>.
> 5. Confirm with gh api /repos/<owner>/<repo>/hooks that the
>    hook is active.
```

Teardown reverses it:

```
> Tear down the github transport for <owner>/<repo>:
>
> 1. List webhooks on the repo (gh api /repos/<owner>/<repo>/hooks).
> 2. Find the one with config.url starting <tunnel-url>/github/webhook.
> 3. Delete it: gh api -X DELETE /repos/<owner>/<repo>/hooks/<id>.
> 4. helix-org config delete transport.github.
```

No webhook ID needs to be persisted in helix-org; teardown
discovers it by URL. This survives helix-org restarts and
DB resets — if the webhook somehow outlives the installation,
the same teardown prompt cleans it up.

The token itself is *not* deleted, because it isn't created in
the first place — it's the owner's existing `gh` auth, which
they're going to keep using for everything else.

## Future: secrets per Worker

Today there's one `transport.github.token` for the whole
installation; every Worker on a github stream uses it. The next
step is per-Worker secrets: a `Secret` org-graph object granted
to specific Workers, like tools are. A Worker's `gh` would be
provisioned from whichever `Secret` they hold a grant for. That
unlocks:

- Multiple repos with different tokens (Identity-B machine user
  for repo A, owner's PAT for repo B).
- Different bots with different permissions on the same repo.
- Revoking a specific Worker's access without rotating the
  installation-wide token.

Out of scope for this transport doc; the transport just reads
`transport.github.token` for now. When secrets-per-Worker
lands, the transport flips to reading the Worker's granted
secret instead, and stream config grows a `secret_id` field.
Migration is straightforward because the transport's read of
the token happens at one well-defined point (Worker activation,
provisioning the Environment's `gh`).

## What changes in existing code

1. **`transports/github/`** (new) — package containing:
   - `transport.go`: the inbound HTTP handler, HMAC verify,
     payload parse, Stream lookup, Message build.
   - `summary.go`: per-event-type humanisation for
     `Message.Body`. One function per event type, fallback for
     unknown types.
   - `transport_test.go`: table-driven tests of the HTTP
     handler against captured GitHub webhook fixtures
     (`testdata/<event>.json`).
2. **`server/server.go`** — register `/github/webhook`. Pull
   `transport.github` config at startup; if absent, skip
   registration and log "github transport disabled (no config)".
3. **`store/`** — no schema changes. `Stream.Transport` already
   carries `Kind` + JSON config; `github` is a new value of
   `Kind`.
4. **`tools/publish.go`** — at the top of the outbound path,
   reject `Kind == "github"` with the error message above.
5. **`tools/create_stream.go`** — validate `github` config:
   `repo` non-empty, `events` non-empty list of known GitHub
   event names. Reject unknown event names so typos surface at
   create time, not webhook time.
6. **`config/`** — add `transport.github` to the documented
   config keys.
7. **`design/github-transport.md`** — this file.

`make check` must stay green at every commit.

## Resolved

- **`From` for system events.** Empty when the payload has no
  `sender.login`. `From=""` is already the convention for
  "no human originator" (cron, alerts).
- **Delivery deduplication.** Skip. The transport stays
  fire-and-forget; revisit when we see an actual duplicate
  cause a real problem. `X-GitHub-Delivery` is preserved in
  `Message.MessageID` so a future dedup layer has the key it
  needs.
- **Body curation.** None. `Subject`/`Body` are the upstream
  payload's natural title/text fields, used verbatim. The
  transport invents no prose. Readers that want a digest format
  it themselves.
- **Webhook auto-registration.** Done by the chat agent during
  setup, via the owner's `gh`. No bespoke CLI command. Teardown
  is a separate prompt that discovers the webhook by URL and
  deletes it.

## Open questions

- **`actor` field in stream config.** Pre-flag identity-match
  results in `Extra` (`matches_actor: true` if `assignees` /
  `requested_reviewers` / `@`-mentions include the configured
  handle). Cheap to implement, removes role-side boilerplate.
  Defer until we have an Identity-B demo to motivate the exact
  shape.
- **Org-level webhooks.** GitHub supports webhooks at org level
  with `repository.full_name` in the payload. The transport
  routes by `repo` already, so it would Just Work — no code
  changes. Document and demonstrate later, once we have more
  than one demo repo.
- **Multiple repos per installation, different tokens.** A
  one-token-per-installation model breaks down once Workers on
  different repos need different permissions. The
  secrets-per-Worker direction described above is the planned
  resolution; defer the transport-side mechanics until that
  lands.
