# Requirements: Configurable Model Selection for Claude Subscription Mode

## Background

When using Claude Code (`claude_code` runtime) with subscription credentials in ANGER/Helix, no model preference is passed to Claude Code — it falls back to its own built-in default (currently Sonnet). Users doing complex work want to default to Opus, and want the ability to switch between the three main Claude tiers without leaving the Helix agent configuration screen.

## User Stories

**US-1: Default to Opus in subscription mode**
As a Helix user with a Claude subscription, I want the Claude Code agent to use Opus by default so I get better results for complex tasks without changing credentials or switching to API key mode.

**US-2: Choose model tier in agent configuration**
As a Helix user with a Claude subscription, I want a dropdown in the agent configuration that lets me pick Opus, Sonnet, or Haiku, and have that choice persist across sessions.

## Acceptance Criteria

**AC-1 — Default to Opus**
- When a `claude_code` + `subscription` agent has no explicit model configured, the sandbox container receives `ANTHROPIC_MODEL=claude-opus-4-6` so Claude Code uses Opus.

**AC-2 — Model dropdown in agent config UI**
- The `CodingAgentForm` component shows a simple dropdown when `codeAgentRuntime === 'claude_code'` and `claudeCodeMode === 'subscription'`.
- The dropdown lists exactly three options: Claude Opus 4.6, Claude Sonnet 4.5, Claude Haiku 4.5 (hardcoded, not pulled dynamically).
- The selected model is saved on the assistant config and passed to the container via `ANTHROPIC_MODEL` env var.
- If the user saves without selecting a model, Opus is the default.

**AC-3 — No regression for API key mode**
- The existing provider + model picker for `api_key` mode is unaffected.
- Subscription model configuration is a separate field on `AssistantConfig` and does not conflict with `GenerationModel` used in API key mode.

## Out of Scope

- Fancy model browser or per-session model override.
- Extended thinking / effort controls in this dropdown.
- Dynamically fetching model list from the `listClaudeModels` API endpoint (hardcode three tiers for now).
