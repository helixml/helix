# Design — Hide secrets and keys in the UI

## Approach

Frontend-only change. Introduce one shared `<MaskedSecret>` component and use it (or the equivalent `type="password"` pattern) everywhere a secret is currently rendered visibly.

We deliberately do **not** keep the existing partial-mask pattern (`abc12...xyz`). It leaks the prefix/suffix, gives a false sense of security, and is inconsistent with the rest of the UI — pick "fully hidden + reveal" and use it everywhere.

## Audit — places that show a secret in plain text today

| # | File | What's leaked | Current state | Action |
|---|------|---------------|---------------|--------|
| 1 | `frontend/src/components/app/APIKeysSection.tsx:106` | Agent API key (table row) | Partial mask `xxxxx...xxx`, full key in DOM | **Replace** with `<MaskedSecret>` |
| 2 | `frontend/src/components/app/CodeExamples.tsx:79` | Agent API key (cURL/Python/JS snippets) | Full key rendered inline in syntax-highlighted code | Display masked placeholder; page-level reveal toggle; copy still uses real key |
| 3 | `frontend/src/components/settings/OAuthSettings.tsx:328-338` | OAuth 1.0a RSA private key | Plain multiline TextField | Add `type="password"` masking + reveal toggle (multiline mask via custom render) |
| 4 | `frontend/src/pages/Account.tsx:283-320` | User/org API keys | Already password field with reveal toggle | **No change** (already meets the bar) |
| 5 | `frontend/src/pages/Secrets.tsx:155` | Secret values list | Already `*****` (no reveal) | **No change** (already meets the bar; intentional no-reveal) |
| 6 | `frontend/src/components/providers/AddProviderDialog.tsx:98-104` | Provider API key edit | Smart masking + reveal toggle | **No change** (already meets the bar) |
| 7 | `frontend/src/pages/GitRepoDetail.tsx:1192-1212` | Git password / PAT | Password field + reveal toggle | **No change** (already meets the bar) |
| 8 | `frontend/src/components/settings/OAuthSettings.tsx:241` | OAuth client secret | Already `type="password"` | **No change** (already meets the bar) |

So the actual code changes land in items **1, 2, 3** plus a new shared component.

## New shared component: `<MaskedSecret>`

Location: `frontend/src/components/widgets/MaskedSecret.tsx`

Props:

```ts
interface MaskedSecretProps {
  value: string;                 // the real secret
  monospace?: boolean;           // default true
  size?: 'small' | 'medium';     // default small
  copyable?: boolean;            // default true
  revealable?: boolean;          // default true
  autoHideMs?: number;           // default 30_000; 0 disables auto-hide
  ariaLabel?: string;            // for screen readers; defaults to "Secret value"
}
```

Behaviour:
- Renders `••••••••••••••••` (16 bullets, fixed width — does not encode length) by default.
- Eye icon toggles between hidden and revealed.
- When revealed, starts a `setTimeout(autoHideMs)` to re-hide; cancelled if user toggles manually first.
- When document loses focus (`visibilitychange` → `hidden`), immediately re-hide.
- Copy icon writes `value` to clipboard via `navigator.clipboard.writeText`; shows a transient "Copied!" tooltip.
- Visually consistent with the existing reveal-toggle pattern in `Account.tsx` (Visibility / VisibilityOff icons from MUI or `lucide-react` to match local convention — `APIKeysSection.tsx` already uses lucide).

## Changes per file

### 1. `APIKeysSection.tsx`
- Delete `maskKey()` and `CopyKeyButton` (replaced by `<MaskedSecret>`).
- Replace the `<Chip>` + `<CopyKeyButton>` group with `<MaskedSecret value={apiKey.key} />`.

### 2. `CodeExamples.tsx`
- Add a single `showKey` state (default `false`) and a "Show key" toggle button next to the existing "Copy" button.
- Compute `displayKey = showKey ? apiKey : 'hl-••••••••'` and pass `displayKey` to `example.code(address, displayKey)` for **rendering**.
- For the **copy** action, always pass the real `apiKey` to `example.code(...)` — copy gets the working snippet.
- Auto-hide after 30 s and on `visibilitychange` (same rule as `MaskedSecret`).

### 3. `OAuthSettings.tsx`
- Private key field (line 328-338): keep `multiline` for editing, but default the field to `type="password"` is not supported with `multiline`. Two options:
  - **(A) Preferred:** Render `<MaskedSecret>` when there is an existing value and the user is not actively editing; switch to a plain multiline TextField on click-to-edit. Mirrors the `AddProviderDialog` masked-when-not-focused pattern.
  - (B) Simpler fallback: render `value` as fixed-length bullets when not focused; reveal on focus. Same UX as (A) but without the dedicated component.
- Pick (A) for consistency.

## Decisions

- **No partial masking.** Either fully hidden or fully revealed. Reasoning: prefix/suffix leakage is what the user is asking us to remove, and an inconsistent mid-mask between screens is worse than one consistent rule.
- **No backend change in this iteration.** The "right" long-term fix is for `GET /apps/:id/api_keys` to never return the full key after creation (cf. GitHub, Stripe, AWS). That is a breaking API change with downstream impact (CLI, generated SDK, code examples panel) — handled as a follow-up task, not in this PR.
- **Auto-hide on reveal.** 30 s timer + immediate hide on tab/window blur. Catches the common "I revealed it to copy and then walked away" case.
- **Same lucide icons as the rest of the file.** `APIKeysSection.tsx` uses `lucide-react`; `Account.tsx` uses MUI icons. Use lucide in `MaskedSecret` to match the surface where the primary fix lands; both icon sets coexist in the codebase already.

## Codebase notes for future agents

- The agent "Keys" tab is rendered from `frontend/src/pages/App.tsx:271` via `<APIKeysSection>`, with a sibling `<CodeExamples>` panel at `frontend/src/pages/App.tsx:285` that receives `account.appApiKeys[0]?.key` — both surfaces show the same key, so both must be addressed together.
- The Helix codebase already has **three different** secret-masking patterns in production: full mask + no reveal (Secrets), password field + reveal (Account / Git), and "smart" prefix/suffix mask (Providers / current Agent Keys). Consolidating onto one pattern is the point of the new shared component.
- `frontend/CLAUDE.md` rule: use the generated API client (`api.getApiClient()`), never raw fetch. This task is UI-only so no API calls are added; just preserved.
- Test the change in the inner Helix at `http://localhost:8080` — register `test@helix.ml` / `helixtest`, create an org, create an agent, add an API key, verify the Keys tab.
