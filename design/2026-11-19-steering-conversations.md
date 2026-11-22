# Steering conversations

While an agent is mid-turn, a message can now be delivered three ways:

| Key (composer, agent busy) | Mode | What happens |
|---|---|---|
| Enter | **steer** | Delivered into the running turn at the agent's next step. Nothing is cancelled. |
| Tab | queue | Waits until the turn finishes. |
| ⌘/Ctrl+Enter | interrupt | Cancels the turn, then sends. |

When idle, Enter just sends. A hint next to the composer says what Enter will do.

## Who can steer

Zed reports `agent_capabilities {acp_thread_id, steering_supported}` per thread when it
subscribes to the thread, and again after every `agent_ready` for that thread so a
reconnected Helix relearns it. Helix stores it as `session.config.agent_steering_supported`.

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

A steer saved before the capability is known waits like a queued prompt. When
`agent_capabilities` reports steering, the handler re-runs the session's queue so that
steer is delivered straight away. The frontend reads the flag from the session query
the spec-task page already polls, and refetches it when a new turn starts.

## Turn split (Zed)

Helix creates the steer's interaction at dispatch. When the injection succeeds Zed
pushes the steered user entry (external-originated, so not echoed) and the thread
subscription splits the turn at it: entries before it are flushed and
`message_completed` is sent for the steered-from request; later output streams under
the steer's request_id, and the agent's final stop completes it. Helix therefore sees
two ordinary turns.

The steer task is one serial queue per Zed process, so the `_session/steering` call is
bounded at 10s. Past that the message goes through the follow-up path, which has its own
silence watchdog. If the agent injects the message after Zed gave up, the agent gets it
twice; that is preferred to a wedged steer queue or a lost message. Pending splits are
keyed by entry index, so two steers into one turn each split it. Closing a thread drops
its splits and pending native steers, and releases its steered-turn guards.

An agent may answer `injected` for a turn that has already stopped (it should answer
`promptRequired`). Zed re-checks that the thread is still generating before splitting; if
not, it delivers the message as a follow-up, like `promptRequired`.

Known edge: if steering lands before the turn produced any output, Zed falls back to a
follow-up (the steered entry would merge into the turn's own user message).

Tests: Go `TestPromptHistoryHandlersSuite` (steer rules, capability handler), frontend
`RobustPromptInput.test.tsx` (keys + hint), Zed
`steer_splits_the_running_turn_on_the_wire` (the split through the real subscription), Zed E2E Phase 18 (the scripted ACP agent
implements `_session/steering`, so the split is asserted exactly).

## In the chat

`interactions.steered` is set when a steer prompt is dispatched while another turn is
still running (an idle steer is an ordinary turn). The chat shows a quiet
"↳ Steered" note above that prompt. A turn that is still open but no longer the
newest shows a one-line note instead of the old "⚠ Incomplete interaction" alert:
"Still working — your next message reaches the agent at its next step" when the next
turn is a steer, otherwise "Still finishing this turn". Interrupted turns get the same
quiet treatment ("Interrupted") instead of an alert box.
