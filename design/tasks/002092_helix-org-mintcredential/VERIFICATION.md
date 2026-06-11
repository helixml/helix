# Verification

## What was verified automatically

### Build
- `CGO_ENABLED=0 go build ./api/...` — clean.
- `CGO_ENABLED=0 go build ./api/pkg/org/... ./api/pkg/server/...` — clean.

### Tests
- `CGO_ENABLED=1 go test -count=1 ./api/pkg/org/...` — all packages green.
- New: 5 tests in `api/pkg/org/infrastructure/transports/github/credential_provider_test.go`.
- New: 8 tests in `api/pkg/org/application/tools/mint_credential_test.go`.
- Updated: 4 tests in `api/pkg/server/helix_org_github_app_test.go` (mintFn signature widened).

### Inner Helix (runtime)
- API container in inner Helix successfully Air-rebuilt with the new wiring.
  Confirmed by docker logs: `building...` followed by no `failed to build` line
  and the API serving HTTP 200 on `http://localhost:8080/`.
- Earlier Air-build error `pkg/server/helix_org.go:308:40: undefined:
  credential` (missing import) was already fixed before this note — the
  current build is clean.

## Tool exposure (verified by code path, not at runtime)

- `MintCredential` is registered in `tools.RegisterBuiltins` between
  `CreateStream` and `StreamMembers`. Every Worker whose Role lists
  `mint_credential` sees it on its MCP `tools/list` response — same path
  every other built-in flows through.
- The owner Role (created at org bootstrap) lists `mint_credential` in its
  tools — verified by reading `bootstrap.go` and the new entry in
  `ownerMutationTools`.

## Manual end-to-end test plan (requires real GitHub App + repo)

The full self-healing recovery path requires real GitHub credentials and a
real repo to push to; nothing in this codebase can synthesise a working
installation token. Once an operator has those, the reproduction is:

1. Register / sign in at `http://localhost:8080` (`test@helix.ml` /
   `helixtest` per CLAUDE.md), complete onboarding, end up at an org.
2. Install the Helix GitHub App on a test repo (Settings → Integrations →
   GitHub).
3. From the helix-org chat, hire a Worker with a Role that includes
   `mint_credential`, `publish`, `subscribe`, `read_events`, plus the base
   read tools. The Role prompt should include the mint-export-retry
   paragraph from `owner_role.md` §"Long-running credentials".
4. From the Worker's container terminal, confirm `gh api user` succeeds
   under the boot-time `GH_TOKEN`.
5. Force expiry. Two equally good options:
   - Revoke the installation token in GitHub (Settings → Apps → installed
     Helix App → "Suspend"); or
   - In the Worker's container, run `export GH_TOKEN=ghs_deliberately_bad`
     to simulate expiry without affecting other Workers.
6. Re-run `gh api user` — expect 401.
7. Have the agent call `mint_credential {provider: "github"}`, parse
   `.token`, `export GH_TOKEN=$(...)`, and retry. Expect success.

Document the actual run in this directory as `E2E-RUN.md` once executed,
including the exact command output for steps 4–7. That artefact is what
closes acceptance criterion **AC9** in `requirements.md`.

## Why we did not run the E2E here

The inner-Helix sandbox is up and the API restarted cleanly, but exercising
the GitHub minting path requires connecting a real Helix GitHub App with a
real installation, plus a repo whose pushes can be observed for 401 →
re-mint → success. That setup is operator state, not something the agent
can synthesise in-session. The unit-test surface (8 tests on the tool, 5
on the GitHub provider, 4 updated resolver tests) plus the clean inner-
Helix Air-build is the strongest signal available without those
credentials.
