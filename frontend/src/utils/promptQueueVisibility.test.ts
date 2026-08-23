import { describe, expect, it } from 'vitest'
import {
  QUEUE_DISPATCH_GRACE_MS,
  nextQueueVisibilityDeadline,
  selectVisibleQueuedPrompts,
} from './promptQueueVisibility'

const NOW = 1_000_000

const entry = (overrides: Partial<{ id: string; status: string; timestamp: number }> = {}) => ({
  id: overrides.id ?? 'a',
  status: overrides.status ?? 'pending',
  timestamp: overrides.timestamp ?? NOW,
})

describe('selectVisibleQueuedPrompts', () => {
  it('hides a just-submitted prompt while an idle agent is picking it up', () => {
    const entries = [entry({ timestamp: NOW - 200 })]
    expect(selectVisibleQueuedPrompts(entries, { isAgentBusy: false, nowMs: NOW })).toEqual([])
  })

  it('shows it once the grace window passes without a dispatch', () => {
    const entries = [entry({ timestamp: NOW - QUEUE_DISPATCH_GRACE_MS })]
    expect(selectVisibleQueuedPrompts(entries, { isAgentBusy: false, nowMs: NOW })).toHaveLength(1)
  })

  it('shows a prompt sent while the agent is mid-turn immediately', () => {
    const entries = [entry({ timestamp: NOW })]
    expect(selectVisibleQueuedPrompts(entries, { isAgentBusy: true, nowMs: NOW })).toHaveLength(1)
  })

  it('shows a backlog immediately — only one prompt can be in flight', () => {
    const entries = [entry({ id: 'a', timestamp: NOW }), entry({ id: 'b', timestamp: NOW })]
    expect(selectVisibleQueuedPrompts(entries, { isAgentBusy: false, nowMs: NOW })).toHaveLength(2)
  })

  it('shows a failed prompt immediately so its Restart affordance is reachable', () => {
    const entries = [entry({ status: 'failed', timestamp: NOW })]
    expect(selectVisibleQueuedPrompts(entries, { isAgentBusy: false, nowMs: NOW })).toHaveLength(1)
  })

  it('renders nothing for an empty queue', () => {
    expect(selectVisibleQueuedPrompts([], { nowMs: NOW })).toEqual([])
  })
})

describe('nextQueueVisibilityDeadline', () => {
  it('returns when a hidden prompt must be revealed', () => {
    const entries = [entry({ timestamp: NOW - 500 })]
    expect(nextQueueVisibilityDeadline(entries, { isAgentBusy: false, nowMs: NOW })).toBe(
      NOW - 500 + QUEUE_DISPATCH_GRACE_MS,
    )
  })

  it('returns null when the queue is already visible or empty', () => {
    expect(nextQueueVisibilityDeadline([entry()], { isAgentBusy: true, nowMs: NOW })).toBeNull()
    expect(nextQueueVisibilityDeadline([], { nowMs: NOW })).toBeNull()
  })
})
