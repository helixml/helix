// githubStreamConstants holds the small bits of GitHub-transport
// metadata shared between the New Stream dialog and the per-stream
// Edit dialog: the curated event whitelist with rich descriptions,
// and the regex patterns that mirror the backend's
// transport/github.go validator.
export type EventOption = { value: string; help: string }

export const GITHUB_EVENT_OPTIONS: EventOption[] = [
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

// Exactly one slash, both halves non-empty. Mirror of the backend's
// repo check.
export const GITHUB_REPO_PATTERN = /^[^/\s]+\/[^/\s]+$/

export const eventValue = (o: EventOption | string) =>
  typeof o === 'string' ? o : o.value
