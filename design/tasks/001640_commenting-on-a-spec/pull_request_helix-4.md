# Auto-start desktop when sending a message to a stopped session

## Summary

When a user sends a message (spec comment or session chat) and the desktop is stopped, the system now automatically resumes the desktop in parallel with submitting the message. Previously, messages sent while the desktop was stopped would time out after 2 minutes with "Agent did not respond".

## Changes

- `DesignReviewContent.tsx`: imports `useSandboxState` and `GET_SESSION_QUERY_KEY`; polls desktop state via `planningSessionId`; fires `v1SessionsResumeCreate` in parallel (non-blocking) when `sandboxState === "absent"` on comment submit; shows info/error snackbar feedback
- `SpecTaskDetailContent.tsx`: both session chat `onSend` handlers now call `handleStartSession()` (which already shows a snackbar) when `isDesktopPaused` before sending the message
