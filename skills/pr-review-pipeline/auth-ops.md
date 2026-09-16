# GitHub auth ops doctrine

Learned in production on the live pipeline. Anyone operating the coordinator or a reviewer task follows this — most mysterious "token broke" failures are this, in this pattern.

## 1. Fresh token per auth batch

`get_secret` the GitHub token **immediately before each batch of authed calls**. Never reuse a token exported earlier in a task, and never across steps. Installation tokens are short-lived; the broker mints a fresh one per fetch, and a stale export is the #1 source of mid-review 401s. Never print the token in a command that echoes argv.

## 2. One probe, then exactly one re-mint

Health check is a **read** (`GET /repos/REPLACE_REPO`), not `/user` — a 403 from `.../user` is *normal* for installation tokens and proves nothing.

On 401 "Bad credentials" right after a fresh mint (known intermittent broker fault): re-mint **once** (`get_secret` again) and retry **once**. Then stop. **Never POST-loop** — a repeated POST can half-apply server-side even when the client sees an error, so blind retry loops risk duplicate reviews on the PR.

## 3. Persistent failure = stop with verbatim headers

If the failure survives the single re-mint, do not improvise fixes. Stop and paste, verbatim, into the report / `ask_human`:

- HTTP status
- response `message`
- `x-github-request-id`
- `x-accepted-github-permissions` (what GitHub *expected* the token to have — this is the fastest diagnostic when the grant is wrong)

A report with those four lines gets fixed in one round-trip; a report that says "token was 401, tried again" does not.

## 4. 403 "Resource not accessible by integration" = installation fault

This is **not** a permissions problem to go fix in the GitHub UI. The GitHub UI cannot show you the fix, and "adding" the permission it seems to want will change nothing. It means the GitHub App installation/connection behind the token is at fault: repo outside the installation's scope, installation revoked/transferred, or the webhook/app wiring stale. Route it to the org owner as an installation/connection problem.

## 5. Anonymous reads are fine on public repos

Verifying state (does a review exist? what is the head SHA?) needs no auth on a public repo — plain `curl` without headers works. Use it to verify when a token path is suspect; a token failure must never stop you from *checking* whether a review already exists (that's what prevents double-posting).

## 6. Salvage pattern — the review survives the token dying

A reviewer that finished its analysis but cannot post (auth dead for any reason) does **not** lose the work:

1. Write the finished verdict + the exact review body (and its inline `comments`) to its own workspace disk: the body file plus a `review.json` manifest `{ "event": "APPROVE|COMMENT", "commit_id": "<sha reviewed>", "body_file": "<path>" }`.
2. Report that the verdict is on disk, with path and PR/SHA, and mark the task as needing a fresh dispatch to post.
3. A fresh task (fresh sandbox, fresh token) runs [scripts/post_review.sh](scripts/post_review.sh) against that `review.json` and posts it.

The payload persists on the task's workspace; a broken token costs at most a re-dispatch, never the review.

## 7. Only verified posts count

An activation ends with "review posted" **only** if the reviews API shows exactly one review by `REVIEWER_LOGIN` at the target commit. [scripts/post_review.sh](scripts/post_review.sh) encodes the whole rule: one POST attempt, then GET-verify; nonzero exit (never "success") unless verification passes. Never claim a review you cannot verify.
