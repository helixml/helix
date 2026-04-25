# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

Helix Org — a Go prototype run independently from the `helix` monorepo. It has its own tooling and does not share build/test infrastructure with the parent repo. Rules in the parent `helix/CLAUDE.md` do **not** apply here; treat this directory as its own project.

**Current year: 2026** — include "2026" in web searches for documentation and browser APIs.

## Design Philosophy (read this before writing code)

The ultimate goal is that the system is configured and used **almost entirely via the prompts of specific Roles and Positions**. Behaviour lives in the profile/prompt, not in the codebase. The code is scaffolding that lets this prompt-driven ecosystem thrive.

Practical consequences when choosing between alternatives:

- **Prefer data and text over code.** If a feature can be expressed as a profile edit, a scope value, or a tool description, do that before adding Go logic.
- **Keep the core generic.** Tool definitions, scope shapes, and enforcement decisions are owned by individual tools — not hard-coded in the registry, server, or domain layer. New tools (including MCP tools later) must be addable without editing the core.
- **Keep the MCP surface small.** MCP tools are reserved for system primitives — operations that mutate the org graph (Workers, Positions, Roles, Channels, Grants, Streams). Anything else a Worker needs to do should go through the shell tools provisioned in their Environment (`bash`, `curl`, `git`, `gh`, `python`, etc.). If you're tempted to add an MCP wrapper like `publish_to_blog` or `fetch_url`, stop — the Role text describes how to use the shell directly, and if the workflow changes, only the Role changes. The test: does this operation mutate org-graph state? If yes, MCP. If no, shell.
- **No workflow in code.** Tools do exactly one thing. The code does not orchestrate multi-step sequences on behalf of an agent — it does not subscribe Workers to channels, grant tools implicitly, auto-create related records, or otherwise chain steps together. Orchestration lives in the *prompt* of the Worker invoking the tool. If a Role declares `DefaultTools` or `DefaultStreams`, those fields are **reference data the hiring manager's prompt reads**, not triggers the code acts on. When writing or reviewing a tool, ask: "is the code making a decision that the agent should be making?" If yes, remove it.
- **Write the smallest thing that works.** No speculative abstractions, no optional plumbing that isn't exercised today. If two tools share code, extract it then — not in advance.
- **Social enforcement first.** The default is that a Worker reads their scope from their prompt and complies. Only reach for hard enforcement when the cost of a violation is high.

When a design choice looks like it could go either way, pick the one that pushes more responsibility into prompts/configuration and less into Go code.

## Architecture at a Glance

- **Storage**: SQLite, driven by GORM with `AutoMigrate`. The database file lives alongside the binary by default and is configurable via flag/env. No raw SQL migration files.
- **Interface**: Two HTTP surfaces. Read endpoints (and the one-shot `/bootstrap`) speak [jsonapi.org](https://jsonapi.org) (`application/vnd.api+json`). All mutations flow through MCP at `/workers/{id}/mcp` (Streamable HTTP transport, no auth yet). There is no direct-to-database code path outside the server.
- **CLI**: A thin client binary that runs the server (`helix-org serve`) or performs the one-time bootstrap (`helix-org bootstrap ...`). All other actions are invoked over MCP — point an MCP client (Claude Code, mcp-cli, etc.) at the worker URL you want to act as. The CLI never touches the database directly.
- **Auth**: Deferred. Treat all callers as the root owner for now; real authentication is a later phase.

## Setup

Install required development tools before doing anything else:

```bash
make tools
```

## Build, Test, and Check

Always use `make` commands — never run `go test`, `go vet`, or `golangci-lint` directly. The Makefile sets required build tags, CGO flags, and environment variables that raw `go` commands miss.

```bash
make build                       # Build the binary into ./bin
make run                         # Run the app end-to-end via `go run`
make run ARGS="--foo bar"        # Run with CLI flags
make test                        # Run all tests (race + -count=1)
make test PKG=./domain/...       # Test a specific package
make test-cover                  # Run tests + write coverage.out / coverage.html
make check                       # Format, vet, lint, and test (modifies files)
make ci                          # CI-safe: fmt-check, vet, lint, test (no writes)
```

`make check` is for local use — it runs `goimports -w` and may modify files. `make ci` runs `fmt-check` instead, failing if anything is unformatted without touching files. CI must use `make ci`; contributors must pass `make check` locally before pushing.

## Running the Project End-to-End

```bash
make run                                  # default entry point via `go run`
make run ARGS="--config ./local.yaml"     # pass flags
make build && ./bin/helix-org --help      # compiled binary (matches what CI ships)
```

`make run` is for fast iteration. Before pushing or tagging a release, exercise the compiled binary (`make build && ./bin/helix-org ...`) — the `go run` path can mask build-tag or linker differences.

## Shipping Code

Before committing:

1. Run `make check` and confirm it passes (or at minimum `make test` for affected packages). Do not commit code that has not been validated.
2. Fix any failures before committing — do not skip or work around them.

Commits and PRs use **Conventional Commits**:

- Prefix: `feat:`, `fix:`, `docs:`, `refactor:`, `chore:`, `test:`, etc.
- Example commit: `feat: add webhook retry logic`
- PR titles follow the same format: `feat: add webhook retry logic`

When pushing additional commits to an existing PR, update the PR title and description to reflect the full set of changes in the branch.

## Go

- Fail fast: `return fmt.Errorf("failed: %w", err)` — never log and continue
- Error on missing configuration — fail with an error, don't log a warning and continue
- Use structs, not `map[string]interface{}`
- GORM AutoMigrate only — no SQL migration files
- Use gomock, not testify/mock
- No fallbacks — one approach, no fallback code paths
- No type aliases — update all references when moving or renaming types
- No panics — return errors; rewrite methods to support error returns if needed
- Log errors once at the top level — domain code returns errors, only handlers/workers log them

## Testing

When the user says "tdd", follow red-green strictly:

1. **Red**: Write a failing test. Run it, confirm it fails.
2. **Green**: Minimal fix. Run test, confirm it passes.
3. Run the full test suite for regressions.

Practices:

- **Tests live next to the code**: `foo.go` → `foo_test.go` in the same directory. Use package `foo` for whitebox tests and `foo_test` only when the test must exercise the public API in isolation (e.g. to avoid import cycles).
- **Table-driven tests** for anything with multiple input cases. Use `t.Run(name, ...)` per case so failures name the case.
- **`t.Parallel()`** in tests that don't share global state.
- **Race detector is always on** (`make test` adds `-race`). Treat race failures as hard bugs, never flakes.
- **`-count=1`** is set so tests never use the build cache — all runs are fresh.
- **Mocks**: use `gomock` generated via `mockgen`. Place generated mocks in `mocks/<pkg>/` and regenerate them as part of `make tools` updates, not by hand. Prefer hand-rolled fakes where the interface is small enough that a mock adds no value.
- **Fixtures**: put reusable test data under `testdata/` (Go ignores it during builds).
- **Coverage**: run `make test-cover` before large PRs. Treat coverage as a diagnostic, not a gate — low coverage on a file means "go look," not "write busywork tests."
- **No `testing.Short()` skips by default**. If a test must be slow or external, gate it on an explicit build tag (e.g. `//go:build integration`) and document how to run it.

## Linting

`golangci-lint` config lives in `.golangci.yml`. The enabled linters (`errcheck`, `govet`, `ineffassign`, `staticcheck`, `unused`, `gofmt`, `goimports`, `misspell`, `revive`, `gosec`, `bodyclose`, `errorlint`, `nolintlint`) catch a deliberate, narrow set. Do not disable linters to silence findings — fix the code.

- **Fix the finding, don't suppress it.** `//nolint:<linter>` is only acceptable with a trailing comment explaining *why* the rule is wrong for this site (`nolintlint` enforces this). Unexplained `//nolint` directives fail the lint run.
- **Formatting is non-negotiable**: `goimports` with `-local github.com/helixml/helix-org` groups local imports separately. Run `make fmt` before committing; CI runs `make fmt-check` and will fail on any drift.
- **Error wrapping**: `errorlint` enforces `%w` instead of `%v` for errors, and forbids type-assertion on errors — use `errors.As` / `errors.Is`.
- **`gosec`** flags raw SQL string concatenation, weak crypto, and command injection. If one of these fires, the fix is almost always to restructure the code, not to suppress the warning.
- **New linters** are added by editing `.golangci.yml` in a dedicated `chore: enable <linter>` PR that also fixes all findings it surfaces — never in the same PR as unrelated changes.

## Software Engineering

Object-oriented design principles:

- **Naming**: Classes by what they are, not what they do (avoid -er suffixes). Methods are builders (noun) or manipulators (verb), rarely both. Variables should be explainable as single/plural nouns; prefer simple names over compound ones.
- **Constructors**: One primary constructor, secondaries delegate to it. Keep constructors light. Prefer `new` only in secondary constructors.
- **Methods**: Prefer fewer than five public methods per class. Avoid static methods. Avoid null arguments and return values. Prefer richer encapsulation over getters/setters.
- **Encapsulation**: Prefer four or fewer encapsulated objects per class. Favour composition over inheritance.
- **Interfaces**: Prefer interfaces. Keep them small.
- **Immutability**: Default to immutable classes. Avoid type introspection and reflection unless the language idiom demands it.
- **No globals**: Prefer classes over public constants or enums.
- **Testing**: Prefer fakes over mocks.
- **Design**: Think in objects, not algorithms. Tell objects what you want; don't ask for data.
- **Boolean parameters**: Don't use a boolean to switch between fundamentally different behaviours (split the method or use polymorphism). Booleans are fine for orthogonal modifiers like filters or formatting options.
- **Dependency injection**: Use constructor injection. When a constructor accumulates many parameters, group related ones into a parameter/options object.
- **Language idioms**: Where these principles conflict with strong language conventions (e.g. Go exported struct fields), follow the language idiom and note the deviation.
