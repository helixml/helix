# Requirements: Process Worker Activation Triggers One at a Time

## Background

In production, busy Streams overwhelm an AI Worker's context window. A real
example: a GitHub Stream emits an event for every commit, CI action, and issue.
While a Worker is mid-activation, all of those events queue up. Today the
activation Queue **coalesces** the whole backlog into a single follow-up
activation, so the Worker's next run is handed dozens of triggers at once — a
huge prompt that blows the context budget.

The coalescing was originally added to bound *cost* under webhook bursts. But it
trades cost for context: one giant activation instead of many small ones. For
busy streams that trade is now hurting us.

## Decision

Stop coalescing. Drain the per-Worker pending queue **one trigger per
activation**, preserving arrival (FIFO) order and the existing "at most one
in-flight activation per Worker" guarantee.

This is deliberately the simplest fix. Each activation now carries exactly one
trigger, so context per activation is bounded regardless of how busy the Stream
is. The backlog is worked off sequentially.

## User Stories

### Story 1 — Bounded context on busy streams
As an operator running a Worker subscribed to a busy Stream, I want each
activation to handle a single event, so that a backlog never produces an
oversized prompt that exhausts the context window.

**Acceptance criteria:**
- When N triggers are pending for a Worker, the Queue invokes the Spawner N
  times, each with exactly one trigger.
- Triggers are delivered in arrival order (FIFO).
- At most one activation per Worker runs at any time (unchanged invariant).
- Distinct Workers still activate in parallel (unchanged invariant).
- Per-(org, worker) lane isolation is unchanged (no cross-tenant leak).

### Story 2 — No behavioural surprise for the single-event case
As a Worker author, I want the common single-event activation to look exactly as
it does today, so nothing downstream changes for normal traffic.

**Acceptance criteria:**
- A Worker woken by one event receives a prompt identical to today's
  single-trigger prompt (no "N triggers queued" preamble).
- Transcript markers for a single trigger are unchanged.

## Out of Scope

- Rate-limiting, debouncing, or dropping events.
- Summarising or compacting multiple events into one.
- Any change to how events are published, stored, or fanned out.
- Removing the multi-trigger rendering code in `briefing` (kept dormant; safe to
  remove later but not required for this fix).
