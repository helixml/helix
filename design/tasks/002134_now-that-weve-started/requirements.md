# Requirements: Configurable Model Selection for Claude Subscription Mode

## Background

When using the Zed/Claude ACP agent in Claude subscription mode (via zed.dev), the default model is determined by the cloud API response, which currently resolves to Claude Sonnet. Users who frequently do complex work need a way to default to Claude Opus without switching to API key mode (which already has a full model picker).

## User Stories

**US-1: Default to Opus**
As a Claude subscription user, I want the agent to default to Claude Opus so that I get better results for complex tasks without manually switching every session.

**US-2: Configure model in agent settings**
As a Claude subscription user, I want a model dropdown in the agent configuration panel so that I can choose between Opus, Sonnet, and Haiku and have that preference persist.

## Acceptance Criteria

**AC-1 (Default change — minimum viable)**
- When using Zed cloud/Claude subscription mode and no model preference has been saved, the agent defaults to the latest Claude Opus model available from the cloud.

**AC-2 (Model picker — preferred)**
- The agent configuration panel shows a model selector when the active provider is the Zed cloud (subscription) provider.
- The selector lists exactly three options: Claude Opus, Claude Sonnet, Claude Haiku (the latest version of each available from the cloud API).
- Selecting a model persists to user settings under `agent.default_model` and survives restarts.
- The picker is visually similar to the API key mode model selector (provider icon + model name + chevron).

**AC-3 (No regression)**
- Users on API key mode are unaffected; the existing full model picker continues to work.
- Users who have previously saved a `default_model` preference continue to use that preference.

## Out of Scope

- Fancy model switcher / full model browser for subscription mode (keep it to three hardcoded tiers).
- Per-thread model overrides (future work).
- Exposing extended thinking or effort settings via this picker.
