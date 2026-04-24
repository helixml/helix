# Contributing to Helix

Thanks for your interest in contributing to Helix! This document covers the contribution workflow. For setting up a development environment, see [`local-development.md`](./local-development.md).

## Where to start

- **Bugs and feature requests**: open a discussion in [Discord](https://discord.gg/VJftd844GE) - the team triages there first.
- **Docs**: typo fixes and small clarifications can go straight to a PR.
- **Larger changes**: please discuss the design in Discord before starting work, so we can flag any conflicts with in-flight features.

## Development setup

See [`local-development.md`](./local-development.md) for the full walkthrough. The short version:

```bash
git clone git@github.com:helixml/helix.git
cd helix
cp .env.example-prod .env       # then edit OPENAI_* if using an external LLM
./stack start
```

Open <http://localhost:8080> and register - in dev mode the first user is auto-promoted to admin.

## Branching and pull requests

1. **Fork** the repo if you're an external contributor; otherwise create a branch directly.
2. **Branch from `main`**, using a descriptive name:
   ```bash
   git checkout -b feature/short-description
   git checkout -b fix/short-description
   ```
3. **Commit in small, atomic changes** with clear messages (see [Commit messages](#commit-messages) below).
4. **Open a PR against `main`**. Use the full URL form when referencing other PRs or issues (e.g. `https://github.com/helixml/helix/pull/123`).
5. **Keep PRs focused** - one logical change per PR. Refactors should be separate from behaviour changes.
6. **Test your change end-to-end** before requesting review. If something is genuinely untestable in your environment, say so explicitly in the PR description.

We use **regular merge commits** (not squash) - the merge UI is set accordingly.

## Commit messages

Follow the existing style in the repo:

```
Short imperative summary (<=72 chars)

Optional body explaining the *why*. Wrap at ~72 cols. Reference
PRs/issues with full URLs:
  https://github.com/helixml/helix/pull/123
```

Avoid:

- Customer or contact names in commit messages or PR descriptions - use generic phrasing.
- Unsubstantiated claims about severity ("critical fix", "major refactor") without evidence.

## Code style

### Go

- Format with `gofmt` (most editors do this on save).
- Lint with `./stack lint` (requires [`golangci-lint`](https://golangci-lint.run/welcome/install/)).
- Wrap errors with context: `return fmt.Errorf("doing X: %w", err)` - never log and continue.
- Prefer typed structs for API payloads over `map[string]interface{}`.
- Use `gomock` for mocks (not `testify/mock`).
- Don't add fallbacks or "just in case" code paths - one approach, fix it properly.

### TypeScript / React

- Build / type-check with `cd frontend && yarn build` before pushing.
- Lint with `cd frontend && yarn lint`.
- Use the **generated API client** (`api.getApiClient()`) - never call `fetch` directly. If an endpoint is missing, add swagger annotations to the Go handler, run `./stack update_openapi`, then use the regenerated method.
- Use **React Query** for all API calls; invalidate queries after mutations.
- Routing uses **react-router5** via `useRouter()` - prefer `router.navigate('name', { params })` over raw `<a>` / `<Link>`.

## Tests

- Add tests for new behaviour. We use Go's standard `testing` package with `testify/suite` and `gomock` for table-driven and mock-heavy tests respectively.
- For full local runs: `./stack test [./path/...]` (boots Postgres / Chrome via compose).
- For a quick build-only sanity check: `cd api && go build ./...`.
- Frontend doesn't have heavy unit-test coverage; manual end-to-end verification in the browser is expected for UI changes.

CI (Drone) runs the full suite on every PR.

## Documentation

- **Code docs**: don't add docstrings or comments unless the *why* is non-obvious. Well-named identifiers are the documentation.
- **User docs**: live at <https://helix.ml/docs> in a separate repo.
- **Design docs**: longer-form notes about non-trivial work go in `design/YYYY-MM-DD-name.md`.

## Licensing

By contributing you agree that:

- Your changes will be released under the same [license](./LICENSE.md) as the rest of the project.
- Copyright in your contributions will be assigned to HelixML, Inc.

See the [README](./README.md#-license) for the license summary.

## Getting help

- [Discord](https://discord.gg/VJftd844GE) - the fastest way to reach maintainers and other contributors.
- [Documentation](https://helix.ml/docs) - product and deployment docs.
