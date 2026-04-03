# Requirements: Interrupt Message Actually Interrupts Zed

## Problem

When a user has a message in the Helix queue and clicks the interrupt button to convert it to an interrupt message, Helix creates a new interaction and sends the message to Zed — but does **not** cancel Zed's current in-progress work. Zed finishes its current turn, then processes the interrupt message. This causes Zed and Helix to diverge on session state.

## User Stories

**US-1**: As a user, when I promote a queued message to interrupt, I want Zed to immediately stop its current work and start on my new message.

**US-2**: As a user, I want to see consistent state between Helix and Zed — Helix should not show a new interaction as started while Zed is still responding to the old one.

## Acceptance Criteria

- AC-1: When a message is promoted from queue to interrupt (or sent directly as interrupt), Helix sends a cancellation signal to Zed over the WebSocket before sending the new `chat_message` command.
- AC-2: Zed, upon receiving the cancellation signal, stops its current ACP task/agent turn and emits an appropriate completion event so Helix can close out the interrupted interaction.
- AC-3: The interrupted interaction in Helix is marked with a terminal state (e.g. `interrupted` or `error`) rather than left in `waiting`.
- AC-4: The new interrupt message arrives in Zed only after Zed has acknowledged (or completed) the cancellation.
- AC-5: If Zed is idle (no active turn), receiving the cancellation is a no-op.
