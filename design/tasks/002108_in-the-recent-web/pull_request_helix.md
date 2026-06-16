# Scope project secrets to dev, prod, or both environments

## Summary

Project secrets are encrypted env vars injected into a project's containers.
Until now every project secret was injected into **dev** containers (interactive
sessions / spec tasks), and the recently added **web service** ("prod") deploy
received no project secrets at all. Prod and dev often need different values for
the same key (e.g. a dev vs prod database URL), so this adds a per-secret
**environment scope** — `dev`, `prod`, or `both` — that controls where each
secret is injected.

`both` is the default, so existing secrets keep working: they continue to flow
to dev and now also reach the prod web service (previously prod got nothing).

## Changes

- **Types**: new `SecretScope` (`dev`/`prod`/`both`) with `Valid()` and
  `AppliesTo()` helpers; `Scope` field on `Secret` (GORM `default:'both'`) and on
  `CreateSecretRequest`.
- **Store**: `CreateSecret` defaults empty scope to `both` and allows the same
  secret name across non-overlapping scopes (a `dev` and a `prod` secret can
  share a name), while still rejecting overlapping collisions. AutoMigrate adds
  the column; a one-time idempotent backfill normalises legacy rows to `both`.
- **Injection**: `GetProjectSecretsAsEnvVars` now filters by target environment.
  The dev path (HydraExecutor) requests `dev`-scoped secrets; the web service
  deploy requests `prod`-scoped secrets and injects them via
  `hydra.ExecRequest.Env` (not inlined into the bootstrap shell script, so values
  don't leak into command logs).
- **API**: `createProjectSecret` validates and persists the scope (default
  `both`); swagger/OpenAPI + TypeScript client regenerated.
- **Frontend**: Add-Secret dialog gains a Dev/Prod/Both selector (default Both);
  each secret row shows a coloured scope chip.
- **Tests**: scope validity/filtering (`AppliesTo`), store uniqueness across
  scopes + defaulting (passes against live DB), and prod-secret env injection
  for the web service controller.

## Verification

- `go build ./pkg/...` clean; `go test ./pkg/types/ ./pkg/webservice/` pass;
  store secret tests pass against the dev Postgres (also exercises the backfill).
- Frontend type-checked with `yarn tsc` (clean).
