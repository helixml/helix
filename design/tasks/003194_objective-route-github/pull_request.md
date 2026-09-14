# feat(api): route GitHub PR review feedback to spec task agents

## Summary

Closes the feedback loop between GitHub PR reviews (human or bot) and the spec task whose sandbox opened the PR. Previously nothing notified a running task's agent when a review landed on its PR; the agent only saw CI results (poll-based) and design-review comments made inside Helix.

The design mirrors the existing CI notifier path end to end (same prompt-queue sender, same interrupt semantics) with a webhook-driven trigger:

- **Provisioning**: creating a GitHub pull request installs (idempotently, best-effort) a `pull_request_review` webhook on the external repo. A 256-bit HMAC secret is auto-generated per repo on first install and stored on the repo row (`git_repositories.git_hub.webhook_secret`).
- **Endpoint**: `POST /api/v1/webhooks/github/reviews/{repo_id}` on the insecure router. It loads the repo row, validates `X-Hub-Signature-256` with *that repo's* secret (constant-time), fails closed (401) for repos without a secret, 404s unknown repos, acks non-consumable events/actions (ping, push, edited, dismissed), and drops deliveries whose payload names a different repo than the URL's (anti-confusion).
- **Correlation**: new `SpecTaskFilters.PRMatch` (repository_id + PR number, JSONB containment against `repo_pull_requests`) selects the tasks tracking that repo+PR; tasks in done/terminal statuses are skipped.
- **Fetch + normalize**: the review's inline comments are fetched via the repo's own stored credentials through `GitRepositoryService.ListPullRequestReviewComments` (metadata-only repo read — never triggers a clone), filtered to the delivering review's ID. One normalized message per review: PR link/title, reviewer, verdict, review body, inline comments (`path:line author: body`), source link.
- **Delivery**: `enqueueSpecTaskAgentMessage(..., interrupt=true)` — the session-scoped prompt queue used for CI results and reviewer comments; offline agents receive it on reconnect.

**Multi-tenant isolation**: the signing secret is per-repo, so one org's leaked repo secret can only produce deliveries that correlate to tasks tracking that same repo row — it confers no power over other orgs' tasks on a shared deployment. CI feedback already has this property structurally (the orchestrator polls each task's repos with the repo's own credentials; no inbound shared credential), so no CI changes were needed.

**Config**: `GITHUB_INTEGRATION_REVIEW_WEBHOOKS=true` enables the feature (off by default; unset leaves the endpoint inert for any repo). Known ceiling (documented in code): secret rotation takes effect at the repo's next PR creation via the idempotent webhook upsert; no rotation endpoint yet.

Main files: `api/pkg/server/spec_task_github_review_webhook.go` (handler + normalization), `api/pkg/services/git_repository_service{,_pull_requests}.go` (webhook install, per-repo secret, review-comment fetch), `api/pkg/agent/skill/github/client.go` (`UpsertWebhook`, `ListPullRequestReviewComments`), `api/pkg/store/store_spec_tasks.go` + `memorystore` (PRMatch filter), `api/pkg/types/{simple_spec_task,git_repositories}.go`, `api/pkg/config/config.go`, `api/pkg/server/server.go` (route + wiring).

## Testing

- **Unit (Go)**: full `pkg/server` suite, `pkg/services`, `pkg/store/memorystore`, `pkg/types`, `pkg/config` — all green locally (server 13s, services 41s). New `SpecTaskGitHubReviewWebhookSuite` covers: signature verification (valid/tampered/wrong digest/missing/malformed), per-repo secret isolation (a different repo's secret → 401; secretless repo → fail closed; unknown repo → 404), payload-repo mismatch dropped without correlating, ping/push/edited/dismissed acked with no store work, correlation filter contents (repository_id + PR number), done-task skip, and an end-to-end handler run asserting the enqueued `PromptHistoryEntry` (interrupt=true, correct session/spec-task linkage, normalized content including the inline comment).
- **End-to-end (local dev stack)**: seeded org/repo row (with per-repo secret + per-repo GitHub base URL) + spec task tracking PR #42, fake GitHub API container serving review comments, real signed deliveries via curl against the running API:
  - own repo's secret → HTTP 200, exactly one `prompt_history_entries` row with `interrupt=true` and the normalized message (only review 77's inline comment included; another review's comment correctly excluded)
  - different repo's secret → 401
  - own secret but payload claiming another repo → 200 ack, no prompt row
  - unknown repo id → 404
  - unsigned → 401
- Full suites re-run after the per-repo redesign; gofmt clean on all touched files.
