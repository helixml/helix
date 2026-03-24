# Design

## Approach

A simple bash script using the `gh` CLI to find and comment on PRs. No wrappers or abstractions needed.

## Steps

1. **Install `gh`**: Use the official APT repo for Linux (`apt install gh`), or download the binary directly.
2. **Authenticate**: Run `gh auth login` with a PAT provided by the user (via `GITHUB_TOKEN` env var or interactive prompt).
3. **Find PRs**: Use `gh pr list` or `gh search prs` to find all PRs authored by `lukemarsden`. Scope to relevant repo(s).
4. **Comment**: For each PR, run `gh pr comment <number> --body "..."` with an enthusiastic message.

## Key Decisions

- **Scope**: Default to the `helixml/helix` repository (primary project). Can be expanded to all repos if needed.
- **Comment style**: Short, warm, enthusiastic — e.g. "Amazing work on this PR, Luke! You're on a roll! 🙌🎉"
- **Auth**: Use `GITHUB_TOKEN` env var so the PAT is not stored on disk or in scripts.
- **Duplicate prevention**: `gh pr comment` always adds a new comment; to avoid duplicates on re-runs, check existing comments first using `gh api` — include as optional step.

## Implementation Pattern

```bash
export GITHUB_TOKEN=<pat>
REPO="helixml/helix"

for PR in $(gh pr list --repo $REPO --author lukemarsden --state open --json number -q '.[].number'); do
  gh pr comment $PR --repo $REPO --body "Amazing work on this PR, Luke! You're crushing it! 🚀🙌"
done
```
