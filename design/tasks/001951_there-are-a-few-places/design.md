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

## Dark theme consistency

The reviewer flagged that form elements on the current Keys tab render with light/white backgrounds, breaking the dark theme. This is a known shape of bug in this codebase: the global MUI `ThemeProvider` (`frontend/src/contexts/theme.tsx`) only defines overrides for `MuiDialog`, `MuiMenu`, and `MuiCssBaseline` — there are **no global overrides** for `MuiTextField`, `MuiChip`, `MuiTable`, `MuiPaper`, etc. So those components fall back to MUI defaults, which include light surface colours that fight the dark `#121214` / `#1e1e24` palette defined in `frontend/src/themes.tsx`.

Workaround pattern already in the codebase: locally-styled wrappers like `DarkTextField` defined ad-hoc in `frontend/src/components/app/AzureDevOpsSkill.tsx:51-73` and `frontend/src/components/app/AddApiSkillDialog.tsx:89` (explicit `#F8FAFC` text on `#4A5568` border for outlined fields). We follow the same convention rather than touching the global theme — fixing the global theme is a much larger change with risk of regression across every screen.

Rules for this task:

- **`<MaskedSecret>` must be dark-theme-correct without caller overrides.** Use either `useTheme()` to read `theme.palette` and apply explicit `color` / `backgroundColor` / `borderColor`, or build it as a `styled()` wrapper following the `DarkTextField` pattern at `AzureDevOpsSkill.tsx:51-73`. Hex values to match the existing dark wrappers: text `#F8FAFC` / `#e0e0e0`, border `#4A5568`, focused border `#3182CE`, panel `#1e1e24`.
- **Audit every component touched on the Keys tab**, not just the secret display:
  - `APIKeysSection.tsx` — the `<Chip variant="outlined">` and `<Button variant="outlined">` need verification under dark mode; if either renders light, switch to a styled wrapper or set explicit `sx` colours.
  - `CodeExamples.tsx` — the new "Show key" toggle button must match the existing dark "Copy" overlay (`rgba(0, 0, 0, 0.6)` background, light text).
  - `OAuthSettings.tsx` private-key field — the masked-when-not-focused state must use the dark surface, not a default light TextField.
- **Do not introduce a new colour palette.** Reuse the hex values already in `themes.tsx` (`#121214`, `#1e1e24`, `#e0e0e0`) or the `DarkTextField` palette referenced above. No new "branded" colours.
- **Out of scope** — promoting the local `DarkTextField` pattern to a shared component, or adding `MuiTextField` / `MuiChip` overrides to the global theme. Those are tempting cleanups but would touch every screen in the app and need their own task.

Verification step in `tasks.md` covers a side-by-side visual check against an adjacent tab (e.g. "MCP") to confirm no jarring colour break when switching tabs in the agent settings sidebar.

## Decisions

- **No partial masking.** Either fully hidden or fully revealed. Reasoning: prefix/suffix leakage is what the user is asking us to remove, and an inconsistent mid-mask between screens is worse than one consistent rule.
- **No backend change in this iteration.** The "right" long-term fix is for `GET /apps/:id/api_keys` to never return the full key after creation (cf. GitHub, Stripe, AWS). That is a breaking API change with downstream impact (CLI, generated SDK, code examples panel) — handled as a follow-up task, not in this PR.
- **Auto-hide on reveal.** 30 s timer + immediate hide on tab/window blur. Catches the common "I revealed it to copy and then walked away" case.
- **Same lucide icons as the rest of the file.** `APIKeysSection.tsx` uses `lucide-react`; `Account.tsx` uses MUI icons. Use lucide in `MaskedSecret` to match the surface where the primary fix lands; both icon sets coexist in the codebase already.
- **Style `MaskedSecret` defensively against the missing global theme overrides.** Don't assume `MuiTextField` / `MuiChip` defaults will look right in dark mode — they don't. Apply explicit colours from the `DarkTextField` palette (`AzureDevOpsSkill.tsx:51-73`) or read `useTheme()`. See "Dark theme consistency" section above.

## Codebase notes for future agents

- The agent "Keys" tab is rendered from `frontend/src/pages/App.tsx:271` via `<APIKeysSection>`, with a sibling `<CodeExamples>` panel at `frontend/src/pages/App.tsx:285` that receives `account.appApiKeys[0]?.key` — both surfaces show the same key, so both must be addressed together.
- The Helix codebase already has **three different** secret-masking patterns in production: full mask + no reveal (Secrets), password field + reveal (Account / Git), and "smart" prefix/suffix mask (Providers / current Agent Keys). Consolidating onto one pattern is the point of the new shared component.
- `frontend/CLAUDE.md` rule: use the generated API client (`api.getApiClient()`), never raw fetch. This task is UI-only so no API calls are added; just preserved.
- Test the change in the inner Helix at `http://localhost:8080` — register `test@helix.ml` / `helixtest`, create an org, create an agent, add an API key, verify the Keys tab.
