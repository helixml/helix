# Implementation Tasks

- [x] In `AddProviderDialog.tsx`, change `useCreateProviderEndpoint()` destructuring from `mutate` to `mutateAsync` so that `await createProviderEndpoint(...)` properly rejects on API failure and the catch block can call `setError()`
- [~] In `openai_client_google.go`, `listGoogleModels`: strip the `models/` prefix from model IDs (e.g. `"models/gemini-2.5-flash"` → `"gemini-2.5-flash"`) to match the naming convention used by other providers
- [ ] Verify the `autoSetKodit` hardcoded model name `"gemini-2.5-flash"` now matches the stripped model ID returned from the Google API
- [ ] Build frontend (`yarn build`) and confirm the dialog shows an error inline when an invalid API key is submitted, rather than closing with a false success toast
