# Requirements: Google AI API Key Onboarding Failure

## Problem

When a new user adds a Google AI (Gemini) API key during onboarding, they are unable to make progress. The provider dialog silently swallows errors: it always shows "Provider connected successfully" and closes, regardless of whether the API call actually succeeded. If the backend rejected the request, the provider is never saved, no green checkmark appears on the Google Gemini card, and the user is stuck with the Continue button disabled.

## User Stories

**As a new user**, I want to add my Google AI API key during onboarding so that I can use Gemini models in Helix.

**Acceptance criteria:**
- If the API call to save the provider fails, an error message is shown inside the dialog (not a silent close)
- If the provider saves but model listing fails (timeout, bad key, etc.), the onboarding step still shows a meaningful message so the user understands the key was accepted but models couldn't be loaded
- A valid Google AI API key (AIzaSy... format from aistudio.google.com) results in a connected provider with models available for agent creation

## Secondary Issue

The `autoSetKodit` effect hardcodes `"gemini-2.5-flash"` as the kodit enrichment model, but Google's model list API returns IDs with a `models/` prefix (e.g. `"models/gemini-2.5-flash"`). This mismatch means the enrichment model is set to a non-matching ID.
