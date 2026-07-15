# Stale Zed thread recovery

Date: 2026-07-15
Status: implemented; live lifecycle verification pending

## Prime evidence

Session `ses_01kxjxj39zx81m2s6knptgzyxj` lost its runtime thread when the
workspace was reaped at 20:08 UTC after inactivity. A fresh container was
created at 20:33:40 UTC, but the database still retained Zed thread ID
`c158d52d-3300-4289-93dd-d6bda758bc6a`.

Helix therefore sent the next turn to a thread that did not exist in the new
NativeAgent runtime. NativeAgent returned its exact no-thread error marker and
interaction `int_01kxkqpn5bm899shcf12r5cdb2` failed. The existing recovery path
recognized only Codex's missing-rollout error, so a native Zed thread loss was
terminal instead of self-healing.

## Root fix

Recovery is limited to authoritative missing-thread markers. It does not use a
broad substring match.

When a recognized stale-thread error arrives:

1. Compare the error's `acp_thread_id` with the session's persisted
   `ZedThreadID`. If they differ, the error belongs to an older request and
   cannot clear the replacement thread.
2. Clear the matching persisted thread metadata and its in-memory context
   mapping.
3. Resolve the same Waiting interaction through the authoritative request
   mapping.
4. Replay that interaction once without `acp_thread_id`, causing NativeAgent to
   create a thread instead of resuming the missing one.
5. Let the existing `thread_created` handler persist the replacement thread ID
   and rebuild its mapping.

Duplicate errors for the old thread are swallowed after recovery has already
cleared or replaced it. They do not fail the replayed interaction or trigger a
second replay. Arbitrary agent errors retain the thread ID and follow the normal
failure path.

This fixes the root lifecycle mismatch: database thread metadata can outlive
the runtime storage that made the thread resumable.

## Verification

- Focused automated tests for native missing-thread recovery passed.
- Tests cover marker recognition, persisted-ID comparison, metadata and mapping
  cleanup, same-interaction replay without `acp_thread_id`, duplicate stale
  error suppression, and preservation on unrelated errors.
- NOT tested: a live connected Zed lifecycle run that creates a thread, reaps
  its workspace, starts a fresh container, sends the next turn, and confirms
  the same Waiting interaction completes on the replacement thread.
