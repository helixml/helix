# Requirements: Fix Empty Truncated Response in Comment Bubbles

## Problem Statement

When agent responses in the `InlineCommentBubble` component are truncated to show "last 4 lines", the display can show nothing meaningful if those 4 lines are empty or whitespace-only. The user sees "...showing last 4 lines" but the actual content area appears empty.

## Root Cause

The current code splits by `\n` and takes the last N lines without filtering empty lines. If the agent response ends with blank lines (common in markdown formatting), the truncated view shows only those empty lines.

## User Stories

1. As a user reviewing agent comments, I want the collapsed view to show actual content, not empty lines.

## Acceptance Criteria

- [ ] Truncated view displays the last N lines that contain actual content (non-empty/non-whitespace)
- [ ] The "showing last X lines" count reflects meaningful content lines
- [ ] Full expanded view still shows the complete unmodified response
- [ ] Streaming responses continue to work correctly