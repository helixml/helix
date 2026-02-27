# Requirements: Claude Code Permission Bypass Regression Fix

## Problem Statement

Claude Code is prompting for permissions on every tool use despite having a setting to bypass permissions. This regression was introduced in commit `6b8c9c945` which incorrectly renamed a JSON field.

## User Stories

1. **As a Helix user**, I want Claude Code to automatically execute tools without prompting me for permission every time, so I can focus on my work without constant interruptions.

## Root Cause Analysis

Commit `6b8c9c945` ("fix: update Zed config for upstream deprecated settings") made two changes:
1. ✅ **Correct**: Changed `always_allow_tool_actions` to `tool_permissions.default="allow"` (agent-level Zed setting)
2. ❌ **Incorrect**: Changed `default_mode` to `default` for agent_servers.claude config

The second change is wrong. Looking at Zed's `BuiltinAgentServerSettings` struct in `crates/settings_content/src/agent.rs:366`, the field is named `default_mode`, not `default`.

**Before (working)**:
```json
{
  "agent_servers": {
    "claude": {
      "default_mode": "bypassPermissions",
      "env": { ... }
    }
  }
}
```

**After (broken)**:
```json
{
  "agent_servers": {
    "claude": {
      "default": "bypassPermissions",
      "env": { ... }
    }
  }
}
```

Zed ignores the unrecognized `default` field, so Claude Code starts with no default mode set, requiring permission prompts.

## Acceptance Criteria

- [ ] Claude Code launches in `bypassPermissions` mode by default
- [ ] Tool actions execute without permission prompts
- [ ] Existing sessions continue to work after fix
- [ ] New sessions start with correct bypass behavior