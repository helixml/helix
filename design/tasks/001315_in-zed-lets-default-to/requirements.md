# Requirements: Default Follow Mode for Agent

## Overview
Enable "follow mode" by default when using the Zed agent, so the editor automatically follows the agent's cursor/activity during code generation.

## User Stories

### US-1: Auto-follow on new threads
As a user, when I start a new agent thread and send a message, I want the editor to automatically follow the agent's activity so I can watch the changes being made without manually enabling follow mode.

### US-2: Configurable behavior
As a user, I want to be able to disable auto-follow via settings if I prefer not to have the editor follow the agent by default.

## Acceptance Criteria

### AC-1: Default behavior change
- [ ] New agent threads start with follow mode enabled by default
- [ ] Editor scrolls to and focuses on files the agent is editing
- [ ] Existing toggle button continues to work (can turn off follow mode)

### AC-2: Settings support
- [ ] Add `auto_follow_agent` setting to agent settings (default: `true`)
- [ ] Setting is documented with description
- [ ] Users can set `"agent": { "auto_follow_agent": false }` to disable

### AC-3: Backwards compatibility
- [ ] Manual toggle still works during generation
- [ ] If user turns off follow mode mid-generation, it stays off
- [ ] Next message respects the setting (restores auto-follow if enabled)