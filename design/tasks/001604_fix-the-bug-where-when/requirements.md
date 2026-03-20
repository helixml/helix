# Requirements: Fix SpectTask Label Persistence Bug

## Problem

When creating a spectask with multiple labels, only one label is saved. All labels selected by the user should be persisted.

## Root Cause

The `AddSpecTaskLabel` store function uses a read-then-write pattern:

1. Read current task from DB (`GetSpecTask`)
2. Append label
3. Write entire task back (`UpdateSpecTask`)

The frontend calls this function **concurrently** for all labels using `Promise.all()`. Under PostgreSQL MVCC, all concurrent requests read the same initial state (empty labels) and each overwrites the previous result. Only the last writer's label survives.

## User Stories

- **As a user**, when I select multiple labels during task creation, all selected labels should appear on the task after creation.

## Acceptance Criteria

- [ ] Creating a spectask with 2+ labels persists all labels, not just one
- [ ] Existing label add/remove behavior is unchanged for single-label operations
- [ ] No race condition when labels are added concurrently
