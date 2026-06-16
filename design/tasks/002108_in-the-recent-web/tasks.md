# Implementation Tasks: Scope Project Secrets to Dev, Prod, or Both Environments

## Backend — types & storage
- [x] Add `SecretScope` string type with `dev`/`prod`/`both` consts and a `Valid()` helper in `api/pkg/types/types.go`.
- [x] Add `Scope SecretScope` field to `types.Secret` with GORM tag `type:varchar(16);default:'both';index`.
- [x] Add `Scope string` (omitempty) to `types.CreateSecretRequest`.
- [x] Verify GORM AutoMigrate adds the column; add a one-time `UPDATE secrets SET scope='both'` guard for any NULL/empty rows.
- [x] Update `CreateSecret` uniqueness check in `api/pkg/store/store_secrets.go` to include scope, rejecting same-scope or `both`-overlap name collisions.

## Backend — env-var injection
- [x] Change `GetProjectSecretsAsEnvVars` in `api/pkg/server/secrets_handlers.go` to accept a `scope` arg and include secrets where `scope == arg || scope == both`.
- [x] Update the dev wiring (bound to `dev` via a closure in `server.go`) so the dev path requests `dev`-scoped secrets.
- [x] Add a project-secrets getter to `webservice.Controller` (field + `SetProjectSecretsGetter`) and set it in `server.go` (bound to `prod`).
- [x] In `webservice` `runBootstrap`, fetch `prod`-scoped secrets and pass them via `ExecRequest.Env` (not inlined in the shell script); thread the project ID through.

## Backend — API handler
- [x] In `createProjectSecret`, validate the requested scope (default `both`) and set it on the `Secret`.
- [x] Confirm `listProjectSecrets` returns `scope` while still nil-ing out `value` (Scope is JSON-serialised metadata; no change needed).
- [x] Regenerate swagger/OpenAPI docs + TypeScript client for the secret endpoints (`./stack update_openapi` equivalent).

## Frontend
- [x] Regenerate the API client types so `scope` is available on secret create/list (`TypesSecretScope` enum + `scope` field).
- [x] Add a Dev / Prod / Both scope selector (default Both) to the "Add Secret" dialog in `frontend/src/pages/ProjectSettings.tsx`.
- [x] Show each secret's scope (chip/label) in the secrets list, and update the helper text to mention dev vs prod injection.

## Change default scope to `dev` (user feedback)
- [x] Default scope is `dev` not `both` — Helix is primarily a dev platform; prod web hosting is secondary. Also strictly preserves pre-feature behaviour (legacy secrets were dev-only). Updated GORM default, CreateSecret default, backfill (`scope='dev'`), handler default, env coercion, frontend default + dialog order + label, and docs/tests. Builds + go/tsc tests green.

## Tests
- [x] Store test (`TestSecretScopeUniqueness`): same name allowed across differing scopes; rejected for same scope / `both` overlap; omitted scope defaults to `both`. Passes against live DB.
- [x] Unit test (`TestSecretScopeAppliesTo`/`TestSecretScopeValid`): scope filtering — `dev` and `prod` each include `both`, exclude the other.
- [x] Defaulting covered by store test (omitted scope → `both`) + AutoMigrate backfill (verified by store suite running against live DB).
- [x] Web service injection test (`TestProjectSecretEnv`): prod getter results reach the exec env; missing getter / empty project / getter error don't block deploy. (Full `runBootstrap` is not interface-mockable without new abstractions — extracted `projectSecretEnv` helper instead.)
