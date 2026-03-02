# Requirements: Fix Design Review Comment Streaming

## Problem Statement

Agent responses to design review comments stopped streaming to the UI ~1 week ago. Users can still submit comments and the agent responds, but the streaming updates no longer appear in real-time - users only see the response after it's complete.

## User Stories

### US-1: Design reviewer sees streaming responses
**As a** design reviewer leaving comments on a spec document  
**I want to** see the agent's response streaming in real-time  
**So that** I get immediate feedback and can follow the agent's reasoning

### US-2: Non-owner commenter receives updates
**As a** user commenting on someone else's spec task  
**I want to** receive streaming updates for comments I posted  
**So that** I don't have to wait for the full response to appear

## Acceptance Criteria

1. When a comment is submitted, the agent's response streams into the comment bubble in real-time
2. Streaming works for both the spec task owner AND other users who comment
3. The fix does not regress the performance optimizations (patch-based streaming, throttled DB writes)
4. Existing chat streaming functionality continues to work unchanged

## Root Cause (Investigation Notes)

Commit `c29e74bab` (Feb 25) introduced patch-based streaming to reduce WebSocket traffic. This broke comment streaming because:

- `publishInteractionPatchToFrontend` only publishes to the session owner
- The `requestID` (needed to look up the commenter) is not passed through the streaming path
- The `streamingContext` cache doesn't store the `requestID`

The frontend `DesignReviewContent.tsx` listens for `session_update` events, but during streaming only `interaction_patch` events are now sent.