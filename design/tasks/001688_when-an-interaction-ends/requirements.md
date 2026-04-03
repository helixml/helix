# Requirements: Skip "Agent Finished" Notification When User Already Replied

## User Story

As a user actively chatting in the Helix UI, when I send a new message while (or immediately after) the agent is finishing its previous response, I should NOT receive an "Agent finished working" notification — because I'm clearly already looking at the interface.

## Problem

When an interaction completes, the system emits an `agent_interaction_completed` attention event, which shows a bell badge and optionally a browser notification. If the user immediately sent a follow-up message, this notification is noise: the user is demonstrably active and watching the session.

## Acceptance Criteria

1. When `message_completed` fires for interaction N, and the session already has a newer interaction (interaction N+1 with `state=waiting`, created by a user message in the same session), the `agent_interaction_completed` attention event is NOT emitted.
2. When `message_completed` fires and there is NO newer user interaction in the same session, the notification IS emitted as before (no regression).
3. The check is done server-side in `handleMessageCompleted`, before calling `attentionService.EmitEvent`.
4. Existing attention events (Slack notifications, browser notifications) continue to work for all other cases.
