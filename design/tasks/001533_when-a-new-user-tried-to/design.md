# Design: Google AI API Key Onboarding Fix

## Root Cause

**File:** `frontend/src/components/providers/AddProviderDialog.tsx`, `handleSubmit` (line 127)

`useCreateProviderEndpoint()` returns `{ mutate }`. The dialog destructures `mutate` and aliases it:
```ts
const { mutate: createProviderEndpoint } = useCreateProviderEndpoint();
```

Then in `handleSubmit`:
```ts
await createProviderEndpoint({...}); // mutate returns void — await resolves immediately
snackbarSuccess('Provider connected successfully'); // ALWAYS fires
handleClose();                                       // ALWAYS fires
```

`mutate` returns `void`, so `await void` resolves to `undefined` instantly. The try/catch never catches anything. The mutation result is handled by React Query's internal `onError`, but `useCreateProviderEndpoint` in `providersService.ts` defines no `onError`, so failures are silently dropped.

**Fix:** Switch to `mutateAsync` (returns a Promise that rejects on error):
```ts
const { mutateAsync: createProviderEndpoint } = useCreateProviderEndpoint();
```
This makes the existing try/catch work correctly — on failure, `setError(...)` is called and the dialog stays open showing the error.

## Secondary Issue: Google Model Name Prefix

`listGoogleModels` in `api/pkg/openai/openai_client_google.go` (line 81) sets `ID: model.Name` where Google returns names like `"models/gemini-2.5-flash"`. The `autoSetKodit` effect in `Onboarding.tsx` (line 434) hardcodes `model: "gemini-2.5-flash"` (no prefix). These IDs won't match.

**Fix:** Strip the `models/` prefix from Google model IDs in `listGoogleModels`, so model IDs are consistent with what other providers return (e.g. `"gemini-2.5-flash"`, not `"models/gemini-2.5-flash"`).

## Code Paths

| File | Purpose |
|------|---------|
| `frontend/src/components/providers/AddProviderDialog.tsx:80` | `mutate` alias — change to `mutateAsync` |
| `frontend/src/components/providers/AddProviderDialog.tsx:127` | `handleSubmit` — fix awaiting |
| `api/pkg/openai/openai_client_google.go:81` | model ID construction — strip `models/` prefix |
| `frontend/src/pages/Onboarding.tsx:434` | `autoSetKodit` — model name must match after prefix strip |

## Pattern Note

This project uses `mutateAsync` (not `mutate`) in dialog submit handlers where errors need to surface in the UI. `mutate` is only appropriate for fire-and-forget calls with no UI error feedback. See `providersService.ts` — `useCreateProviderEndpoint` already has no `onError` handler, so all error surfacing must be done at the call site via `mutateAsync` + try/catch.
