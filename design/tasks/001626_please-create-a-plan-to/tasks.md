# Implementation Tasks

- [ ] Install `gh` CLI (via apt or binary download)
- [ ] Authenticate `gh` using a GitHub PAT from the user (set `GITHUB_TOKEN` env var)
- [ ] Run `gh pr list --repo helixml/helix --author lukemarsden --state open` to find PRs
- [ ] For each PR, post an enthusiastic pick-me-up comment using `gh pr comment`
- [ ] Verify comments were posted by checking one PR in the browser or via `gh pr view`
