# Implementation Tasks: Scope Project Secrets to Dev, Prod, or Both Environments

## Backend — types & storage
- [x] Add `SecretScope` string type with `dev`/`prod`/`both` consts and a `Valid()` helper in `api/pkg/types/types.go`.
- [x] Add `Scope SecretScope` field to `types.Secret` with GORM tag `type:varchar(16);default:'both';index`.
- [x] Add `Scope string` (omitempty) to `types.CreateSecretRequest`.
- [x] Verify GORM AutoMigrate adds the column; add a one-time `UPDATE secrets SET scope='both'` guard for any NULL/empty rows.
- [x] Update `CreateSecret` uniqueness check in `api/pkg/store/store_secrets.go` to include scope, rejecting same-scope or `both`-overlap name collisions.

## Backend — env-var injection
- [~] Change `GetProjectSecretsAsEnvVars` in `api/pkg/server/secrets_handlers.go` to accept a `scope` arg and include secrets where `scope == arg || scope == both`.
- [~] Update the dev wiring (`SetProjectSecretsGetter` / `ProjectSecretsGetter` in `hydra_executor.go` and `server.go:569`) so the dev path requests `dev`-scoped secrets.
- [~] Add a project-secrets getter to `webservice.Controller` (constructor + field) and set it in `server.go`.
- [~] In `webservice` `runBootstrap`, fetch `prod`-scoped secrets and pass them via `ExecRequest.Env` (not inlined in the shell script); thread the project ID through.

## Backend — API handler
- [ ] In `createProjectSecret`, validate the requested scope (default `both`) and set it on the `Secret`.
- [ ] Confirm `listProjectSecrets` returns `scope` while still nil-ing out `value`.
- [ ] Regenerate swagger/OpenAPI docs for the secret endpoints.

## Frontend
- [ ] Regenerate the API client types so `scope` is available on secret create/list.
- [ ] Add a Dev / Prod / Both scope selector (default Both) to the "Add Secret" dialog in `frontend/src/pages/ProjectSettings.tsx`.
- [ ] Show each secret's scope (chip/label) in the secrets list, and update the helper text to mention dev vs prod injection.

## Tests
- [ ] Store test: same name allowed across differing scopes; rejected for same scope / `both` overlap.
- [ ] Unit test: `GetProjectSecretsAsEnvVars` filters correctly for `dev` and `prod` (each includes `both`).
- [ ] Test: pre-existing secrets default to `both` and appear in both dev and prod injection.
- [ ] Web service deploy test: prod-scoped secrets reach `ExecRequest.Env`; dev-only secrets do not.
