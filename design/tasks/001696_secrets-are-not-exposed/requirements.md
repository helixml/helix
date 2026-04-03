# Requirements: Secrets Not Exposed to Human Desktop (Bug Fix)

## Problem Statement

The human desktop (exploratory desktop) does NOT have secrets set as environment variables, unlike the spec task desktop. This breaks AI agent functionality because tools like Claude Code cannot authenticate with external APIs.

The spec task desktop correctly receives secrets via `DesktopAgentAPIEnvVars()`, but the human/exploratory desktop is missing this injection.

## User Stories

### US1: Human Desktop Has API Credentials
**As a** user of the exploratory/human desktop
**I want** API keys injected into my environment
**So that** AI agents and MCP tools can make authenticated requests

**Acceptance Criteria:**
- [ ] `USER_API_TOKEN` available in human desktop environment
- [ ] `ANTHROPIC_API_KEY` available in human desktop environment
- [ ] `OPENAI_API_KEY` available in human desktop environment
- [ ] `ZED_HELIX_TOKEN` available in human desktop environment
- [ ] Running `env | grep API` shows the expected keys

### US2: Parity with Spec Task Desktop
**As a** developer
**I want** human desktop and spec task desktop to have identical secret injection
**So that** behavior is consistent across desktop types

**Acceptance Criteria:**
- [ ] Human desktop receives same env vars as spec task desktop
- [ ] Both use `DesktopAgentAPIEnvVars()` or equivalent

## Out of Scope

- Hiding secrets from desktop (they need to be visible for tools to work)
- Additional security hardening of desktop containers
