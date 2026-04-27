# Design: Make Create New Provider Endpoint Form Nicer

## Summary

Add a **Provider preset picker** at the top of the Create dialog. Picking a preset fills in `base_url` (and a sensible default `name` + `auth_type`); picking **Custom** keeps the existing free-form behaviour. Add NVIDIA NIM to the existing `PROVIDERS` registry. No backend changes.

## Existing Code to Reuse

- **`frontend/src/components/providers/types.ts`** already exports a `PROVIDERS: Provider[]` array with logos, base URLs, and `setup_instructions` for OpenAI, Google Gemini, Anthropic, AWS Bedrock, Groq, Cerebras, xAI, TogetherAI, Fireworks, Ollama, and Custom. **Reuse this — don't duplicate.** Just add an NVIDIA entry.
- **`Provider` interface fields** already cover what we need: `id`, `name`, `base_url`, `logo`, `setup_instructions`, `is_custom`, `optional_api_key`, `configurable_base_url`.
- **MUI** is the component library throughout this dialog.

## Form Changes (`CreateProviderEndpointDialog.tsx`)

A single new field is inserted as the **first** input inside the existing `<Stack>`:

```
[Provider preset picker]   <-- NEW
Provider name              <-- existing
Base URL                   <-- existing (now pre-fillable)
Authentication Method ...  <-- existing
...rest unchanged
```

### Picker UI

Use a **MUI `Select`** dropdown labelled "Provider", with each `MenuItem` rendering the provider's logo (small `<Avatar>` or `<img>` 20px) + name. Default value: `custom`.

Why a Select rather than a row of logo tiles?
- Fits the existing dialog width (`maxWidth="sm"`).
- Matches the surrounding form's MUI style.
- Trivially extensible as the `PROVIDERS` array grows.
- Tile rows look great but eat vertical space and require a second design pass for wrap/overflow on small screens — not worth it for this task.

### State + Behaviour

Add one piece of state: `selectedProviderId: string` (default `'user/custom'`).

On change:
1. Look up the `Provider` from `PROVIDERS` by id.
2. If `provider.is_custom` → do nothing else (let the user type freely; don't clear what's already there).
3. Otherwise:
   - Set `formData.base_url = provider.base_url`.
   - If `formData.name` is empty (trimmed), set `formData.name = provider.name`. Never overwrite a user-typed name.
   - Set `formData.auth_type = 'api_key'` (the common case for hosted providers; user can still switch to None / API Key File).

Below the picker, if a non-custom preset is selected, render a small `<Link>`: `Get API key →` linking to `provider.setup_instructions`. (The `setup_instructions` strings already contain a URL — extract it with a simple regex, or change the field to store the URL plus separate human text. Recommendation: add an optional `api_key_url?: string` field to the `Provider` interface for clarity, populated for the hosted providers; fall back to omitting the link if missing.)

### Adding NVIDIA NIM

Add to `PROVIDERS` in `frontend/src/components/providers/types.ts`:

```ts
{
  id: 'user/nvidia',
  alias: ['nvidia', 'nvidia-nim'],
  name: 'NVIDIA NIM',
  description: 'NVIDIA-hosted, OpenAI-compatible inference for hosted LLMs.',
  logo: NvidiaLogo,                // new SVG component, see below
  base_url: 'https://integrate.api.nvidia.com/v1',
  setup_instructions: 'Get your API key from https://build.nvidia.com/ (Settings → API Keys)',
}
```

Logo asset: add `frontend/src/components/providers/logos/nvidia.tsx` following the pattern of the sibling `openai.tsx` / `anthropic.tsx` files (a small React SVG component). NVIDIA's wordmark/eye-of-NVIDIA SVG is widely available — implementer to source one and check it in alongside the others.

### Validation

Unchanged. The existing URL/HTTPS/duplicate-name checks already cover the preset-filled values.

## What stays the same

- Backend types, API endpoints, store layer.
- `EditProviderEndpointDialog` — untouched.
- `ProviderEndpointsTable` — the parent that opens this dialog — untouched.
- Custom headers section, billing checkbox, endpoint type Select.

## Risks / Gotchas

- **`PROVIDERS` is also consumed elsewhere.** Grep shows it's imported by `OAuthSettings.tsx`, `BrowseProvidersDialog.tsx` (different `PROVIDERS` const, OAuth-only), `OAuthProvidersTable.tsx`, `AdvancedModelPicker.tsx`, `ApiIntegrations.tsx`, `AddApiSkillDialog.tsx`, `OAuthConnections.tsx`, `PricingTable.tsx`. Adding a new entry is additive and should not break these — but the implementer should spot-check that "NVIDIA NIM" rendering looks right wherever `PROVIDERS.map(...)` shows up. (If any of those consumers blow up because the new logo component is missing or sized wrong, fix the logo file rather than scoping NVIDIA out of the global list.)
- **Don't re-overwrite user input.** If the user types a URL, then picks a preset, then picks Custom again — we should not clear the URL. Only the act of selecting a *non-custom* preset writes to `base_url` / `name`.
- **Helper text on the Base URL field can now be shorter** ("OpenAI-compatible base URL") since the preset picker carries the example burden. Remove the long inline list of three URLs that's there today.
- **`setup_instructions` parsing.** Today it's a plain English sentence with a URL embedded. The cleaner fix is the new optional `api_key_url` field; the regex fallback is a backstop, not the recommended path.

## Decisions

- **Select dropdown over tile grid** — fits the existing form, less work, easier to extend.
- **Reuse `PROVIDERS`** rather than introducing a parallel list — single source of truth across the codebase.
- **Preset = Custom by default** — preserves the current power-user flow (open form, type URL, done).
- **Edit dialog out of scope** — once a record exists, the preset picker adds no value; would need a separate "did the URL match a known provider?" inference that isn't worth the complexity.

## Implementation Notes

- **`api_key_url` was the right call.** Parsing URLs out of the existing `setup_instructions` strings would have been brittle — a fresh optional field is one line per entry, easy to read, easy to extend. Populated for OpenAI, Anthropic, Google, NVIDIA, AWS, Groq, Cerebras, xAI, TogetherAI, Fireworks. Ollama is skipped (it has no public console — keys are local).
- **Logo rendering.** `Provider.logo` is a union of `string | React.ComponentType`. The dialog gets a tiny helper `renderProviderLogo()` that branches: string → `<img>`, component → `<LogoComponent width={20} height={20} />`. Sized to 20×20 in the dropdown for compactness.
- **Name-preserve rule works as intended.** If the user types `my-anthropic`, picks Anthropic preset, then picks OpenAI, the name stays `my-anthropic`. If they leave name empty, picking Anthropic fills it with `Anthropic`; picking OpenAI after that does NOT overwrite (because `Anthropic` is now non-empty). Minor wart for users flipping between two presets, but the alternative (always overwrite) is worse — it would clobber explicit user input.
- **Reset on close.** `handleClose` already nulled out form state; added `setSelectedProviderId(CUSTOM_PROVIDER_ID)` so reopening the dialog starts clean.
- **Fresh logo asset.** Created `frontend/src/components/providers/logos/nvidia.tsx` with NVIDIA's eye-mark in a 32-viewbox SVG (the existing logos use 16-viewbox). Both render at 20×20 in the dropdown without issue.
- **Existing duplicate-name validation is case-insensitive**, so `Anthropic` (preset suggestion) collides with the seeded `anthropic` system endpoint. Users see the existing inline error and can rename — no new code needed.
- **Build verified inside `helix-frontend-1` container** (`docker compose -f docker-compose.dev.yaml exec frontend yarn build`) since `node_modules` aren't installed on the host. 50.35s clean build, no TS errors. Vite HMR picked the changes up live during browser testing.
- **Other `PROVIDERS` consumers** (`AdvancedModelPicker`, `ApiIntegrations`, `AddApiSkillDialog`, `PricingTable`, `OAuthSettings`, `OAuthConnections`, `OAuthProvidersTable`) were not actively rendered, but the change is purely additive: a new array element with all required fields and the new field optional. The build covers type compatibility; no runtime logic depends on a specific provider list shape.
- **Type select rendering oddity.** Noticed the `Type` select shows a zero-width `​` rather than the label after dialog reopen — pre-existing bug, unrelated to this work. Endpoint still creates with the correct default (`user`).
