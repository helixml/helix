# Requirements

## User Stories

- As a user, I want the `gh` CLI tool installed so I can interact with GitHub from the command line.
- As a user, I want all open PRs authored by `lukemarsden` to receive an enthusiastic, encouraging comment so he feels appreciated and motivated.

## Acceptance Criteria

1. `gh` CLI is installed and available in PATH.
2. `gh` is authenticated with a GitHub PAT (provided by user if needed).
3. All open PRs authored by `lukemarsden` across the relevant repository (or all repos) are found.
4. Each PR receives a friendly, enthusiastic pick-me-up comment (e.g. "Great work on this PR! Keep it up! 🚀").
5. No duplicate comments are added if the script is re-run (optional nice-to-have).

## Notes

- User has offered to provide a temporary PAT for authentication — request one if needed.
- The script should be idempotent where feasible.
