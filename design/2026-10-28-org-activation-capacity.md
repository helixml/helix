# Org activation capacity and Chief of Staff startup

Date: 2026-10-28
Status: fix prepared in a development worktree; production rollout pending

## Incident

The production API used one process-wide semaphore with a limit of 8 for all
organization Bot activations. The limit was held for the complete activation,
including long-running session polling. At the time of the Birding Live
incident, the goroutine dump showed all 8 slots occupied. A later
`POST /api/v1/orgs/birding-live/bots/chief-of-staff/activate` returned 202, but
the activation waited before startup or transcript work began. Later clicks
returned the same unfinished activation record, so they did not enqueue a new
run.

The affected activation was `a-a01100bb-ba41-42bb-b086-f95fd22a286c`, started
at 2026-10-28 11:11:33 UTC. The unfinished row had no error because admission
was waiting before the activation audit completion path could run.

Three other production sessions had exited desktops while their interactions
still reported `waiting` with partial response data:

- `ses_01m5zr8qyhge9vpsndc6r4spmf` / `int_01m5zybgjhp3h5jf18evjbdcfy`: 181738 response characters, 69 entries
- `ses_01m5yrzranzz6bsjghg99tzp7s` / `int_01m5zyt0ve2wz99ndqmct4s3mp`: 110624 response characters, 24 entries
- `ses_01m422wytmac5gzx9eatvf61m0` / `int_01m5zz5capxj04a6gb1ssyh6bd`: 132846 response characters, 42 entries

The existing auto-wake scan only selects old waiting interactions with an
empty response and no response entries. These partial interactions are
therefore outside that recovery path. This remains a separate lifecycle
follow-up; changing the poll guard here could interrupt healthy long-running
turns.

## Cause

The value 8 was not derived from runner capacity. It was preserved from the
old cached spawner when the runtime became multi-tenant. The shared semaphore
was retained in the multitenancy fix as a process-wide cap. Durable delivery
invokes the spawner with a background context, so a full semaphore could wait
indefinitely and emit no user-facing error.

## Fix

The prepared fix replaces the global semaphore with a shared limiter keyed by
organization. The default is 32 concurrent activations per organization and
the runner's own admission limits remain authoritative. Organizations can
override the default through the existing settings registry key
`worker.max_concurrent_activations`.

When an organization reaches its limit, the activation records a clear error,
logs the organization, bot, active count, and limit, and releases the durable
delivery according to trigger type. Manual and hire activations are terminal
failures that the user can retry; automatic event activations retain durable
retry behavior.

The Bot API now exposes the latest completed activation error. The bot loading
screen and active workspace show that message with a retry action, and the
session start control reports API failures through the existing snackbar.

## Verification and limits

Focused backend and race tests cover limiter behavior, configuration
validation, activation completion, and the Bot status response. The six-file
frontend test run passed 53 tests, including the canonical resolver used by
chart and sidebar Start actions, persisted errors in the detail/chat surfaces,
and the session lifecycle snackbar. Full frontend typecheck remains blocked by
the pre-existing `WorkspaceReviewMessage.ts` /
`workspaceReviewMessage.ts` filename case collision. The production
deployment has not been performed. The default of 32 is a raised application
guard, not a measured promise that two GPUs can sustain 32 simultaneous
sessions. Existing partial-response interactions and their stopped desktops
still need a separate lifecycle investigation.
