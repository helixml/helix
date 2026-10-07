# API key listings no longer return key secrets

Parent audit task: 003592. Fix task: 003611.

## Defect

`GET /api/v1/organizations/{org}/api_keys` returned full `types.ApiKey` rows,
including the plaintext `key`. Org owners list every member's org-scoped keys,
so an owner could read other members' secrets, and an org key authenticates as
the member who created it.

Separately, `GET /api/v1/api_keys` and `POST /api/v1/api_keys` treated a caller
on an org-, project-, spec-task- or session-scoped API key like a browser
session: the GET returned (and auto-created) the owner's unscoped personal key,
and the POST minted a new unscoped key. Either way the scope was dropped. For an
org key this also undid the rule that an org key does not carry its owner's
global admin (`auth_middleware.go`), and for a session key it produced a credential
that outlives the session.

## Fix

### Org key list: metadata only

- New `types.APIKeySummary` (`api/pkg/types/api_key_summary.go`): `id`,
  `key_prefix`, name, owner, owner type, created, type, app/org/project scope.
  No secret.
- `id` is `key_` + the first 12 bytes of SHA-256(secret), hex. `api_keys` is keyed
  by the secret itself, so the ID is derived rather than stored. That means no
  migration, and it is stable for existing rows.
- `key_prefix` is `hl-` plus 4 characters (24 bits of a 256-bit secret), the same
  amount the UI already showed when masking. Secrets shorter than twice that get
  no prefix.
- `listOrgAPIKeys` returns `[]orgAPIKeyResponse{APIKeySummary, owner_email}`.
- `DELETE /organizations/{org}/api_keys/{key}` now takes the key **ID**. It is
  resolved against this org's `api`-type keys, so an ID from another org is a 404.
  Raw secrets are no longer accepted in the URL.
- A member sees their secret once, in the `POST /organizations/{org}/api_keys`
  response. The created-key dialog now says so.

### Scoped keys on `/api/v1/api_keys`

`isScopedAPIKeyCaller` is true for a request authenticated with an API key that
has org/project/spec-task/session/app scope, or a non-`api` key type.

- `GET` with no filter (the "give me my personal key" form) returns 403.
- `GET` with `types=`/`app_id=` returns `api`-type keys reduced to their prefix,
  except the key being presented. App, embed and bot-instance keys are confined by
  the auth middleware, so they are returned unchanged. This keeps
  `helix org bots keys` working with an org key.
- `POST` returns 403 unless it mints an **app** key. App keys only reach the chat
  endpoints (`AppAPIKeyPaths`), and the CLI mints them for bots with whatever key
  it has.

**Decision on the scope question from the audit:** a scoped key must not mint an
unscoped personal key. Inheriting the caller's scope onto the new key was
considered and rejected. For session keys it would tie the new key's lifetime to
session revocation in non-obvious ways. There is also no use case: people create
personal keys from the browser, and org keys from the org page.

`system.DefaultWrapper` now honours an `*HTTPError` status (via `errors.As`)
instead of always returning 500, so these handlers can return 403.

### Endpoints reviewed and left as they are

- `GET /api/v1/api_keys` for browser sessions and personal keys: returns only the
  caller's own keys (`Controller.GetAPIKeys` filters by owner), so nothing crosses
  users.
- `POST /api/v1/admin/users/{id}/api-keys`: global-admin only, by design returns
  the target's key so an orchestrator can act as that user. An org-scoped admin
  key is not admin, so it cannot reach this.
- `GET /api/v1/api_keys/check?key=`: needs the secret to begin with.

## Frontend

- `OrgApiKeys.tsx`: shows `key_prefix...`, no copy button, and rows and delete use
  `id`. The code-examples dialog uses a `<YOUR_API_KEY>` placeholder.
- `CreateSandboxDialog.tsx` / `SandboxApiExamples.tsx`: used to put the first
  org key's secret into the examples. For an owner, that could be another
  member's key. Now passes the prefix of one of the caller's own keys as a hint.
- API client regenerated (`ServerOrgAPIKeyResponse`).

## Verification (inner Helix, `ADMIN_USER_IDS=none-configured`)

Users: alice (owner, org A), bob (member, org A), carol (owner, org B), and admin
(`users.admin=true`). A non-admin got 401 on `GET /api/v1/users`, the admin got 200.

- With the old code, the org A list for alice contained bob's secret, and a scoped
  admin key could obtain an unscoped key that passed admin checks. With the fix,
  neither happens.
- Owner, member and admin listings contain no `key` field. carol listing org A
  gets 403.
- carol deleting bob's key by ID: 403 via org A, 404 via org B. bob deleting
  alice's key: 403. A raw secret in the URL: 404. `api_keys` was unchanged after
  each denied call. alice deleting a member key by ID returned 200 and removed the
  row, and the UI delete worked too.
- Scoped key `GET /api_keys` returned 403, and `POST` returned 403 for both the
  query and body forms. The `api_keys` row count was unchanged. Personal-key and
  cookie-session callers still got 200.
- Regression tests (`org_apikey_handlers_test.go`, `api_key_handlers_test.go`):
  6 fail against the old handlers and all pass with the fix.
