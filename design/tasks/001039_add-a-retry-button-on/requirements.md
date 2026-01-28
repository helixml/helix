# Requirements: Retry Button for External Agent Errors

## User Stories

1. **As a user**, when sending a prompt to the Zed IDE agent fails, I want to see a retry button so I can attempt to resend my message without retyping it.

2. **As a user**, when the system encounters an error while communicating with Zed, I want clear visual feedback about what went wrong and an easy way to try again.

## Acceptance Criteria

- [ ] When an interaction with an external agent (Zed) enters the error state, a "Retry" button is displayed alongside the error message
- [ ] Clicking the retry button resends the original user prompt to the Zed agent
- [ ] The retry button uses the same styling as existing retry buttons in the codebase (MUI Button with ReplayIcon)
- [ ] The retry functionality works in both the `EmbeddedSessionView` (session panel) and standalone session views
- [ ] Error state is cleared when retry is initiated

## Out of Scope

- Automatic retry with exponential backoff (already exists for connection issues in `DesktopStreamViewer`)
- Retry for non-external-agent (regular Helix) errors (already supported)
- Offline message queuing (handled separately by `RobustPromptInput`)