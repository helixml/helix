# Requirements: Default Follow Mode for Agent

## Overview
Enable "follow mode" by default when using the Zed agent, so the editor automatically follows the agent's cursor/activity during code generation.

## User Stories

### US-1: Auto-follow on new threads
As a user, when I start a new agent thread and send a message, I want the editor to automatically follow the agent's activity so I can watch the changes being made without manually enabling follow mode.

## Acceptance Criteria

### AC-1: Default behavior change
- [ ] New agent threads start with follow mode enabled by default
- [ ] Editor scrolls to and focuses on files the agent is editing
- [ ] Existing toggle button continues to work (can turn off follow mode)

### AC-2: Backwards compatibility
- [ ] Manual toggle still works during generation
- [ ] If user turns off follow mode mid-generation, it stays off
- [ ] Next message restores follow mode (since default is now true)