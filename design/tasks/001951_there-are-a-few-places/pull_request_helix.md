# Hide secrets and keys in the UI

## Summary

The agent settings "Keys" tab was rendering API keys in plain text — visually
masked in the table (`xxxxx...xxx`) but the **full key was rendered inline in
the cURL / Python / Go / JS code examples** in the right-hand panel. This PR
hides keys by default everywhere they appear on that tab, with an explicit
reveal toggle and auto-hide on tab blur or after 30 s.

## Changes

- **New widget `frontend/src/components/widgets/MaskedSecret.tsx`** — fixed-length
  bullet mask, eye-icon reveal toggle, copy button, 30 s auto-hide, hide
  immediately when the tab loses focus. Styled explicitly for dark mode (text
  `#F8FAFC` / border `#4A5568` / focused border `#3182CE`) so it doesn't depend
  on the missing global MUI theme overrides.
- **`frontend/src/components/app/APIKeysSection.tsx`** — replaced the
  `Chip` + `CopyKeyButton` group (which leaked first 5 + last 3 chars of every
  key) with `<MaskedSecret value={apiKey.key} />`.
- **`frontend/src/components/app/CodeExamples.tsx`** — added a "Show / hide
  key in code" toggle. By default the rendered snippet shows
  `"hl-••••••••"`, but the COPY button always writes the snippet with the
  real key. Same 30 s + visibility-blur auto-hide rule.

## Audit notes

The original design also called out
`frontend/src/components/settings/OAuthSettings.tsx` (RSA private key field).
During implementation I discovered that file is dead code — zero imports
across the codebase, last touched in two `wip` commits. The live OAuth
provider admin (`frontend/src/components/dashboard/OAuthProvidersTable.tsx`)
has no private key field at all and already uses `type="password"` for
`client_secret`. So no change is needed there. Other audited locations
(`Account.tsx`, `Secrets.tsx`, `AddProviderDialog.tsx`, `GitRepoDetail.tsx`)
were already masking correctly and are unchanged.

## Testing

- `yarn build` (in `helix-frontend` container) — passes, no type errors.
- Manual end-to-end in the inner Helix:
  - Registered, created an org and an agent, opened the Keys tab.
  - Default state: key shows as `••••••••••••••••`; copy works.
  - Click eye → key revealed; click again → re-hidden; auto-hides after 30 s.
  - Code examples panel: default `"hl-••••••••"`; COPY copies the real key
    regardless; "Show key in code" toggle reveals.
  - Dark theme verified — no white form elements.

## Screenshots

![Keys tab — masked by default](https://github.com/helixml/helix/raw/helix-specs/design/tasks/001951_there-are-a-few-places/screenshots/01-keys-tab-masked-default.png)

![Keys tab — key revealed](https://github.com/helixml/helix/raw/helix-specs/design/tasks/001951_there-are-a-few-places/screenshots/02-keys-tab-revealed.png)

![Code example — key revealed in snippet](https://github.com/helixml/helix/raw/helix-specs/design/tasks/001951_there-are-a-few-places/screenshots/03-code-example-revealed.png)

## Out of scope (follow-ups)

- Backend hardening: `GET /apps/:id/api_keys` still returns the full key
  string. Industry standard (GitHub / Stripe / AWS) is to return the key once
  on creation and never again. That's a breaking API change with downstream
  impact (CLI, generated SDK) — separate task.
- Deleting the dead `OAuthSettings.tsx` file — separate cleanup task.

Design doc: `/design/tasks/001951_there-are-a-few-places/`
