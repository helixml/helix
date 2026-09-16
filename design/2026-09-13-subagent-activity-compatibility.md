# Subagent activity compatibility

## Problem

The task Agents panel originally consumed only Zed's normalized subagent metadata. Persisted tool calls from other harnesses use different shapes, so completed sessions could appear empty even though the transcript showed subagents.

## Harness payloads

| Harness | Observed payload | Normalization |
| --- | --- | --- |
| OpenCode (Qwen, GLM) | `<task id="…" state="…"><task_result>…` in tool content | Use the task ID, state, and result |
| Codex (legacy) | `spawnAgent` and `closeAgent` tool calls without child metadata | Keep spawns distinct by tool-call ID and settle them when matching closes exist |
| Codex (current) | `receiverThreadIds` in ACP collaboration tool input | Persist the child session ID and mirror child-session activity |
| Claude Code | Native `subagent_spawned` and `subagent_state_update` ACP notifications | Advertise native subagent-session capability and use the existing normalized event path |

## Verification

- Frontend parser and interaction-timeline tests cover structured, legacy Codex, and OpenCode records.
- Production frontend build completes.
- Focused Zed tests cover current Codex collaboration metadata and native capability negotiation for Codex and Claude.
- Final verification uses fresh dev-stack tasks for OpenCode Qwen, OpenCode GLM, Codex Luna, and Claude Code Opus, including reload after completion.
