# D3 / dispatcher coupling: scale-up oscillation with static load

Author: PS
Date: 2026-06-17
Status: proposed

Live demo on yd.helix.ml today surfaced an architectural bug in the autoscaler. D3 (on-demand scale-up) and D4 (idle scale-down) interact with the dispatcher in a way that causes the system to oscillate when load is at-cap but static. PRs landed today (#2637, #2646) fix the worst tactical symptoms, but the underlying signal that drives D3 is wrong and a real fix requires touching the dispatcher.

## Symptoms observed

With Floor=1, Max=3, HEADROOM_MIN=1, IdleTimeout=5min, MaxSandboxes=2 per Runner:

1. User starts 2 spec tasks. Both land on the only Ready Runner (R1) via the dispatcher's lowest-active sort. R1 reports `active=2, max=2`.
2. ComputeManager's reconcile sees `headroom = 2 - 2 = 0 < HEADROOM_MIN=1` and submits a Work Requirement for R2.
3. R2 boots (~3-5 min). Becomes Ready with `active=0`.
4. No new sessions arrive. R1 still has its 2 sessions. R2 sits idle.
5. After 5 min of idleness, D4 deprovisions R2.
6. Now we are back to 1 Runner with `active=2, max=2`. Headroom is again 0 < 1. D3 fires again. Loop.

End user sees the system continuously spin EC2s up and down, even though nothing has changed.

## Root cause

D3's trigger is "headroom < HEADROOM_MIN", where `headroom = sum(MaxSandboxes) - sum(active_sandboxes)` across Ready+online Runners.

That measures **current at-cap-ness**, not **unsatisfied demand**.

The dispatcher (`FindAvailableSandboxInstance` in `api/pkg/store/store_sandbox.go`) places sessions by `active_sandboxes ASC` with no hard cap at MaxSandboxes. Three independent consequences flow from this:

1. Existing sessions stay on the Runner they were placed on. There is no migration mechanism.
2. New capacity that comes up after a placement has happened cannot serve that prior session - it can only catch the next one.
3. If no next session arrives, the new capacity stays idle and D4 reclaims it.

So D3 fires on a signal (at-cap-ness) that the system has no way to relieve except by waiting for new traffic to arrive. When traffic is static, D3 creates capacity that D4 then destroys, indefinitely.

## What today's PRs do and do not fix

| PR | Fixes |
|---|---|
| #2637 (floor predicate Ready+offline) | Stops a different churn: D3 over-firing when a dead Runner is still being counted as alive. Doesn't address static-load oscillation. |
| #2646 (D3 counts in-flight provisioning) | Stops *over-shooting* across cycles when a single burst arrives: cycle 2 doesn't double-count the deficit that cycle 1 already submitted a Provision for. Reduces the amplitude of the oscillation but doesn't eliminate it: the at-cap signal still re-fires after D4 sheds the unused capacity. |
| #2644 (bash SIGTERM + D4 Force=true) | Lets the D4 deprovision actually terminate workers (instead of leaving CANCELLING WRs forever). Means each oscillation cycle now ends cleanly instead of leaving stuck WRs as debris. Speeds up the churn rather than reducing it. |

The oscillation root cause is upstream of all three. It needs a different signal for D3.

## Options

### Option A: tune HEADROOM_MIN=0 (config-only)

D3 only fires when `headroom < 0`, i.e. when the dispatcher has already over-placed beyond MaxSandboxes. Under the current loose dispatcher, that IS the genuine "real demand exceeded capacity" signal.

| Pros | Cons |
|---|---|
| Zero code change. Stops the oscillation today. | No proactive pre-warming. First session past cap pays the full 3-5 min cold-start latency. |
| Easy to recommend in deployment docs. | Quiet failure mode if operator sets MIN > 0 without realising the implication. |

Recommended **as the deployment default** until a structural fix lands. Already documented as a knob in `config.go`.

### Option B: hard-cap dispatcher + session queue + queue-depth scaling (recommended strategic answer)

Three connected changes:

1. **Dispatcher hard-caps at MaxSandboxes**. `FindAvailableSandboxInstance` returns "no capacity" rather than picking the lowest-active Runner when all Runners are at cap.
2. **Sessions queue**. On no-capacity, the API enqueues the session request (DB row, in-memory ring, or NATS subject). The client gets a "pending, here is your queue ID" response.
3. **D3 fires on queue depth, not at-cap-ness**. Each non-empty cycle, ComputeManager checks queue size. Below threshold X: nothing. At-or-above X: provision new Runner(s).

After scale-up, the new Runner pulls from the queue (compose-manager-style pull, not dispatcher push), removing the "new capacity sits idle" failure mode.

D4 keeps its IdleTimeout-based shed. With queue-depth signalling, D3 only fires when there's actual unsatisfied demand. If the queue is empty, no Runner gets provisioned, so none sit idle, so D4 has nothing to reclaim. No oscillation possible by construction.

| Pros | Cons |
|---|---|
| Architecturally correct. Decouples scale signal from placement state. | Largest change of any option. New endpoint shapes, new error semantics, persistent queue state. |
| Unblocks several other things: cost/quota enforcement, fair scheduling across tenants, graceful overload behavior. | Backward compatibility: existing `/sandboxes/start` synchronous flow has to either become async or have a sync veneer over a polling loop. |
| Solves dispatcher task #5 in the same change. | Migration path complexity for in-flight deployments. |

### Option C: D3 fires on "demand growth rate"

Track `active_sandboxes` over a rolling window. Only fire D3 if active count is monotonically increasing AND projected to exhaust capacity within N reconcile cycles.

| Pros | Cons |
|---|---|
| Smaller scope than B. No new infrastructure. | Hand-tuned window and projection. Brittle thresholds. |
| Stops the static-load oscillation. | Doesn't help with sudden bursts (no history yet). |
| Doesn't expose pending state to users. | Operator can't easily reason about why it did/didn't scale. |

A reasonable interim improvement if B is too big to bite off in one release.

### Option D: longer IdleTimeout (config-only band-aid)

Set IdleTimeout to e.g. 30-60min so D4 holds onto extras long enough that some real demand might use them before reclamation.

| Pros | Cons |
|---|---|
| One env var. | Doesn't solve oscillation, just stretches the period. |
| | Wastes EC2 capacity for the IdleTimeout window if demand never materialises. |
| | Cost-sensitive operators won't accept this. |

Mention for completeness; not a serious candidate.

## Recommendation

**Short term (next release)**: ship A as the documented default. Update the `HELIX_COMPUTE_SCALEUP_HEADROOM_MIN` comment in `config.go` and the operator docs to explain that `0` is the right value for the current dispatcher behaviour and any positive value will oscillate under static load.

**Strategic (next quarter)**: implement B. Estimated scope:

| Component | Change |
|---|---|
| `api/pkg/store/store_sandbox.go` | `FindAvailableSandboxInstance` returns ErrNoCapacity when all Runners at MaxSandboxes |
| New: `api/pkg/sandbox/queue/` | Pending-session queue. Schema: `(id, org_id, project_id, request_payload, created_at, claimed_by_runner, claimed_at)`. Backed by DB to survive API restart. |
| `api/pkg/server/sandboxes_api_handlers.go` | `POST /sandboxes` returns 202 with queue id when capacity unavailable. New `GET /sandboxes/{queue_id}/status` for poll. |
| `api/pkg/hydra/*` (or compose-manager) | Runner-side puller. On boot, register interest in the queue; pull next pending session when slot is free. |
| `api/pkg/sandbox/compute/manager.go` | D3 reads queue depth instead of (or in addition to) headroom. New `HELIX_COMPUTE_QUEUE_DEPTH_TRIGGER` config. |
| Frontend | Show "pending" state on session list while a sandbox is queued. |

Risks: backward compatibility with the existing sync API, queue starvation (one tenant monopolising), poison messages. These are solvable.

## Open questions

1. **Queue persistence**: DB rows give us durability through API restart but cost a read on every reconcile. In-memory is faster but loses queue on restart. NATS is overkill for this size. **Probably DB.**
2. **Queue timeout**: when does a queued session error out vs wait? Probably configurable, default ~5 min.
3. **Per-tenant fairness**: round-robin across orgs? Weighted by quota? Defer for now, queue can be FIFO in v1.
4. **Backward compat**: do existing UI/CLI callers handle 202 with a poll URL? If not, the API can busy-wait internally for some short window before returning "still pending" - works as long as the wait is short enough to fit inside a typical client timeout.
5. **Interaction with the inference router**: inference dispatch uses `inferenceRouter.PickRunner` which is a separate path. Does it also need queuing? Probably yes, for symmetric behavior, but the design is identical so this doc covers it.

## Related

- PR https://github.com/helixml/helix/pull/2637 - floor predicate Ready+offline
- PR https://github.com/helixml/helix/pull/2646 - D3 in-flight capacity counting
- PR https://github.com/helixml/helix/pull/2644 - bash SIGTERM + D4 Force=true
- Task #5 - "Sandbox dispatcher redesign: cap enforcement, atomic claim, GPU memory awareness, hybrid pack/spread" (the existing version of this work item; this doc supersedes / expands it)
- Live demo session 2026-06-17 - first end-to-end reproduction of the oscillation
