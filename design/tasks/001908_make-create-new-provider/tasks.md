# Implementation Tasks

- [x] Add an `api_key_url?: string` field to the `Provider` interface in `frontend/src/components/providers/types.ts`, and populate it for the hosted entries (OpenAI, Anthropic, Google Gemini, NVIDIA NIM, Groq, Cerebras, xAI, TogetherAI, Fireworks, AWS Bedrock).
- [x] Add an `NvidiaLogo` SVG React component at `frontend/src/components/providers/logos/nvidia.tsx` following the existing `openai.tsx` / `anthropic.tsx` pattern.
- [x] Add a new entry to `PROVIDERS` for **NVIDIA NIM** with `base_url: 'https://integrate.api.nvidia.com/v1'`, the new logo, and `api_key_url: 'https://build.nvidia.com/'`.
- [x] In `CreateProviderEndpointDialog.tsx`, add a `selectedProviderId` state (default `'user/custom'`).
- [x] Insert a new **Provider** `<Select>` as the first input inside the dialog `<Stack>`, populated from `PROVIDERS` (logo + name in each `MenuItem`).
- [x] Wire the picker's `onChange`: when a non-custom preset is chosen, set `formData.base_url` to the preset URL, set `formData.name` to the provider name *only if* the name field is empty, and set `formData.auth_type = 'api_key'`. When **Custom** is chosen, do nothing.
- [x] Below the picker, render a small "Get API key →" `<Link>` (target `_blank`, `rel="noopener"`) when a non-custom preset is selected, sourced from `provider.api_key_url`. Hide it for Custom.
- [x] Shorten the existing Base URL `helperText` to a brief "OpenAI-compatible base URL" — the preset picker now carries the example burden.
- [~] Verify the rest of the form (validation, auth radios, endpoint type, billing checkbox, custom headers, submit) still works unchanged.
- [ ] Smoke-test the other components that import `PROVIDERS` (`AdvancedModelPicker.tsx`, `ApiIntegrations.tsx`, `AddApiSkillDialog.tsx`, `PricingTable.tsx`, `OAuthSettings.tsx`, `OAuthConnections.tsx`, `OAuthProvidersTable.tsx`) to confirm the new NVIDIA entry renders cleanly wherever the list is iterated.
- [ ] In the inner Helix at `http://localhost:8080`: register / log in, open Dashboard → Provider Endpoints → "Add Endpoint", then verify (a) Custom default behaves like today, (b) picking OpenAI / Anthropic / Google / NVIDIA fills the URL, (c) the "Get API key" link opens the right page, (d) creating an endpoint with a preset succeeds.
- [ ] `cd frontend && yarn build` to confirm the production build is clean before pushing.
