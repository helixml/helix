# Requirements: Make Create New Provider Endpoint Form Nicer

## Problem

The current "Create New Provider Endpoint" dialog (`frontend/src/components/dashboard/CreateProviderEndpointDialog.tsx`) presents users with a blank `Base URL` text field and a helper-text hint listing a few example URLs (OpenAI, Anthropic, Google). Users must copy-paste the correct URL. This is the form that admins / power users hit every time they wire up a new LLM provider, and the most common targets are a small, well-known set: OpenAI, Anthropic, Google Gemini, and (more recently) NVIDIA NIM at `https://integrate.api.nvidia.com/v1`.

NVIDIA isn't listed anywhere in the codebase yet — neither in the helper text nor in the existing `PROVIDERS` registry at `frontend/src/components/providers/types.ts`.

## User Stories

**As an admin setting up a new LLM provider**, I want to pick from a list of common providers (OpenAI, Anthropic, Google, NVIDIA) so that the Base URL, suggested name, and auth method are filled in automatically — without me having to find the right URL elsewhere.

**As an admin setting up a less-common provider**, I want to keep typing my own Base URL (the existing behaviour) so that nothing forces me into a preset.

**As a user who picked the wrong preset**, I want the Base URL field to remain editable so that I can still tweak it (e.g. switch region for AWS, or point Ollama at a different host).

## Acceptance Criteria

1. The Create dialog shows a **Provider** selector at the top with at least: **OpenAI, Anthropic, Google Gemini, NVIDIA NIM, Custom**.
2. Selecting a preset (anything other than Custom) auto-fills:
   - `base_url` with the canonical URL for that provider
   - `name` with the provider name *if* the name field is empty (don't overwrite user input)
   - `auth_type` = `api_key`
3. The Base URL field remains a normal editable `TextField`. Switching the preset back to Custom (or editing the URL by hand) does not erase user input.
4. When a preset is selected, a small "Get API key →" link points to the provider's key/console page (already captured in `setup_instructions` in `PROVIDERS`).
5. `https://integrate.api.nvidia.com/v1` is added to the `PROVIDERS` registry as **NVIDIA NIM** so it shows up alongside the others (also makes it available to any future component that consumes `PROVIDERS`).
6. The preset selector defaults to **Custom** on first open, so existing power-user workflow (just type a URL) is unchanged.
7. All existing form fields (auth method radios, endpoint type, billing checkbox, custom headers, validation) keep working unchanged.

## Out of Scope

- "Test connection" button before submit.
- Auto-discovering models after URL entry.
- Vertex AI–specific fields (`vertex_project_id`, etc.) — the backend has them but the form doesn't expose them; leave that for a separate task.
- Editing the `EditProviderEndpointDialog` — only the Create dialog is in scope. (Edit already has a real saved record; preset picking doesn't add value there.)
