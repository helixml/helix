# Requirements: Scope Project Secrets to Dev, Prod, or Both Environments

## Background

Helix supports **Project Secrets** — encrypted name/value pairs stored per
project and injected as environment variables. Today every project secret is
injected into **dev** containers (interactive project sessions, spec tasks,
exploratory sessions via `HydraExecutor`).

The recently merged **web service** feature (PR #2096) lets a project deploy a
long-running hosted web app (a "prod" sandbox provisioned by
`api/pkg/webservice/controller.go`). Prod and dev often need *different* values
for the same secret (e.g. a dev vs prod database URL or Stripe key), and some
secrets should only ever exist in one environment.

This task makes each Project Secret carry an **environment scope** — `dev`,
`prod`, or `both` — controlling where it is injected.

## User Stories

**US-1 — Choose a scope when creating a secret**
As a project owner, when I add a project secret I can choose whether it applies
to Dev, Prod, or Both, so I can keep environment-specific credentials separate.

**US-2 — Different value per environment**
As a project owner, I can create two secrets with the same name — one scoped
`dev` and one scoped `prod` — so each environment gets the correct value.

**US-3 — Dev containers only see dev/both secrets**
As a developer, my interactive project sessions and spec tasks receive only
secrets scoped `dev` or `both`, never `prod`-only secrets.

**US-4 — Prod web service only sees prod/both secrets**
As a project owner, my deployed web service container receives only secrets
scoped `prod` or `both`, never `dev`-only secrets.

**US-5 — Existing secrets keep working**
As an existing user, all secrets I created before this change continue to behave
exactly as before (injected into dev), with no manual migration.

**US-6 — See and edit scope in the UI**
As a project owner, the Secrets tab shows each secret's scope and lets me set it
when adding a secret.

## Acceptance Criteria

- A `Secret` has a scope field with allowed values `dev`, `prod`, `both`.
- Creating a project secret accepts an optional scope; default is `both`.
- Two secrets may share a name within one project **only** if their scopes
  differ (uniqueness is now scoped by `(owner, project_id, app_id, name, scope)`,
  or names with overlapping scopes are rejected — see design).
- Dev injection (`HydraExecutor`) includes secrets where scope is `dev` or
  `both`; excludes `prod`-only secrets.
- Prod injection (web service deploy) includes secrets where scope is `prod` or
  `both`; excludes `dev`-only secrets.
- All pre-existing secrets are treated as scope `both` after migration (so dev
  behaviour is unchanged and they also flow to prod).
- The Secrets tab in Project Settings displays each secret's scope and the
  "Add Secret" dialog includes a scope selector (Dev / Prod / Both).
- Secret values are never returned by the API; only metadata (name, scope) is
  listed.

## Out of Scope

- Per-secret rotation, versioning, or audit history.
- App-level (non-project) secret scoping.
- Staging or arbitrary custom environments beyond dev/prod/both.
