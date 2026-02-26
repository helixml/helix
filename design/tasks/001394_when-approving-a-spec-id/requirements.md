# Requirements: Include Approval Message in Agent Instruction

## User Story

As a reviewer approving a spec, I want my approval message to be sent to the agent so that it can see any additional context, notes, or encouragement I've included.

## Current Behavior

- When approving a spec, the reviewer can enter a comment (e.g., "Looks great! Focus on error handling")
- This comment is stored in `task.SpecApproval.Comments`
- The agent receives an implementation instruction but **does NOT see the approval comment**

## Expected Behavior

- The approval comment should be included in the instruction sent to the agent
- The agent should see the reviewer's message before starting implementation

## Acceptance Criteria

- [ ] When a reviewer approves with a comment, the agent sees that comment in the approval instruction
- [ ] When a reviewer approves without a comment (or with default "Design approved"), no extra section appears
- [ ] The comment appears clearly labeled (e.g., "Reviewer's note:") so the agent understands the context

## Out of Scope

- Changing the frontend approval flow
- Modifying how rejection/revision comments work (already working correctly)