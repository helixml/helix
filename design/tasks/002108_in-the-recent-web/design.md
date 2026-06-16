# Design: Scope Project Secrets to Dev, Prod, or Both Environments

## Current State (discovered in codebase)

**Type** — `api/pkg/types/types.go:2438` `type Secret struct{...}` with
`Owner, OwnerType, Name, Value []byte, AppID, ProjectID`. GORM-managed table,
auto-migrated at `api/pkg/store/postgres.go:183` (`&types.Secret{}` in the
`AutoMigrate` list — **adding a column is automatic**, no SQL migration needed).

**Store** — `api/pkg/store/store_secrets.go`:
- `CreateSecret` enforces uniqueness on `(owner, name, project_id, app_id)`
  inside a transaction (line ~29).
- `ListProjectSecrets(ctx, projectID)` returns all secrets for a project.

**HTTP handlers** — `api/pkg/server/secrets_handlers.go`:
- `createProjectSecret` (line ~310) builds a `Secret` from
  `types.CreateSecretRequest`, encrypts the value, stores it.
- `listProjectSecrets` (line ~268) lists project secrets, **nils out values**.
- `GetProjectSecretsAsEnvVars(ctx, projectID)` (line ~368) decrypts secrets and
  returns `KEY=value` strings.

**Dev injection (currently the only consumer of project secrets)** —
`GetProjectSecretsAsEnvVars` is wired to `HydraExecutor` via
`SetProjectSecretsGetter` (`api/pkg/server/server.go:569`). In
`api/pkg/external-agent/hydra_executor.go:170` secrets are appended to
`agent.Env` for every dev/desktop container.

**Prod injection (web service) — currently MISSING.**
`api/pkg/webservice/controller.go` `runBootstrap` (line ~210) only sets
`HELIX_WEB_SERVICE_PORT` before running `.helix/startup.sh`; it injects **no**
project secrets today. The hydra `ExecRequest` already supports an `Env []string`
field (`api/pkg/hydra/sandbox_ops.go:250`), so prod injection can be added.

**Frontend** — `frontend/src/pages/ProjectSettings.tsx` `renderSecretsTab`
(line ~1754) + "Add Secret" dialog (line ~2125). Uses generated API client
`v1ProjectsSecretsDetail` / `v1ProjectsSecretsCreate`.

## Key Decisions

### 1. Add a `Scope` field, default `dev`
Add to `types.Secret`:
```go
Scope SecretScope `json:"scope" yaml:"scope" gorm:"type:varchar(16);default:'dev';index"`
```
**Default `dev`** (user decision): Helix is primarily a dev platform with prod
web hosting as a secondary feature, and dev-only also preserves the pre-feature
behaviour *exactly* (project secrets were only ever injected into dev). Legacy
rows are backfilled to `dev`, so nothing starts leaking into prod web services.
with
```go
type SecretScope string
const (
    SecretScopeDev  SecretScope = "dev"
    SecretScopeProd SecretScope = "prod"
    SecretScopeBoth SecretScope = "both"
)
```
Any request that omits scope is coerced to `dev`. GORM AutoMigrate adds the
column with the `dev` default; a one-time idempotent
`UPDATE secrets SET scope='dev' WHERE scope IS NULL OR scope=''` backfills any
legacy NULL/empty rows.

### 2. Filter at the env-var boundary, not in storage
Change `GetProjectSecretsAsEnvVars` to take a scope:
```go
func (s *HelixAPIServer) GetProjectSecretsAsEnvVars(ctx, projectID string, scope types.SecretScope) ([]string, error)
```
It includes a secret when `secret.Scope == scope || secret.Scope == SecretScopeBoth`.
- The dev wiring (`SetProjectSecretsGetter`) passes `SecretScopeDev`.
- The web service controller passes `SecretScopeProd`.

`ProjectSecretsGetter` signature in `hydra_executor.go:29` gains the scope arg;
the dev call site at line ~171 passes `dev`. (Alternatively, bind the dev getter
to dev at wire-up time to avoid changing the func type — pick whichever keeps the
diff smaller; binding at wire-up is preferred.)

### 3. Wire prod injection into the web service deploy
Give `webservice.Controller` a secrets getter (same `func(ctx, projectID, scope)`
signature), set at construction in `server.go`. In `runBootstrap`, fetch
prod-scoped env vars and pass them via the `ExecRequest.Env` field (NOT inlined
into the shell script, to avoid leaking values into logs):
```go
env := []string{fmt.Sprintf("HELIX_WEB_SERVICE_PORT=%d", containerPort)}
if c.getProjectSecrets != nil {
    secretEnv, _ := c.getProjectSecrets(ctx, project.ID, types.SecretScopeProd)
    env = append(env, secretEnv...)
}
// ExecRequest{... Env: env}
```
`runBootstrap`/`runDeploy` already have the project ID; thread it through.

### 4. Name uniqueness across scopes
To allow a `dev` and a `prod` secret with the same name (US-2), extend the
uniqueness check in `CreateSecret` to include scope:
`(owner, name, project_id, app_id, scope)`. Reject a create that would collide
with an existing secret of the **same** scope, or with a `both`-scoped secret of
the same name (since `both` already covers that environment). Keep the friendly
error message.

### 5. API request/UI
- Add `Scope string` to `types.CreateSecretRequest` (omitempty; default `both`
  applied server-side). `createProjectSecret` validates it is one of the three
  values and sets it on the `Secret`.
- `listProjectSecrets` already nils values; `Scope` is safe metadata and is
  returned.
- Regenerate the frontend API types (swagger) so `scope` appears.
- UI: the "Add Secret" dialog gets a Dev / Prod / Both selector (default Both);
  the secrets list shows a small scope chip per row.

## Data / Control Flow

```
Create:  UI dialog (name,value,scope) → POST /projects/{id}/secrets
         → encrypt value → CreateSecret (uniqueness incl. scope)

Dev:     HydraExecutor.OnBeforeCreate → getProjectSecrets(projectID, dev)
         → secrets where scope ∈ {dev, both} → agent.Env

Prod:    webservice.runBootstrap → getProjectSecrets(projectID, prod)
         → secrets where scope ∈ {prod, both} → ExecRequest.Env → startup.sh
```

## Gotchas / Notes

- Project secrets are encrypted at rest (`crypto.EncryptAES256GCM`); decryption
  happens only inside `GetProjectSecretsAsEnvVars`. Keep that boundary.
- Do not inline secret values into the bootstrap shell script — use
  `ExecRequest.Env` so values don't land in command logs.
- Default is `dev` (user decision). This is the most backward-compatible choice:
  pre-existing secrets were dev-only, and they stay dev-only — nothing newly
  leaks into prod web services. Prod secrets are strictly opt-in.
- Keep allowed-value validation centralized (a helper like
  `SecretScope.Valid()`).

## Implementation Notes (as built)

- **Types** (`api/pkg/types/types.go`): added `SecretScope` with `Valid()` and
  `AppliesTo(target)` helpers, `Scope` field on `Secret` (GORM
  `default:'both';index`), and `Scope` on `CreateSecretRequest`.
- **Store** (`store_secrets.go`): `CreateSecret` defaults empty scope to `both`
  and rejects name collisions whose scopes overlap (`scopesOverlap` helper —
  `both` overlaps everything). `postgres.go` backfills `scope='both'` for
  NULL/empty rows after AutoMigrate (idempotent).
- **Env injection** (`secrets_handlers.go`): `GetProjectSecretsAsEnvVars` now
  takes a `target types.SecretScope` and filters via `scope.AppliesTo(target)`.
  Wired in `server.go` with two closures — dev (HydraExecutor) bound to
  `SecretScopeDev`, prod (web service) bound to `SecretScopeProd`. The
  `ProjectSecretsGetter` func types stayed `(ctx, projectID)` so only the
  binding changed, not every call site.
- **Web service** (`webservice/controller.go`): added `ProjectSecretsGetter`
  field + `SetProjectSecretsGetter`. `runBootstrap` gained a `projectID` param
  and injects prod secrets via `hydra.ExecRequest.Env` (extracted into the
  testable `projectSecretEnv` helper). Secrets are NOT inlined into the bootstrap
  shell script — `exec env HELIX_WEB_SERVICE_PORT=... bash startup.sh` inherits
  the exec process env, so secrets propagate to startup.sh without hitting logs.
- **Frontend** (`ProjectSettings.tsx`): regenerated API client exposes
  `TypesSecretScope`. Added an Environment select (default Both) to the Add
  Secret dialog and a coloured scope chip per secret row (`secretScopeLabel`
  helper). Verified with `yarn tsc` (clean).
- **Gotcha**: local `frontend/dist/` is a root-owned read-only bind mount, so
  `yarn build` fails at the output-copy step (transform succeeds). Use
  `yarn tsc` for type verification locally.
- **Gotcha**: `swag` installs to `$(go env GOPATH)/bin`, which isn't on PATH in
  this env — export it before running `swag init`.
- **Verification**: `go build ./pkg/...` clean; `go test ./pkg/types/
  ./pkg/webservice/` pass; store secret tests pass against the live dev Postgres
  (also exercises the backfill migration).
- **Default scope `dev`** (changed from an initial `both` after user feedback):
  Helix is primarily a dev platform; prod web hosting is a secondary feature.
  This also means pre-existing secrets keep their exact dev-only behaviour and
  prod secrets are strictly opt-in — no behaviour change for existing users.
