# helix-org SaaS alpha integration

**Status:** design / not started.
**Owner:** phil.
**Triggered by:** helix-org backend work is done (envs, sandboxes, the
heavy lifts). Next step is to put it in front of a handful of internal
users on the SaaS deployment to start dogfooding, without exposing it
to the rest of the user base and without exposing it on self-hosted
deployments at all.

## Problem

helix-org currently runs as its own binary out of
`./helix-org/`, with its own SQLite store and its own HTTP listener on
`:8080` serving `/workers/{id}/mcp` and `/webhooks/{streamID}`. To get
it in front of internal users on SaaS we need:

1. A deploy story — ideally no new service, no new container, no new
   ingress.
2. An access-control story that is **server-enforced**, not just a
   frontend toggle (the frontend can always be tampered with).
3. A way to grant access per-user without a redeploy, and a way for
   self-hosted installs to never see the feature at all.
4. Persistence that survives container redeploys on SaaS.

A frontend-only flag is out. An `HELIX_ALPHA_FEATURES` env var is out
because it's global to the deployment, so any logged-in SaaS user
would see it. Admin-only is out because self-hosted installs would
expose it to their admin.

## Proposed approach

### Same process, same binary

helix-org's HTTP server is already decoupled from networking:
`server.New(...)` (helix-org/server/server.go:45) returns a `*Server`,
and `Server.Handler()` (line 73) returns an `http.Handler` built over
`*http.ServeMux`. Bootstrap is a callable function
(`bootstrap.Run`, helix-org/bootstrap/bootstrap.go:52), idempotent —
returns `ErrAlreadyInitialised` if the owner Worker already exists.

So we embed helix-org as a Go dependency of the helix API binary:

- Add `github.com/helixml/helix-org` to helix's `go.mod` with a
  `replace` directive pointing at `./helix-org` so the in-tree code is
  what gets compiled.
- On helix API startup, after the main store init: open a SQLite store
  via `helix-org/store/sqlite`, build the Registry / Broadcaster /
  Dispatcher, call `bootstrap.Run` (idempotent), construct
  `server.New(...)`, take `orgHandler := orgServer.Handler()`.
- Mount it in `api/pkg/server/server.go` around line 716 alongside
  the existing `authRouter` / `adminRouter`:

  ```go
  authRouter.PathPrefix("/org/").Handler(
      requireFeature("helix-org")(
          http.StripPrefix("/api/v1/org", orgHandler),
      ),
  )
  ```

  `StripPrefix` is required because helix-org's internal routes are
  absolute (`/workers/{id}/mcp`, `/webhooks/{streamID}`).

No new container, no new ingress, no new env var. SaaS gets it; self-hosted
gets it too, but it's gated.

### Per-user feature flag, enforced in middleware

Add an `AlphaFeatures` column to the existing `User` GORM struct at
`api/pkg/types/authz.go:135`:

```go
AlphaFeatures pq.StringArray `gorm:"type:text[];default:'{}'" json:"alpha_features"`
```

GORM AutoMigrate picks it up. No SQL migration file.

Clone the `requireAdmin` pattern at
`api/pkg/server/auth_utils.go:41` into a new
`requireFeature(name string) func(http.Handler) http.Handler` that:

- Pulls the user via `getRequestUser` (`auth_utils.go:142`).
- Returns 403 if the user's `AlphaFeatures` slice does not contain
  `name`.

This is the only gate that matters. The frontend nav entry is purely
cosmetic — tampering with the frontend just produces a 403.

Granting access is one SQL update per user, no deploy:

```sql
UPDATE users
SET alpha_features = array_append(alpha_features, 'helix-org')
WHERE email = 'phil@winder.ai';
```

### Frontend

Extend `UserResponse` at `api/pkg/types/types.go:2411` with
`AlphaFeatures []string`, populate it in the `/auth/user` handler at
`api/pkg/server/auth.go:727`. Frontend gates a new sidebar nav entry
and a route on `user.alpha_features.includes('helix-org')`. Cosmetic
only.

### User identity (deferred)

For the first cut, **every gated user drives the same bootstrapped
owner Worker**. The proxy mount does not forward any user identity to
helix-org; helix-org keeps its current "everyone is owner" stance
internally.

This means the alpha is **not multi-tenant**. Fine for a 2–3-person
internal alpha. Not fine the moment we want to ship to customers.

The later upgrade is to map `helix.User.ID → helix-org.Worker`
(auto-create on first request) and inject the worker ID into the
proxied request. The handler mount is the natural place to do that
when the time comes.

### Storage

helix-org's SQLite file lives at
`${FILESTORE_LOCALFS_PATH}/helix-org/helix-org.db` (derived from the
existing `FILESTORE_LOCALFS_PATH` env var, no new knob). On SaaS that's
the persistent filestore volume, so the file survives container
redeploys.

If `FILESTORE_TYPE != fs` (e.g. `gcs`), startup fails with a clear
error — there is no local path to put the SQLite file in. SaaS currently
runs `fs` against a persistent volume; this is not a constraint in
practice. When we move helix-org's store to Postgres later, the whole
derivation disappears.

## Steps

1. Add `AlphaFeatures pq.StringArray` to `User`
   (`api/pkg/types/authz.go`). Confirm GORM AutoMigrate picks it up on
   a fresh DB.
2. Add `AlphaFeatures []string` to `UserResponse`
   (`api/pkg/types/types.go:2411`) and populate it in the `/auth/user`
   handler (`api/pkg/server/auth.go:727`).
3. Add `requireFeature(name string)` middleware next to
   `requireAdmin` (`api/pkg/server/auth_utils.go:41`). 403 on miss.
4. Add `github.com/helixml/helix-org` to helix's `go.mod` with a
   `replace` directive pointing at `./helix-org`. Confirm
   `go build ./...` from the helix root still works.
5. In `api/pkg/server/server.go`:
   - Resolve the SQLite path from `FILESTORE_LOCALFS_PATH`. Fail
     startup if `FILESTORE_TYPE != fs`.
   - Open the SQLite store, build the Registry / Broadcaster /
     Dispatcher, run `bootstrap.Run` (swallow
     `ErrAlreadyInitialised`).
   - Construct `server.New(...)`, take `orgServer.Handler()`.
   - Mount at `/api/v1/org/` behind
     `requireFeature("helix-org")` + `http.StripPrefix`.
6. Frontend: read `alpha_features` off the user response. Add a
   sidebar entry + route guard for the helix-org view.
7. Grant the column to internal users on SaaS via SQL after deploy.

## Out of scope

- Per-user Worker mapping. Deferred — first cut shares one owner
  Worker across all gated users.
- A real feature-flag service (LaunchDarkly / GrowthBook). The DB
  column is fine for one flag, will be revisited if/when we have
  several or want non-engineer toggling.
- Moving helix-org's store to Postgres. Tracked separately; this work
  doesn't block on it.
- Self-hosted exposure. The column defaults to empty, so self-hosted
  installs never see the nav entry and any direct call returns 403.

## Risks

- **Shared owner Worker.** Every alpha user drives the same org graph.
  Acceptable for the named internal cohort, but we must not widen
  the cohort beyond people who can see each other's state without it
  being a surprise. Document this on the grant SQL.
- **SQLite on a shared volume.** SQLite is fine for a single API
  replica. If SaaS ever scales the API to >1 replica before the
  Postgres migration, helix-org breaks (concurrent writers on the
  same SQLite file). Flag this in the deploy notes.
- **`replace` directive.** Builds depend on the `./helix-org`
  directory being present in the helix checkout. CI already checks
  out the full repo, so this is fine; just note it so nobody tries to
  publish `github.com/helixml/helix` as a standalone module.
