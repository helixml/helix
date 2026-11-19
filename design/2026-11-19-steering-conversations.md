# Steering conversations

While an agent is mid-turn, a message can now be delivered three ways:

| Key (composer, agent busy) | Mode | What happens |
|---|---|---|
| Enter | **steer** | Delivered into the running turn at the agent's next step. Nothing is cancelled. |
| Tab | queue | Waits until the turn finishes. |
| ⌘/Ctrl+Enter | interrupt | Cancels the turn, then sends. |

When idle, Enter just sends. A hint next to the composer says what Enter will do.

## Who can steer

Zed reports `agent_capabilities {acp_thread_id, steering_supported}` per thread; Helix
stores it as `session.config.agent_steering_supported`.

- **Claude Code** (claude-agent-acp): ACP extension `_session/steering`, advertised via
  `InitializeResponse._meta.steering.supported`. Zed sends it with
  `_meta.steering.idleBehavior = "promptRequired"` so an idle session never starts a
  detached turn.
- **Codex** (`@agentclientprotocol/codex-acp` ≥ 2.8): the same `_session/steering`
  contract and capability flag, so it takes the same path as Claude Code. It can also
  answer `outcome: "failed"`; Zed then delivers the message as a follow-up (queue). It
  echoes the steered message as a user chunk only on ACP v2; Zed speaks v1, so the
  entry is not duplicated.
- **Zed native agent**: `Thread::set_end_turn_at_next_boundary` — the turn ends after
  its current tool round, then the steer runs as the next turn. A turn with no tool
  calls therefore behaves like queue.
- **Everything else** (e.g. Qwen Code, Goose, OpenCode unless they adopt the extension): no steering. The composer says "Enter → queue" and the server
  holds a steer prompt exactly like a queued one.

## Server

`prompt_history_entries.steer` (mutually exclusive with `interrupt`). The busy branch
of `processPendingPromptsForSession` dispatches a steer prompt immediately
(`processSteerPrompt`) only when `canSteerMidTurn` (capability + established thread)
and the agent WebSocket is live. `sendQueuedPromptToSession` exempts it from the busy
check on the same terms and forwards `steer` on `chat_message`.

## Turn split (Zed)

Helix creates the steer's interaction at dispatch. When the injection succeeds Zed
pushes the steered user entry (external-originated, so not echoed) and the thread
subscription splits the turn at it: entries before it are flushed and
`message_completed` is sent for the steered-from request; later output streams under
the steer's request_id, and the agent's final stop completes it. Helix therefore sees
two ordinary turns.

Known edge: if steering lands before the turn produced any output, Zed falls back to a
follow-up (the steered entry would merge into the turn's own user message).

Tests: Go `TestPromptHistoryHandlersSuite` (steer rules, capability handler), frontend
`RobustPromptInput.test.tsx` (keys + hint), Zed E2E Phase 18 (the scripted ACP agent
implements `_session/steering`, so the split is asserted exactly).
