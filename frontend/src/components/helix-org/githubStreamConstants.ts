// githubStreamConstants holds the small bits of GitHub-transport
// metadata shared between the New Stream dialog and the per-stream
// Edit dialog: the curated event whitelist with rich descriptions,
// and the regex patterns that mirror the backend's
// transport/github.go validator.
export type EventOption = { value: string; help: string }

export const GITHUB_EVENT_OPTIONS: EventOption[] = [
  {
    value: 'push',
    help: 'Commits pushed to a branch (or tag). This is the "code changed" event. Use the Branches filter below to scope to specific branches (e.g. main, release/*).',
  },
  {
    value: 'create',
    help: 'A branch or tag was created.',
  },
  {
    value: 'delete',
    help: 'A branch or tag was deleted.',
  },
  {
    value: 'release',
    help: 'A release was published, edited, or deleted.',
  },
  {
    value: 'workflow_run',
    help: 'A GitHub Actions workflow run started or finished (requested, in_progress, completed).',
  },
  {
    value: 'workflow_job',
    help: 'An individual GitHub Actions job was queued, started, or completed.',
  },
  {
    value: 'check_run',
    help: 'A check run (CI status check) was created, updated, or completed.',
  },
  {
    value: 'status',
    help: 'A commit status changed (the older statuses API: pending / success / failure).',
  },
  {
    value: 'deployment_status',
    help: 'A deployment status changed (e.g. a deploy succeeded or failed).',
  },
  {
    value: 'discussion',
    help: 'A GitHub Discussion was created, edited, answered, etc.',
  },
  {
    value: 'discussion_comment',
    help: 'A comment on a GitHub Discussion.',
  },
  {
    value: 'fork',
    help: 'The repository was forked.',
  },
  {
    value: 'star',
    help: 'A star was added to or removed from the repository.',
  },
  {
    value: 'label',
    help: 'A label was created, edited, or deleted in the repository.',
  },
  {
    value: 'milestone',
    help: 'A milestone was created, closed, edited, or deleted.',
  },
  {
    value: 'issues',
    help: 'Issue lifecycle: opened, closed, reopened, labeled, assigned, milestoned, etc. Fires for every change to the issue itself (not comments on it).',
  },
  {
    value: 'issue_comment',
    help: 'Comments on issues AND on the PR conversation tab (GitHub treats PR conversation comments as issue comments). NOT line-level review comments.',
  },
  {
    value: 'pull_request',
    help: 'PR lifecycle: opened, synchronize (new commits pushed), closed, merged, ready_for_review, reopened, edited, labeled, assigned, etc.',
  },
  {
    value: 'pull_request_review',
    help: 'A review was submitted on a PR — approve / request changes / comment-review. Fires once per review submission, not per inline comment.',
  },
  {
    value: 'pull_request_review_comment',
    help: 'Line-level inline comments inside a PR diff (the ones attached to a specific file + line, as opposed to the PR conversation tab).',
  },
]

// Lowercase letters, digits and underscores, 2-64 chars, starting
// with a letter. Mirror of githubEventNamePattern in
// api/pkg/org/domain/transport/github.go.
export const GITHUB_EVENT_PATTERN = /^[a-z][a-z0-9_]{1,63}$/

// isValidGitHubEvent mirrors the backend validator, which accepts the "*"
// wildcard as a special case in addition to the slug pattern. Use this for
// validation rather than GITHUB_EVENT_PATTERN alone (which rejects "*").
export const isValidGitHubEvent = (e: string) => e === '*' || GITHUB_EVENT_PATTERN.test(e)

// Exactly one slash, both halves non-empty. Mirror of the backend's
// repo check.
export const GITHUB_REPO_PATTERN = /^[^/\s]+\/[^/\s]+$/

export const eventValue = (o: EventOption | string) =>
  typeof o === 'string' ? o : o.value
