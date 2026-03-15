# Requirements: Claude Agent UX Parity in Zed

## Background

Helix's settings-sync-daemon writes an `agent_servers.claude` config to Zed's `settings.json`. This configuration is missing fields that Zed needs to render the model selector and bypass-permissions toggle in its UI. Users launching the built-in "Claude Agent" via Zed's agent panel get these controls; users whose session uses the daemon-written config don't.

## User Stories

**US-1:** As a Helix user with a Claude subscription session, I want to see a model selector in the Zed Claude agent panel so I can choose which Claude model to use.

**US-2:** As a Helix user, I want to be able to toggle bypass-permissions mode in the Zed UI so I can control whether Claude asks me before running shell commands.

**US-3:** As a Helix user, my model and mode preferences should survive the daemon's 30-second settings sync cycle (i.e., the daemon should not clobber selections I made in the UI).

## Acceptance Criteria

- **AC-1:** When the daemon configures Claude in subscription mode, `favorite_models` is populated with the available Claude models (e.g., `claude-opus-4-6`, `claude-sonnet-4-6`, `claude-haiku-4-5-20251001`) so Zed renders the model selector.
- **AC-2:** When the daemon configures Claude in API key mode, `favorite_models` lists the models available through the Helix proxy.
- **AC-3:** The daemon does NOT hardcode `default_mode: "bypassPermissions"` on every sync; instead it preserves the value the user last set via the UI (treating `default_mode` as a user-owned preference).
- **AC-4:** If a user has not yet chosen a mode, the daemon sets a sensible default (`bypassPermissions` for subscription, omitted or `ask` for API key mode) only on first write, not on subsequent syncs.
- **AC-5:** The model selector and mode toggle are visible and functional in the Zed Claude agent panel for all Helix-managed Claude sessions.
