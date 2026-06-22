# Provisioning a GitHub "service account / robot" for Helix agents

**Date:** 2026-06-06 (last updated 2026-06-08)
**Status:** Design agreed — no implementation yet.
**Decision (2026-06-08):** **Option 1 — every Helix deployment owns its own GitHub
App.** The default setup is the **App Manifest flow** (one-click self-registration,
pre-configured by a Helix-supplied manifest); "bring your own app credentials" is
the override. Helix Cloud is just one deployment of this model (it owns the
`helixml` app). **There is no shared app whose private key ships to customers** —
see §4.5 for why that is rejected on security grounds.
**Author:** design session (Phil + Claude)

---

## 1. The problem

Helix Workers (org-graph runtime) and Sandboxes run `git` and `gh` against the
user's GitHub repos — clone, branch, commit, push, open PRs, manage issues. They
need GitHub credentials.

Today they borrow a **human's** credentials. `newGitHubOAuthResolver`
(`api/pkg/server/helix_org_github.go:40-100`) walks the org's members, finds the
first member with a valid GitHub OAuth connection, and returns *that person's*
access token. The github transport's secret injector
(`api/pkg/org/infrastructure/transports/github/secret_injector.go:30-46`) then
upserts it as the `GH_TOKEN` project secret on every worker activation.

Consequences of borrowing a human identity:

- **Wrong attribution.** Commits/PRs/comments show up as a real employee. Audit
  trails lie about who did what.
- **Fragile.** If that person revokes the OAuth grant, leaves the org, or rotates
  creds, every Worker silently loses GitHub access ("first valid token" roulette).
- **Over-broad.** A user OAuth token carries *all* of that user's scopes across
  *all* their repos/orgs — far more than one project needs.
- **Rate limits charged to a person**, shared across everything they do.

What the user wants: an easy, in-Helix way to give agents their **own**
GitHub identity — a "service account / robot account" — ideally created on the
user's behalf using the OAuth we already have.

---

## 2. The hard constraint (answering the literal question first)

> "Is it possible to create a bot [user] on behalf of the user via OAuth?"

**No — not a GitHub *user* account.** Two independent blockers:

1. **No API.** GitHub exposes no endpoint to create a login. Sign-up is a
   human-only web flow (email verification, CAPTCHA).
2. **ToS forbids it.** "You must be a human to create an Account; accounts
   registered by 'bots' or other automated methods are not permitted." Machine
   accounts are allowed, but only when **a human** registers them and takes
   responsibility, and you may keep **at most one free machine account**.

OAuth doesn't change this. An OAuth (user-to-server) token authenticates an
*existing human* and acts **as** that human. There is no scope that mints a new
identity.

**However** — there is a way to create a *bot* on the user's behalf with an
OAuth-shaped one-click consent. The bot is a **GitHub App**, not a user account,
and it's created via the **GitHub App Manifest flow** (§4.2). This is both the
honest answer to the question and the technically superior one.

---

## 3. The four real identity primitives, ranked

| Primitive | What it is | Own identity? | Create on user's behalf? | Token lifetime | Rotatable | Rate limit | GHES / air-gap | Best for |
|---|---|---|---|---|---|---|---|---|
| **GitHub App** ✅ | First-class bot (`name[bot]`) | Yes (`app[bot]`) | **Yes — Manifest flow** | Installation token, **1h, auto-mint** | Yes (key rotation) | Per-installation, scales (5k–15k/h) | Yes | **Agents doing repo automation** |
| Machine user + fine-grained PAT | A human-made bot login | Yes (named acct) | Partly — acct manual, rest automatable | PAT, months/long-lived | Manual only | Per-user, shared | Yes | Named bot, App-incompatible ops |
| User's own fine-grained PAT / OAuth | The human | No | n/a (it *is* them) | OAuth refreshable / PAT long | — | The human's | Yes | Quick start, single-user |
| Deploy key (SSH) | Per-repo git key | No (repo-scoped) | Yes (API, via user token) | Long-lived key | Manual | n/a (git only) | Yes | git-only, single repo, no API |

**Recommendation: GitHub App is the primary answer.** It is literally GitHub's
"service account" replacement: independent identity, fine-grained per-repo
permissions, short-lived auto-refreshed tokens, clean revocation (uninstall),
higher rate limits, and it works on GitHub Enterprise Server. Everything else is
a fallback for the cases an App can't cover.

---

## 4. Recommended design: GitHub App as the bot

### 4.0 What already exists in Helix (verified 2026-06-07)

A code audit found that the GitHub App foundations are **already present** — there
are three separate App-related pieces, but **no shared operator-level app**. The
remaining work is wiring + flows, not foundations.

- **Token minter already built & working.** `NewClientWithGitHubApp(appID,
  installationID, privateKey, baseURL)` at
  `api/pkg/agent/skill/github/client.go:70-100` uses
  `bradleyfalzon/ghinstallation/v2` (in `go.mod`) to sign the RS256 JWT and
  auto-mint/refresh 1-hour installation tokens. **Phase 1's minter is effectively
  done** — it just isn't called from the worker path.
- **Per-repo App auth is live in production.**
  `api/pkg/services/git_repository_service_pull_requests.go:618-628` reads
  `GitRepository.GitHub.{AppID,InstallationID,PrivateKey}` and prefers App auth
  over OAuth/PAT when opening PRs.
- **Per-org `ServiceConnection` is fully wired** — store CRUD
  (`store_service_connection.go`), REST routes (`server.go:1586-1591`), handlers
  with private-key encryption + a `/test` endpoint
  (`service_connection_handlers.go`), and frontend UI
  (`frontend/src/components/dashboard/ServiceConnectionsTable.tsx`). **But no
  production code mints worker tokens from it** — it's a credential store with a
  test button and no runtime reader.

**The real gaps (given the §4.1 single-app model):**
- **No runtime reader.** Nothing mints worker tokens from a `ServiceConnection` —
  the worker resolver still uses the human-OAuth path. (Phase 1.)
- **No self-serve way to obtain/install an app.** No manifest flow and no
  install-gate UI, so a `ServiceConnection` can only be populated by hand today.
  (Phases 2–3.)
- Note: `config.GitHub` (`config.go:607-613`) is OAuth-only — and that's fine, we
  are **not** adding operator app config (the app lives in `ServiceConnection`, not
  config; see §4.5 for why a config-resident shared key is rejected).

### 4.1 The single model: each deployment owns its own app

There is **one** model, applied uniformly to Helix Cloud and every self-hosted
install:

- **Each Helix deployment holds its own GitHub App credentials** (`app_id` +
  `private_key` + optional webhook secret), encrypted at rest in that deployment's
  own store. Nobody shares a private key with anyone else.
- **The default way to obtain those credentials is the App Manifest flow** (§4.2):
  one click, pre-configured by a Helix-supplied manifest (name, logo, the exact
  permission set), so it *feels* like "the default Helix app" while actually
  minting a fresh app owned by the customer's own GitHub org/account.
- **Override:** a customer who already has a GitHub App can paste its `app_id` +
  PEM instead of running the manifest flow. Same storage, same downstream.
- **Helix Cloud is not special.** It is one deployment that happens to own the
  `helixml`-org app. Architecturally it runs the identical path. This collapses
  the codebase to a single credential path instead of a shared-operator-app path
  plus a per-org path.

Storage: app credentials live in the existing **`ServiceConnection`** model
(`api/pkg/types/oauth.go:246-282`, already has `GitHubAppID` /
`GitHubInstallationID` / encrypted `GitHubPrivateKey`), scoped per Helix org. The
manifest flow writes one; the BYO-app override writes one; the worker resolver
reads it. No separate operator-config app, no `OrgGitHubInstallation` side table.

> **Why not a single shared app with the key in operator config (the earlier
> "Variant A")?** Because for self-hosted that key would have to ship to customer
> infra, and a distributed minting key is a cross-tenant compromise. Rejected —
> see §4.5.

### 4.2 The Manifest flow — the default, zero-config setup path

A handshake that mirrors OAuth; the whole thing must complete within **1 hour**:

1. **User clicks** "Create a GitHub bot for this org" in Helix (scoped to a Helix
   org → a chosen GitHub org).
2. **Helix builds a manifest** (JSON): name (`helix-<org>`), `url`,
   `hook_attributes.url` (Helix webhook), `redirect_url` (Helix callback),
   `public: false`, and crucially `default_permissions` — the minimum set, e.g.
   `contents: write`, `pull_requests: write`, `issues: write`, `metadata: read`
   (+ `workflows: write` only if agents touch Actions). Helix stores a `state`
   nonce — **reuse the existing `OAuthRequestToken` table**
   (`api/pkg/types/oauth.go:88-100`) for CSRF, same as the OAuth flow.
3. **Auto-submitting form** POSTs the manifest to
   `https://github.com/organizations/<org>/settings/apps/new?state=<nonce>`
   (org-owned — preferred so the bot belongs to the org, not a person). The user
   must be a GitHub org owner; otherwise fall back to the user-owned
   `https://github.com/settings/apps/new`.
4. **User reviews** the permission list on GitHub and clicks "Create GitHub App."
5. **GitHub redirects** to Helix `redirect_url?code=<temp>&state=<nonce>`.
6. **Helix exchanges** the code: `POST https://api.github.com/app-manifests/<code>/conversions`
   → returns `{ id, slug, pem (private key), webhook_secret, client_id,
   client_secret, ... }`. (Code valid 1h.)
7. **Helix persists** it — encrypt `pem` + secrets (AES-256-GCM, already used at
   `api/pkg/crypto/encryption.go`) into a `ServiceConnection`
   (`api/pkg/types/oauth.go:246-282` — the model *already has* `GitHubAppID`,
   `GitHubInstallationID`, `GitHubPrivateKey`). Scope it to the Helix org.
8. **Install it immediately:** redirect to
   `https://github.com/apps/<slug>/installations/new` so the user picks which
   repos. The post-install callback gives `installation_id`; store it on the
   `ServiceConnection`.

After this the org has `app_id` + `private_key` + `installation_id` = a fully
owned bot, created with two consent clicks and no manual account creation.

On **GitHub Enterprise Server** the same flow works against the instance host
(`https://<ghes-host>/organizations/<org>/settings/apps/new` and
`/api/v3/app-manifests/...`) — important for the air-gapped/enterprise customers
in CLAUDE.md.

### 4.3 Minting & injecting tokens

This is the part to build once; it is identical regardless of how the app
credentials were obtained (manifest or BYO).

1. **App JWT:** sign a short-lived (≤10 min) RS256 JWT with the app private key
   (`iss = app_id`).
2. **Installation token:** `POST /app/installations/<installation_id>/access_tokens`.
   Optionally pass `repository_ids` + a `permissions` subset to **scope the token
   down to exactly the repos/permissions a given Worker's project needs** (least
   privilege per worker). Returns a token valid **1 hour**.
3. **Cache** the token keyed by `(installation_id, repo-set, perm-set)` with its
   `expires_at`; re-mint on demand when within ~5 min of expiry. (The OAuth
   manager already runs a 1-minute refresh loop, `api/pkg/oauth/manager.go:77-78`
   — model the installation-token refresher on it, or mint lazily at injection.)
4. **Inject into the worker** via the *existing* secret-injector seam
   (`secret_injector.go`). Swap its `TokenResolver` from "first human OAuth token"
   to "installation token for this org's `ServiceConnection`, scoped to this
   project's repos." Two outputs:
   - `GH_TOKEN` for the `gh` CLI.
   - a git credential helper / `https://x-access-token:<token>@github.com/...`
     URL rewrite for raw `git`.
5. **Attribution:** commits land as `helix-<org>[bot]` with noreply email
   `<app-id>+<slug>[bot]@users.noreply.github.com`. Set `git config user.name/email`
   in the worker startup script to match so authorship is clean.

Because tokens are 1h and re-minted, a leaked worker token self-expires — far
safer than today's long-lived borrowed human token.

### 4.4 How it maps onto existing Helix code

| Need | Existing anchor | Change |
|---|---|---|
| Store app creds (app_id + PEM + installation_id) | `ServiceConnection` w/ `GitHubAppID`/`InstallationID`/`PrivateKey` (`types/oauth.go:246-282`) | Already exists — the **single** home for app creds; manifest flow and BYO both write here. No separate operator-config app, no side table |
| Encrypt key | `crypto.EncryptAES256GCM` (`crypto/encryption.go`) | Reuse; ensure `HELIX_ENCRYPTION_KEY` is set (insecure default warning at lines 38-40) |
| CSRF / state | `OAuthRequestToken` (`types/oauth.go:88-100`) | Reuse for manifest + install state |
| Mint installation token | **Already exists:** `NewClientWithGitHubApp` (`agent/skill/github/client.go:70-100`, `ghinstallation/v2`) signs the JWT + auto-refreshes 1h tokens | Reuse as-is; just call it from the worker resolver |
| Resolve token for worker | `newGitHubOAuthResolver` (`helix_org_github.go:40-100`) | Replace/extend: prefer App installation token (from `ServiceConnection`); fall back to OAuth/PAT |
| Inject into worker | `secret_injector.go` + spawn hooks (`spawn_hooks.go:33-117`) | Point resolver at installation tokens; add git credential output |
| PAT fallback store | `GitProviderConnection` w/ encrypted `Token` (`types/oauth.go:169-205`) | Already exists — reuse for §6 |

The pleasant surprise: the data model for the App path largely **already exists**.
The work is the flows (manifest/install), the token minter, and re-pointing the
resolver.

### 4.5 Rejected alternatives (do not re-propose without reading this)

**(A) One shared "Helix" app whose private key ships to / is configured on every
deployment.** *Rejected — cross-tenant compromise.* The app private key (PEM) is a
**minting key**: anyone holding it can mint installation tokens for **every org
that installed that app**, including other customers'. The moment that key sits on
self-hosted customer infra, any customer can extract it from their own box and
mint `helix[bot]` tokens against *other* customers' repos. A secret distributed to
mutually-untrusting parties is not a secret. This is fatal specifically because we
ship to self-hosted infra; it would be merely "centralized risk" if Helix were
Cloud-only. Either way, the manifest model is strictly better, so the shared app
buys nothing.

**(B) Central token broker — Helix Cloud holds the one PEM and mints tokens for
self-hosted instances on request.** *Rejected — breaks air-gap, creates a token
oracle.* It keeps a single `helix[bot]` identity and never ships the PEM, but: (1)
every self-hosted instance now depends on Helix Cloud at runtime for GitHub access
(impossible for air-gapped/GHES, which CLAUDE.md calls out as first-class); (2)
Helix Cloud becomes a minting oracle that must authorize *which* instance may mint
for *which* installation — a serious authz surface and liability concentration.
The single-app-per-deployment model avoids all of this. Keep this option only in
mind if a future requirement *demands* one global bot identity across all
self-hosted installs (it doesn't today).

**(C) Borrow a human's OAuth token (the status quo).** *Rejected — see §1.* Wrong
attribution, fragile, over-broad, shared rate limits. This is the thing we are
replacing.

### 4.6 Surfacing it in the product: install once, then act as the bot

**Core principle (decided 2026-06-08):** the user's own GitHub credentials are used
in **exactly one place — the one-time install of the Helix App.** After that,
*everything* — listing repos in the New Stream picker, the worker's `git`/`gh`,
outbound actions — runs on the **bot's installation token**. The user's account is
never used for data access again. This is the inversion of an earlier draft that
kept the repo picker on the user's OAuth token.

Consequence (intended, least-privilege): the picker shows **only the repos the app
is installed on**. To add a repo, the user re-opens the app's "Configure" screen on
GitHub and grants it — they don't pick from their full personal repo list. A repo
the bot can't see is a repo the bot can't operate on, so showing it would be a lie.

The trigger UX lives in the **New Stream** dialog
(`frontend/src/pages/HelixOrgStreams.tsx:298-669`). Today it gates on a GitHub
**OAuth** connection (`ghConnected` from `useListGitHubRepos`, `:320-322`; the
"Connect GitHub for streams" box, `:548-570`). That gate is **replaced** by a
single **"Install Helix"** gate:

1. **Is the app installed for this org?** Cheapest check: does the org have a
   `github_app` `ServiceConnection` with a non-zero `installation_id`? (The
   `installation_id` was captured at install time — see step 3.) No network call
   needed for the common case.
2. **If not installed → the bootstrap gate.** Render the styled box with an
   **"Install Helix"** button that opens
   `https://github.com/apps/<slug>/installations/new` in a popup. **This is the only
   step that uses the user's GitHub identity** — their github.com session drives the
   consent; they choose the account/org + repos. (Note: this does *not* require
   Helix's separate GitHub OAuth app at all — so the old "Connect GitHub OAuth"
   prerequisite can be dropped, not stacked on top.)
3. **Capturing the result.** Set the app's **Setup URL** to a Helix callback; GitHub
   redirects there post-install with `?installation_id=...`. Helix writes it onto
   the org's `ServiceConnection` and `postMessage`s the dialog to refetch. (v1 ship-
   early fallback: an "I've installed it — re-check" button + refetch on window
   focus, if the Setup-URL callback isn't wired yet.)
4. **Once installed → the repo picker runs on the bot.** `listGitHubRepos` mints an
   installation token (the §4.3 minter) and calls `GET /installation/repositories`
   (go-github `Apps.ListRepos`) — the repos the bot can act on. No user token.
5. **Create the stream / hire the worker.** All downstream GitHub access (worker
   `git`/`gh`, outbound transport) uses the same installation token.

So there is no "second gate" and no per-repo probe — the model is binary per org
("is Helix installed?") and bot-centric everywhere after. The app JWT / 
`GET /repos/{owner}/{repo}/installation` probe from the earlier draft is no longer
needed; presence of a stored `installation_id` is the source of truth.

---

## 5. What a GitHub App *can't* do (know the edges)

Apps cover repo automation completely (contents, PRs, issues, checks, Actions,
releases, deployments). They **cannot**: act as a human reviewer in every UI
context, star/follow/sponsor, or do a handful of user-only operations. If a use
case needs those, that's when the machine-user fallback (§6) earns its place.

---

## 6. Fallback: guided machine-user + fine-grained PAT

For when an App is impossible or unwanted — a customer who insists on a
named human-looking account, an op an App can't do, or a locked-down GHES where
app registration is disabled.

Since account creation can't be automated, make the **guided** path smooth and
automate everything *around* it using the requesting user's OAuth token (which,
with `admin:org`, can manage org membership/teams/repo access):

1. **Wizard step 1 (manual):** "In an incognito window, create a GitHub account
   `acme-helix-bot` with email `bot+helix@acme.com`." (Respect the one-free-
   machine-account ToS limit.)
2. **Wizard step 2 (manual):** "Create a **fine-grained PAT**, scoped to *these*
   repos with *these* permissions" — Helix shows a copy-paste list and a deep
   link. Fine-grained PATs support expiry + per-repo scoping + org approval.
3. **Wizard step 3 (paste):** user pastes the PAT; Helix encrypts it into the
   existing `GitProviderConnection` (`types/oauth.go:169-205`).
4. **Helix automates the rest** via the requesting user's OAuth token: invite the
   machine user to the org, add to a `helix-bots` team, grant repo access. So the
   *only* manual steps are account creation + PAT paste.

Downsides vs the App: long-lived secret, no auto-rotation (PATs can't be minted
by API — only created in the UI), manual steps, the ToS one-account cap. Use only
when the App path genuinely doesn't fit.

---

## 7. Security considerations

- **Short-lived >> long-lived.** Installation tokens (1h, auto-mint) are the
  whole security win over today's borrowed token and over PATs.
- **The private key is the crown jewel.** Already AES-256-GCM encrypted; ensure
  `HELIX_ENCRYPTION_KEY` is set in every deployment (there's an insecure fallback
  today). Support GitHub's multiple-active-keys feature for zero-downtime
  rotation.
- **Least privilege per worker.** Mint installation tokens scoped to a project's
  `repository_ids` + minimal `permissions`, not the whole installation.
- **Webhook secret verification** on the manifest-provided secret.
- **Auditability.** App actions are attributable to the bot and appear in the org
  audit log — the opposite of today's "which human was it" ambiguity.
- **Blast radius is contained by design.** Each deployment owns a distinct app
  key, so a key compromise is limited to that one deployment's installs — never
  cross-tenant. This is the whole reason the shared-key model was rejected (§4.5).

---

## 8. Phased implementation plan

**Decision (2026-06-08): Option 1 — single self-owned-app model.** Everything sits
behind the *already-built* token minter (`NewClientWithGitHubApp`) reading the
*already-wired* `ServiceConnection` store. Order below is dependency-driven, not
Cloud-vs-self-hosted.

- **Phase 1 — unify GitHub identity on the bot (assumes an app is installed; full
  plan in §10).** Centralize identity resolution: if the org has a GitHub-App
  `ServiceConnection` with an `installation_id`, *every* GitHub touchpoint (repo
  picker, worker `git`/`gh`, outbound transport, webhook install) uses an
  installation token; else fall back to today's OAuth path. The repo picker switches
  to `GET /installation/repositories` in app mode. Seed one `ServiceConnection`
  manually via the existing UI to test. Fixes "agents act as a human" *and* makes the
  picker bot-scoped, wherever an app is configured.
- **Phase 2 — the New Stream install bootstrap (§4.6).** Replace the OAuth-connect
  gate with a single **"Install Helix"** gate: open
  `https://github.com/apps/<slug>/installations/new` (the one place the user's
  GitHub identity is used), capture `installation_id` via the app's Setup-URL
  callback (v1 fallback: "I've installed it — re-check" button), persist it onto the
  org's `ServiceConnection`. This is what makes Phase 1 self-serve.
- **Phase 3 — the manifest flow (§4.2), the default zero-config setup.** "Set up
  GitHub for Helix" → manifest → conversion → write `ServiceConnection`. Plus the
  BYO-app override (paste app_id + PEM). This is what lets a fresh self-hosted
  deployment get an app at all without manual GitHub registration. (Helix Cloud's
  `helixml` app can be seeded once manually, so Cloud doesn't block on this.)
- **Phase 4 — hardening.** Per-worker scoped tokens (`repository_ids` + permission
  subset), Setup-URL callback for auto-capturing `installation_id` (§4.6 v2), key
  rotation, GHES base URLs, startup-script `git config` for clean authorship.
- **Phase 5 — PAT/machine-user fallback wizard** for the App-incompatible cases (§6).

Each phase is independently shippable. Phase 1 delivers most of the value for any
deployment that has an app; Phases 2–3 make obtaining/installing the app self-serve.

---

## 9. Recommendation in one paragraph

Don't try to create a GitHub *user* account — it's against ToS and has no API.
Instead make the bot a **GitHub App**, which is GitHub's actual service-account
primitive. **Every Helix deployment owns its own app** (Helix Cloud included); the
default way to get one is the **App Manifest flow** (one-click, Helix-pre-configured
self-registration), with "paste your own app credentials" as the override. **No
private key is ever shared across deployments** — that would be a cross-tenant
compromise (§4.5). Each deployment mints **short-lived per-installation tokens**
with its own key and injects them through the secret-injector seam Helix already
has, scoped per project — replacing the borrowed-human-token resolver that exists
today. The `ServiceConnection` store, encryption, token minter, and injection
plumbing already exist; the net-new work is the manifest + install-gate flows
(surfaced in the New Stream dialog, §4.6) and re-pointing the resolver. Keep a
guided machine-user + fine-grained-PAT wizard as the fallback for the few things an
App can't do.

---

## 10. Phase 1 — code-level implementation plan (corrected 2026-06-08 for the bot-everywhere model)

**Goal:** once an org has an installed Helix App, the org's GitHub identity **is**
the bot for *all* data access and actions — repo listing in the picker, worker
`git`/`gh`, outbound transport, webhook install. The user's OAuth token is retained
**only** as a legacy fallback for orgs that have not installed an app yet. (The
user's GitHub identity is used solely by the §4.6 install bootstrap, which is Phase
2 UI; for Phase 1 the `installation_id` is seeded manually via the existing
`ServiceConnection` UI so the bot path is testable in isolation.)

This is **not** the earlier "one wiring line." Because the repo picker must now list
the *installation's* repos, and an installation token cannot call `/user/repos`,
the identity resolution is centralized and several call sites change. Honest scope:
a small refactor, not a one-liner.

### 10.0 Centralize identity resolution

Today `gitHubTokenResolver` (`helix_org.go:240`) is `func(ctx, orgID) (string,
error)` and five sites consume the bare token. Replace it with an **identity
resolver** that returns enough to both mint and branch by mode:

```go
type OrgGitHubIdentity struct {
    Mode           string // "app" | "oauth"
    Token          string // installation token (app) or user OAuth token (oauth)
    AppID          int64  // app mode
    InstallationID int64  // app mode
    BaseURL        string // GHES; "" = github.com
}
// app installed for org → mint installation token (Mode=app);
// else → existing OAuth walk (Mode=oauth); both empty → Mode="oauth", Token="".
func newOrgGitHubIdentityResolver(getKey func() ([]byte, error), st store.Store,
    oauthFallback func(context.Context, string) (string, error),
    mintFn func(ctx context.Context, appID, installationID int64, pem, baseURL string) (string, error),
) func(context.Context, string) (OrgGitHubIdentity, error)
```

Selection logic: `ListServiceConnectionsByType(ctx, orgID,
ServiceConnectionTypeGitHubApp)` → newest with `AppID!=0 && InstallationID!=0 &&
PrivateKey!=""` → `crypto.DecryptAES256GCM(conn.GitHubPrivateKey, key)` →
`mintFn(...)`. **On any app-side miss/error: log + fall through to the OAuth walk** —
a broken app config must never break an org OAuth could still serve.

### 10.1 New code

**(a) Raw installation-token minter** — `agent/skill/github/client.go`. Sibling of
`NewClientWithGitHubApp` (which builds a client but never exposes the token):

```go
func MintInstallationToken(ctx context.Context, appID, installationID int64, privateKey, baseURL string) (string, error) {
    itr, err := ghinstallation.New(http.DefaultTransport, appID, installationID, []byte(privateKey))
    if err != nil { return "", fmt.Errorf("github app transport: %w", err) }
    if baseURL != "" { itr.BaseURL = strings.TrimSuffix(baseURL, "/") + "/api/v3" }
    return itr.Token(ctx) // live POST /app/installations/{id}/access_tokens
}
```

**(b) Installation-repo listing** — `agent/skill/github/client.go`. The picker in
app mode lists the *installation's* repos, not the user's. Add a method on the
existing `*Client` (built via `NewClientWithGitHubApp`) wrapping go-github
`Apps.ListRepos` (`GET /installation/repositories`):

```go
func (c *Client) ListInstallationRepositories(ctx context.Context) ([]*github.Repository, error)
```

**(c) Identity resolver** (§10.0) — `server/helix_org_github.go`, alongside the
existing `newGitHubOAuthResolver` (which it reuses as the `oauthFallback`).

### 10.2 Consumer changes (the five sites)

| Site | Today | Change |
|---|---|---|
| Worker `GH_TOKEN` injector (`helix_org.go:250` → `secret_injector.go`) | OAuth token | use `identity.Token` (bot in app mode) |
| Repo picker `listGitHubRepos` (`api.go:1965`) | OAuth → `ListRepositories` (`/user/repos`) | **branch on mode:** app → `ListInstallationRepositories`; oauth → unchanged |
| Outbound transport `WithTokenResolver` (`api.go:1917`) | OAuth token | use `identity.Token` |
| Webhook install (`api.go:2114`) | OAuth (`admin:repo_hook`) | use `identity.Token` (bot needs Webhooks permission on the app; else keep OAuth — verify perm) |
| Public webhook outbound (`helix_org.go:342`) | OAuth token | use `identity.Token` |

The `Deps.GitHubTokenResolver` field type changes from returning a string to
returning `OrgGitHubIdentity` (or add a parallel `Deps.GitHubIdentity` and migrate
sites incrementally). `cfg.APIServer.getEncryptionKey` (`utils.go:103`) is in scope
at `helix_org.go:240` to pass as `getKey`.

> **Optional staging:** 1a = worker injector + repo picker (the visible "act as the
> bot"); 1b = outbound transport + webhook install + public webhook. Both land
> behind the same resolver; 1a is the demoable slice.

### 10.3 Known limitations (each maps to a later phase, document in code)

- **1h token expiry.** `GH_TOKEN` is injected statically per activation; minted
  tokens die after 1h, so workers running >1h lose git/gh auth mid-run — a lifetime
  regression vs the long-lived OAuth token. **Fix = Phase 4** re-minting git
  credential helper. Acceptable + flagged for Phase 1.
- **Installation-wide scope.** Resolver has only `orgID`, so it mints an
  installation-wide token, not the project's repo subset. **Least-privilege = Phase 4.**
- **Author identity.** Push *attribution* follows the token (→ `[bot]`), but commit
  `user.name/email` come from the worker's git config — set them to the bot noreply
  email in the worker startup script. **Phase 4.**
- **Picker shows installed repos only** (intended; §4.6). Not a regression to fix —
  a behavior change to communicate in the UI copy.

### 10.4 Test plan

- **Unit** (`helix_org_github_test.go`, gomock `MockStore`, `mintFn` stubbed):
  (1) no app conn → `Mode=oauth`, OAuth token; (2) app conn → `Mode=app`, decrypted
  PEM + stubbed token; (3) two app conns → newest chosen; (4) mint error → falls
  back to OAuth. (`Token()` is a live call — that's why `mintFn` is a seam.)
- **Build:** `go build ./pkg/server/ ./pkg/agent/skill/github/ ./pkg/org/... ./pkg/store/ ./pkg/types/`.
- **E2E (inner Helix):** seed a `github_app` ServiceConnection via the existing
  `ServiceConnectionsTable` UI (test `helixml` app creds) → open New Stream → confirm
  the repo picker lists *installation* repos → hire a worker → confirm `gh auth
  status` + a test push land as `<slug>[bot]`.

### 10.5 Files touched

| File | Change |
|---|---|
| `api/pkg/agent/skill/github/client.go` | + `MintInstallationToken`, + `ListInstallationRepositories` |
| `api/pkg/server/helix_org_github.go` | + `newOrgGitHubIdentityResolver` (reuses `newGitHubOAuthResolver`) |
| `api/pkg/server/helix_org.go` | resolver construction + injector wiring (~240-250, ~342) |
| `api/pkg/org/interfaces/server/api/api.go` | `Deps` type change; `listGitHubRepos` mode branch; transport + webhook sites |
| `api/pkg/server/helix_org_github_test.go` | + unit tests (new file) |

Bigger than the earlier estimate (the `Deps` type change + picker-endpoint branch
ripple across `api.go`), but still **no schema change and no new config** — the
store query, model, encryption, and ghinstallation dep all already exist.

---

### Sources
- [Registering a GitHub App from a manifest — GitHub Docs](https://docs.github.com/en/apps/sharing-github-apps/registering-a-github-app-from-a-manifest)
- [GitHub Terms of Service (machine accounts / no bot-created accounts)](https://docs.github.com/en/site-policy/github-terms/github-terms-of-service)
- [Types of GitHub accounts — GitHub Docs](https://docs.github.com/en/get-started/learning-about-github/types-of-github-accounts)
- [Create a GitHub App from a manifest — Xebia](https://xebia.com/blog/create-a-github-app-from-a-manifest/)
