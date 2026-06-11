# Design: Add `mint_credential` MCP Tool for Worker Credential Refresh

## 1. Problem recap

GitHub credentials are minted as GitHub App installation tokens (~1h TTL) and
injected into a Worker desktop **once, at boot**, as the `GH_TOKEN` env var.
A Docker env var is immutable for the container's life, and there is **no
refresh mechanism**. Sessions that outlive ~1h end up with an expired
`GH_TOKEN`. Re-activation does not help — `SecretInjector` only fires at
container spawn, and the secret is only read into env at boot. The bug is the
lack of refresh, not the TTL.

Slack will hit the same shape of problem the moment its transport lands, so
the fix is built provider-agnostic.

## 2. Decisions (recorded; not re-litigating)

- **Rejected: long-lived bot PAT.** Breaks per-org tenant isolation (the
  resolver is per-org, see `newOrgGitHubIdentityResolver` in
  `api/pkg/server/helix_org.go:262`); enlarges blast radius of a leak;
  doesn't actually remove rotation cliffs.
- **Chosen: generic `mint_credential` MCP tool.** Recurring need is *"give me
  a fresh credential for provider X"*. It enables shell tools (`curl`/`gh`/
  `git`), it doesn't wrap an action, so it earns a place on the MCP surface.
- **Recorded MCP-surface exception.** `CLAUDE.md` says *"keep the MCP surface
  small"* and bans per-action wrappers (`publish_to_blog`, `fetch_url`). A
  generic credential-minting **primitive** is a different category and is
  allowed. This is the only exception we're making in this task; it is
  documented here and at the top of `mint_credential.go` so reviewers don't
  have to dig the rationale up.
- **Delivery via prompting + recovery, not a credential helper.** Per the
  helix-org philosophy (*"social enforcement first"*), the agent calls the
  tool itself. Cost of a violation here is low and recoverable (a single auth
  error mid-task), so no transparent `git`-credential-helper is needed for
  v1. The credential helper remains the documented upgrade path if
  observation shows agents fumbling the mint→export→retry sequence in
  practice.

## 3. Architecture

### 3.1 New domain interface

A small interface in a new package — placement is `org/domain/credential/`
(parallel to `org/domain/transport/`, `org/domain/tool/`, etc):

```go
// Package credential defines the small CredentialProvider interface
// every per-provider credential minter implements. The mint_credential
// MCP tool dispatches across a name-keyed registry of these.
package credential

import (
    "context"
    "time"
)

type Provider interface {
    Name() string                                       // "github", "slack", ...
    Mint(ctx context.Context, orgID string) (Credential, error)
}

type Credential struct {
    Token     string
    ExpiresAt time.Time
    Usage     string // human hint, e.g. "export GH_TOKEN=<token>"
}
```

The interface lives in `domain/` (not `application/` or `infrastructure/`)
because every layer can depend on `domain/` without forming a cycle — same
pattern as `domain/transport` and `domain/tool`. The concrete providers
live alongside their transports under `infrastructure/transports/<name>/`.

### 3.2 New MCP tool

`api/pkg/org/application/tools/mint_credential.go` — new file, structured
identically to `create_stream.go`:

```go
type MintCredential struct {
    deps      Deps
    providers map[string]credential.Provider
}

const MintCredentialName tool.Name = "mint_credential"

type mintCredentialArgs struct {
    Provider string `json:"provider"`
}

func (t *MintCredential) Name() tool.Name             { return MintCredentialName }
func (t *MintCredential) Description() string         { return mintCredentialDescription }
func (t *MintCredential) InputSchema() *jsonschema.Schema { return mintCredentialSchema }

func (t *MintCredential) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
    var args mintCredentialArgs
    if err := json.Unmarshal(inv.Args, &args); err != nil {
        return nil, fmt.Errorf("parse args: %w", err)
    }
    if args.Provider == "" {
        return nil, fmt.Errorf("mint_credential: provider is required")
    }
    p, ok := t.providers[args.Provider]
    if !ok {
        return nil, fmt.Errorf("mint_credential: unknown provider %q; available: %s",
            args.Provider, strings.Join(providerNames(t.providers), ", "))
    }
    orgID := inv.Caller.OrganizationID()
    if orgID == "" {
        return nil, fmt.Errorf("mint_credential: caller has no OrgID")
    }
    cred, err := p.Mint(ctx, orgID)
    if err != nil {
        return nil, fmt.Errorf("mint %s credential: %w", args.Provider, err)
    }
    return json.Marshal(map[string]any{
        "token":      cred.Token,
        "expires_at": cred.ExpiresAt.UTC().Format(time.RFC3339),
        "usage":      cred.Usage,
    })
}
```

**Note the symmetry with `create_stream`:** `orgID := inv.Caller.OrganizationID()`
is the *only* place org is read. The agent picks the **provider**, never the
**org**. A Worker cannot mint another org's credential because the orgID
never came from an arg.

### 3.3 Tool description (load-bearing — drives the recovery loop)

```text
Mints a short-lived credential (~1 hour) for an external provider scoped to
your organization. Currently supported: "github" (more added over time).

Run mint_credential and **export the returned token into your shell
immediately before** any git, gh, or authenticated curl work
(e.g. `export GH_TOKEN=$(...)`).

**If a command fails with a 401/403 / authentication error, your token has
expired** — call mint_credential again, re-export, and retry. Do not give up
on the task; expired tokens are expected for any work that takes more than
~1 hour.

Args:
  provider (string, required) — one of: github
Returns:
  token (string), expires_at (RFC3339), usage (string hint)
```

The final paragraph is the load-bearing bit: it converts silent mid-task
expiry into a self-healing retry, which is what keeps the violation cost low
enough to justify skipping hard enforcement.

We do **not** instruct *"mint at activation"* — `SecretInjector` already does
that at boot; the gap is the long-running session, and the prompt must speak
to that gap, not the boot moment.

### 3.4 GitHub provider implementation

New file `api/pkg/org/infrastructure/transports/github/credential_provider.go`:

```go
type credentialProvider struct {
    identityResolver func(ctx context.Context, orgID string) (githubIdentity, error)
}

func NewCredentialProvider(resolver IdentityResolver) credential.Provider { ... }

func (p *credentialProvider) Name() string { return "github" }

func (p *credentialProvider) Mint(ctx context.Context, orgID string) (credential.Credential, error) {
    id, err := p.identityResolver(ctx, orgID)
    if err != nil { return credential.Credential{}, err }
    if id.Token == "" {
        return credential.Credential{}, fmt.Errorf("no github identity configured for org %q", orgID)
    }
    return credential.Credential{
        Token:     id.Token,
        ExpiresAt: id.ExpiresAt, // ~1h from MintInstallationToken
        Usage:     "export GH_TOKEN=<token>",
    }, nil
}
```

`IdentityResolver` is a small adapter type defined in the github transport,
satisfied by the existing `newOrgGitHubIdentityResolver` constructor in
`helix_org.go:262`. We deliberately reuse the resolver rather than
re-implement the resolution flow — both surfaces (`SecretInjector`,
`mint_credential`) flow through one per-provider minting path.

**Existing resolver needs a small refactor.** Currently the resolver shape is
`func(ctx, orgID string) (string, error)` (token only). To return
`expires_at`, we widen the resolver's return shape to a small struct
exposing `Token`, `ExpiresAt`. The `SecretInjector` keeps using only
`Token`. Discovery + change locations:

- `api/pkg/server/helix_org_github.go` — `newOrgGitHubIdentityResolver`
  signature and return.
- `api/pkg/server/helix_org.go:262` — wiring site.
- `api/pkg/agent/skill/github/client.go:127` — `MintInstallationToken`
  already returns a string today; we widen the wrapper here to also surface
  the GitHub-API `expires_at`.

### 3.5 Wiring

In `helix_org.go` near the existing `secretInjectors` block (`helix_org.go:283`),
build the provider registry from the same resolver:

```go
credentialProviders := map[string]credential.Provider{
    "github": githubtransport.NewCredentialProvider(identityResolver),
}
```

Pass it into `tools.RegisterBuiltins` via the `Deps` struct (new field
`CredentialProviders map[string]credential.Provider`). The
`MintCredential` builtin reads `deps.CredentialProviders` at registration:

```go
&MintCredential{deps: deps, providers: deps.CredentialProviders},
```

Adding a new provider (e.g. Slack) is then:
1. New file `infrastructure/transports/slack/credential_provider.go` that
   satisfies `credential.Provider`.
2. One added line in the `credentialProviders` map in `helix_org.go`.
3. Zero edits to `mint_credential.go`.

That satisfies the philosophy guardrail *"new tools must be addable without
editing the core"* (CLAUDE.md, "helix-org design philosophy").

### 3.6 No more `SecretInjector` (period)

**Update (after user feedback during implementation):** the GitHub
`SecretInjector` (which pushed `GH_TOKEN` into the project env at
container boot) is **removed**. With every Worker holding
`mint_credential` in its baseline tool set, the boot-time env var is
redundant — and slightly harmful: it teaches the agent that
`gh`/`git` work without thinking about credentials, then silently
fails ~1h in when the env var expires. Removing it forces the agent
into the mint→export→retry pattern from turn 1, which is the same
pattern it needs anyway on every subsequent refresh.

After the github registration was removed, the **entire generic
`SpawnSecretInjector` mechanism had zero callers** — ~250 lines of
infrastructure (interface, function adapter, `SpawnerConfig` field,
spawner loop, 3 layers of `helix_org.go` parameter plumbing, 3 test
files). Per CLAUDE.md (`NO FALLBACKS`, `CLEAN UP DEAD CODE
immediately`, `don't design for hypothetical future requirements`),
the whole mechanism is deleted. If a future transport ever needs
push-at-boot secrets, `ProjectService.PutProjectSecret` is already a
public method — re-introducing a registry of injectors is one file.

`gitHubTokenResolver` (the bot-preferring string projection of the
identity resolver) stays — it still backs the outbound github
stream transport's `Token()` lookup and the webhook-install flow.
Only the Worker-shell-token surface moves to `mint_credential`.

## 4. Role / prompt changes

**Update (after user feedback during implementation):** `mint_credential`
is part of `BaseReadTools` (the universal baseline that every Role gets
via `MergeBaseReadTools` and the `RoleReconciler`). The argument:

- It does not mutate the org graph (it returns an external-provider
  credential).
- The cost of a Worker not having it is high — silent mid-task auth
  failures on any session that outlives its boot-time `GH_TOKEN`'s ~1h
  TTL.
- The same argument applies to *every* Worker, not just some Roles.

That makes the per-Role tools-list edit unnecessary. The remaining
hiring-time obligation is the prompt paragraph in any Worker Role whose
Worker runs `gh`/`git`/auth `curl` — without that paragraph the Worker
has the tool but no signal for when to reach for it. Pure prompt
edit, no Go logic.

A note in `BaseReadTools`' doc comment records that `mint_credential` is
the sole non-read entry and why it is justified.

## 5. Security & privacy

- The minted token lands in the agent's transcript/context. That is the
  unavoidable cost of agent-visible refresh. We mitigate by **keeping TTLs
  as short as the provider allows** — `MintInstallationToken` already gives
  us ~1h; we do not extend it.
- No new persistent secret storage. Tokens are minted on demand and never
  cached server-side beyond the response.
- No new auth surface — `mint_credential` is an MCP tool authorized exactly
  the way every other built-in is (presence in `Role.Tools`).
- The schema rejects an `org_id` field entirely; the org is sourced from
  the caller's identity. (See AC2 in requirements.md.)

## 6. Testing strategy

### 6.1 Unit tests (`mint_credential_test.go`)

- Happy path: a fake `credential.Provider` registered as `"fake"` returns a
  pre-canned `Credential`; the tool returns it as the expected JSON shape.
- Unknown provider: returns an error containing `"unknown provider"` and the
  list of registered providers.
- Missing OrgID on caller: returns the "caller has no OrgID" error (mirrors
  `create_stream`).
- `org_id` field in raw args is ignored: org is always
  `inv.Caller.OrganizationID()` — assert that a forged `org_id` in args
  cannot change the resolved org.
- Provider error propagates with the provider name in the wrap.

### 6.2 Provider-side test (`credential_provider_test.go` in github transport)

- `Mint` returns the resolver's token + a non-zero `ExpiresAt`.
- Empty token from resolver → error mentioning the orgID.

### 6.3 End-to-end (inner Helix)

Following CLAUDE.md *"prefer end-to-end testing in the inner Helix"*:

1. Register / sign in at `http://localhost:8080`, create an org, install the
   Helix GitHub App.
2. Hire a Worker on a Role that includes `mint_credential` and `gh`/`git`
   shell tools.
3. From the Worker's container, run a `gh api` call — confirm success.
4. Force expiry: either revoke the installation token via GitHub, or
   overwrite `GH_TOKEN` in the running container with a deliberately bad
   value.
5. Run another `gh api` call — confirm 401.
6. Observe (or invoke) `mint_credential`, `export GH_TOKEN=$(...)`, retry —
   confirm success.

Document the script in the task's notes for repeatability.

## 7. Anti-scope (explicit)

- **No automatic re-injection of `GH_TOKEN` into a running container.** That
  was the original instinct; it requires either a privileged sidecar or a
  Docker exec from outside the container — both too much surface for the
  recovery cost.
- **No caching of minted tokens.** Mint-on-demand is simpler; the upstream
  caching in `MintInstallationToken` is sufficient.
- **No new types of authorization (scopes, rate limits).** Role membership
  is the entire model — we are explicitly following the existing tool
  pattern, not extending it.

## 8. Learnings worth preserving

- **`inv.Caller.OrganizationID()` is the canonical pattern** for any MCP
  tool that needs org scoping. Confirmed by `create_stream.go:103` (and
  similar in `subscribe.go`, `hire_worker.go`). Future tools should not add
  an `org_id` arg.
- **The existing `SecretInjector` and the new `mint_credential` are two
  surfaces over one minting path.** When adding a new transport, design the
  per-provider minting path first, then expose it through whichever surfaces
  the transport needs — push (`SecretInjector`), pull
  (`CredentialProvider`), or both.
- **MCP-surface exceptions need to be recorded inline.** `CLAUDE.md` is
  unambiguous about not growing the surface. This task is an exception (a
  primitive, not an action wrapper), so the rationale lives in the tool
  file's doc comment and in this design doc — not just in chat history.
