# Embedded question actions

Date: 2026-11-25.

## Problem

The embedded Recruit chat showed `Failed to send the answer to the agent`.
The shared `PendingQuestionCard` posts answers and cancellations to the
interaction question routes. Embed-key authorization had no allowlist entry for
either route, so the middleware returned HTTP 403 before the handler ran.
SaaS request audit evidence matched the screenshot at 14:34:04 UTC.

## Fix

The embed middleware now recognizes only POST requests shaped as
`/api/v1/interactions/{interaction}/questions/{request}/respond` or `cancel`.
It loads the interaction and permits the request only when its session exactly
matches the embed key's bound `SessionID`. Missing records, lookup errors,
empty IDs, extra path segments, other methods and unknown actions fail closed.
Both middleware entry points use this check. Existing non-embed behavior and
the question handler authorization remain unchanged.

## Verification

- `go test ./pkg/server -run 'TestEmbedKey|TestInteractionQuestionHandlerSuite|TestQuestionAnsweredBy|TestQuestionAction_' -count=1` passed.
- Removing the session equality check made four sibling-session cases fail;
  restoring it made the targeted suite pass.
- `git diff --check` passed.

No deployment or browser retest was performed; localhost:8080 was unavailable.
