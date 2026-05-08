# Add provider preset picker to Create New Provider Endpoint dialog

## Summary

Most users hit the Create Provider Endpoint dialog to wire up OpenAI, Anthropic, Google, or NVIDIA — and the form left them copy-pasting URLs out of helper text. This PR adds a Provider preset dropdown at the top of the dialog that auto-fills `base_url`, suggests a default `name`, sets `auth_type` to `api_key`, and shows a direct "Get API key" link to the provider's console.

NVIDIA NIM (`https://integrate.api.nvidia.com/v1`) is added to the existing `PROVIDERS` registry so it appears alongside the other hosted providers.

## Changes

- **`frontend/src/components/providers/types.ts`**
  - Added optional `api_key_url?: string` to the `Provider` interface and populated it for OpenAI, Anthropic, Google Gemini, NVIDIA NIM, AWS Bedrock, Groq, Cerebras, xAI, TogetherAI, Fireworks.
  - Added the **NVIDIA NIM** entry (id `user/nvidia`, base URL `https://integrate.api.nvidia.com/v1`).
- **`frontend/src/components/providers/logos/nvidia.tsx`** *(new)* — NVIDIA eye-mark SVG logo, same pattern as the sibling logos.
- **`frontend/src/components/dashboard/CreateProviderEndpointDialog.tsx`**
  - New **Provider** `<Select>` at the top of the dialog, populated from `PROVIDERS` with each option rendering its logo + name. Defaults to **Custom Provider** so existing behavior is unchanged.
  - Picking a non-Custom preset writes `base_url`, sets `auth_type = 'api_key'`, and fills `name` only if the field is empty (never overwrites user input).
  - Renders a **Get API key for &lt;Provider&gt;** link below the picker, sourced from the new `api_key_url` field.
  - Shortened the Base URL helper text — the picker now carries the example burden.

## Behavior

- **Default (Custom):** form behaves exactly as before — type your own name, URL, auth.
- **Pick OpenAI / Anthropic / Google Gemini / NVIDIA NIM / etc.:** URL pre-fills, name suggested if empty, API key field shown, deep link to the provider's console appears.
- **Switch back to Custom:** existing field values are preserved, not cleared.

## Screenshots

![Default state — Custom provider](https://github.com/helixml/helix/raw/helix-specs/design/tasks/001908_make-create-new-provider/screenshots/01-dialog-default-custom.png)

![Provider dropdown open — all 12 providers with logos](https://github.com/helixml/helix/raw/helix-specs/design/tasks/001908_make-create-new-provider/screenshots/02-provider-dropdown-open.png)

![NVIDIA NIM preset applied — URL, name, auth, and key link auto-filled](https://github.com/helixml/helix/raw/helix-specs/design/tasks/001908_make-create-new-provider/screenshots/03-nvidia-preset-applied.png)

![Switching back to Custom preserves user input](https://github.com/helixml/helix/raw/helix-specs/design/tasks/001908_make-create-new-provider/screenshots/04-back-to-custom-preserves-state.png)

![End-to-end: endpoint created with Anthropic preset](https://github.com/helixml/helix/raw/helix-specs/design/tasks/001908_make-create-new-provider/screenshots/05-endpoint-created.png)

## Test plan

- [x] `yarn build` passes cleanly (built in 50.35s, no errors)
- [x] Dialog opens with **Custom Provider** selected; all existing fields render
- [x] Picking each preset (OpenAI, Anthropic, Google Gemini, NVIDIA NIM) fills Base URL and shows the right "Get API key" link
- [x] User-typed `name` is preserved when switching presets; auto-suggested name is replaced only when blank
- [x] Switching back to **Custom** keeps the user's data
- [x] End-to-end: created an endpoint with the Anthropic preset; new row appears in the table with the correct base URL
- [x] No new console errors / warnings in the dev server
