# Design: Customise Login SSO Copy for SaaS vs Enterprise

## Overview

Conditionally change login page copy based on whether the deployment is Helix Cloud (SaaS) or an enterprise/self-hosted instance. The goal is to stop telling SaaS users to "sign in with your organization's SSO" when the SSO is Helix's own Keycloak — not the user's organization.

## Codebase Findings

### Existing Config Infrastructure

The frontend already fetches server config via `useGetConfig()` which returns `TypesServerConfigForFrontend`. This object includes:

- `edition?: string` — `"mac-desktop"`, `"server"`, `"cloud"`, etc. Set via `HELIX_EDITION` env var. **Currently empty on app.helix.ml.**
- `auth_provider?: TypesAuthProvider` — `"regular"` or `"oidc"`
- `billing_enabled?: boolean` — `true` on SaaS, typically `false` on self-hosted

The `edition` field is already defined in `api/pkg/types/types.go` (line ~1037) and `api/pkg/config/config.go` (line ~44), wired through to the frontend config endpoint. It just needs to be set in the SaaS deployment environment.

### Files That Need Changes

1. **`frontend/src/pages/Login.tsx`** — Lines ~425-444. The OIDC branch shows:
   - `"Use your organization's single sign-on to access Helix."` (description)
   - `"Sign in with SSO"` (button)
   - Both are hardcoded strings inside JSX, no existing conditional.

2. **`frontend/src/pages/Session.tsx`** — Line ~1628. Login prompt dialog:
   - `"You can login with your Google account or your organization's SSO provider."`

3. **`frontend/src/hooks/useLiveInteraction.ts`** — Lines ~42-44, ~101-117. Dead code:
   - `isAppTryHelixDomain`: hardcoded `window.location.hostname === "app.helix.ml"` check.
   - `isStale` state, `recentTimestamp` state, stale timer `useEffect`, and `isStale` on the `LiveInteractionResult` interface.
   - This was the plumbing for a SaaS upsell prompt (likely "Upgrade" / "contact sales" shown when a response went stale). The UI side was already removed but the hook-side plumbing was left behind.

4. **`frontend/src/components/session/InteractionInference.tsx`** — Lines ~63, ~81, ~462-481. Dead code:
   - `upgrade?: boolean` prop, the `{upgrade && ...}` block rendering an "Upgrade" button, and the `queue_upgrade_clicked` analytics event.
   - Both call sites in `Interaction.tsx` (~lines 218, 302) always pass `upgrade={false}`, so this button can never appear.

5. **`frontend/src/components/session/Interaction.tsx`** — Lines ~218, ~302. Remove `upgrade={false}` prop from both `<InteractionInference>` usages (prop no longer exists after cleanup above).

6. **SaaS deployment config** (infra) — Set `HELIX_EDITION=cloud`.

### How Login.tsx Currently Works

```
Login.tsx (simplified):
  config = useGetConfig()
  isRegular = config?.auth_provider === "regular"

  if (isRegular) {
    → email/password form
  } else {
    → "Use your organization's single sign-on..." text
    → "Sign in with SSO" button (calls account.onLogin())
  }
```

The `isRegular` branch (email/password) is unaffected — this task only touches the `else` (OIDC) branch.

### How Session.tsx Login Prompt Works

Session.tsx renders a `<Window>` modal when `showLoginWindow` is true (triggered when an unauthenticated user tries to interact with a shared session). It currently has access to `account` context but not `config`. It will need to import `useGetConfig`.

## Dead Code Cleanup

The codebase has two layers of remnant SaaS-only upsell plumbing that should be removed:

### `useLiveInteraction.ts` — stale check + hostname guard

This hook computes `isStale` (true when no streaming update received for 10s) but only when `window.location.hostname === "app.helix.ml"`. The `isStale` value is returned in `LiveInteractionResult` but **no component reads it** — `InteractionLiveStream.tsx` destructures `{ message, status, stepInfos, isComplete }` and ignores `isStale`. The stale check was presumably wired to the upgrade prompt that was removed separately.

**Remove**: `isAppTryHelixDomain` memo, `isStale` state, `recentTimestamp` state, the stale-check `useEffect`, and the `isStale` field from the return type and return value. The `recentTimestamp` reset inside the SSE streaming `useEffect` also becomes dead — remove the `setRecentTimestamp(Date.now())` calls and the `if (isStale) { setIsStale(false) }` block.

### `InteractionInference.tsx` — `upgrade` prop + "Upgrade" button

The component accepts `upgrade?: boolean` and renders an "Upgrade" button that emits `queue_upgrade_clicked` and opens the account settings dialog. Both callers in `Interaction.tsx` pass `upgrade={false}`, so this code path is unreachable.

**Remove**: the `upgrade` prop from the interface and destructuring, the `{upgrade && ...}` JSX block, and `upgrade={false}` from both call sites in `Interaction.tsx`.

## Decision: How to Detect SaaS

**Use `edition === "cloud"`.**

Rationale:
- Semantically explicit — "cloud" means Helix Cloud SaaS, which is exactly what we're checking.
- The `edition` field already exists end-to-end (Go config → API → frontend types). No new fields needed.
- One infra change: set `HELIX_EDITION=cloud` in the app.helix.ml deployment.
- `billing_enabled` would also work today, but it's a proxy — an enterprise customer could enable billing too, and they'd want the "your organization" wording.

## Proposed Copy

| Deployment | Description Text | Button Text |
|-----------|-----------------|-------------|
| SaaS (`edition === "cloud"`) | `"Sign in to your Helix account."` | `"Sign in"` |
| Enterprise (anything else) | `"Use your organization's single sign-on to access Helix."` (unchanged) | `"Sign in with SSO"` (unchanged) |

For Session.tsx login prompt:

| Deployment | Text |
|-----------|------|
| SaaS | `"Sign in to your Helix account to continue."` |
| Enterprise | `"You can login with your Google account or your organization's SSO provider."` (unchanged) |

## Implementation Approach

### Helper

Create a small helper constant in `Login.tsx` (no need for a separate file):

```
const isCloud = config?.edition === 'cloud'
```

### Login.tsx Changes

In the OIDC branch (~line 425), wrap the text in a conditional:

```
<Typography ...>
  {isCloud
    ? 'Sign in to your Helix account.'
    : "Use your organization's single sign-on to access Helix."}
</Typography>

<Button ...>
  {isCloud ? 'Sign in' : 'Sign in with SSO'}
</Button>
```

### Session.tsx Changes

Add `useGetConfig` import and use it inside the component:

```
const { data: config } = useGetConfig()
const isCloud = config?.edition === 'cloud'
```

Then in the login window:

```
<Typography gutterBottom>
  {isCloud
    ? 'Sign in to your Helix account to continue.'
    : "You can login with your Google account or your organization's SSO provider."}
</Typography>
```

### Infrastructure Change

In the SaaS deployment environment (app.helix.ml), add:

```
HELIX_EDITION=cloud
```

This is an ops/infra task, not a code change. The Go config already reads `HELIX_EDITION` and passes it through to the frontend.

## What This Does NOT Change

- The email/password login flow (`isRegular` branch) — unaffected.
- The `account.onLogin()` behaviour — same OAuth/OIDC redirect regardless of copy.
- Any backend logic — purely frontend text changes + dead code removal + one env var.
- Enterprise deployments — they keep existing wording with no action needed.
- The core `useLiveInteraction` streaming/message logic — only the stale check is removed.