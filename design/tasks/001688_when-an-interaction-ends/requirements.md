# Requirements: Skip "Agent Finished" Notification When User Is Active

## User Story

As a user, when the agent finishes working and I'm clearly already engaged with the session, I should NOT receive an "Agent finished working" notification — it's noise because I'm already looking at it.

## Problem

When an interaction completes, the system emits an `agent_interaction_completed` attention event (bell badge, browser notification, Slack message). This notification is unnecessary when the user is demonstrably active:

1. **Interruption (primary case):** The user sent a new message which caused the agent to stop mid-response. The agent finishes because of the interruption, and a new interaction starts immediately. The user is clearly watching.
2. **Quick follow-up:** The user sent a follow-up message right after the agent finished naturally. Again, the user is clearly present.

In both cases, the session already has a newer `waiting` interaction by the time `message_completed` fires — that's the signal to suppress the notification.

## Acceptance Criteria

1. When `message_completed` fires for interaction N, and the session already has a newer interaction (N+1 with `state=waiting`), the `agent_interaction_completed` attention event is NOT emitted.
2. When `message_completed` fires and there is NO newer user interaction in the same session, the notification IS emitted as before (no regression).
3. The check is done server-side in `handleMessageCompleted`, before calling `attentionService.EmitEvent`.
4. Existing attention events (Slack notifications, browser notifications) continue to work for all other cases.
