# Requirements — Hide secrets and keys in the UI

## Problem

Several Helix UI screens render API keys and other secrets in plain text, leaving them visible to anyone glancing at the screen, captured in screenshots, or scraped by screen-recording tools.

The user-named example is the agent settings page:
`https://app.helix.ml/orgs/<org>/agent/<app_id>?tab=apikeys`

Even though the table on that screen visually masks each key with `xxxxx...xxx`, the right-hand "code examples" panel still renders the **full** key inline in cURL / Python / JS snippets. So the key is still on-screen in plain text.

There are other places with the same shape of problem (audit list in design.md).

## User stories

### US-1 — Agent API keys (primary)
**As an** agent owner viewing my agent's "Keys" tab,
**I want** the API key value to be hidden by default with an explicit reveal action,
**so that** I don't expose the key when sharing my screen, recording a demo, or having someone watch over my shoulder.

### US-2 — Code examples panel
**As an** agent owner copying a code example,
**I want** the embedded API key inside the code snippet to be masked by default but still copyable,
**so that** the snippet I copy contains the real key, but the screen does not show it.

### US-3 — Other secret-bearing screens
**As a** Helix user managing OAuth providers, organization API keys, secrets, git credentials, and provider integrations,
**I want** consistent secret-masking behaviour across all those screens,
**so that** I don't have to remember which screens leak and which don't.

## Acceptance criteria

### Agent API keys tab (`?tab=apikeys`)
- [ ] Each key in the table is rendered fully masked by default (e.g. `••••••••••••••••`), with no visible prefix/suffix characters.
- [ ] A reveal toggle (eye icon) per row shows the full key on click; clicking again re-hides it.
- [ ] Revealed keys auto-hide after 30 seconds of being visible, or on tab/focus change of the page.
- [ ] The copy-to-clipboard button still copies the full key without requiring reveal.
- [ ] The table column no longer leaks any characters of the key (today: first 5 + last 3).

### Code examples panel
- [ ] When `apiKey` is present, the rendered snippet shows a masked placeholder (e.g. `hl-••••••••`) instead of the real key.
- [ ] A page-level "Show key in code examples" toggle reveals the real key inside the snippets.
- [ ] The "Copy" button always copies the snippet with the real key, regardless of toggle state.

### Audit & sweep
- [ ] OAuth provider RSA private key field is masked (currently plain `multiline` TextField).
- [ ] All other identified locations (see `design.md` § Audit) use a consistent masked-secret pattern (`type="password"` or the new shared component) — no full secret rendered visibly by default.
- [ ] No new "decorative" partial mask (e.g. `abc12...xyz`): either fully hidden or fully revealed.

### Cross-cutting
- [ ] No backend API changes required for this iteration (frontend-only). Backend hardening (return only masked keys after creation) is called out as follow-up in `design.md` but is out of scope.
- [ ] Behaviour works in both dev (`yarn dev`) and prod (`yarn build`) modes.
- [ ] Verified manually in the inner Helix at `http://localhost:8080`.

## Out of scope

- Server-side changes to stop returning full key values on `GET /apps/:id/api_keys` (called out as follow-up).
- Audit-logging of "reveal" clicks.
- Rotating or invalidating keys that may already have been leaked via screenshots.
- Telemetry / metrics on secret reveal usage.
