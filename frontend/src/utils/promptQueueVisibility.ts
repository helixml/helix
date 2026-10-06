/**
 * When is the composer's queue panel worth showing?
 *
 * Every prompt starts life as a local 'pending' entry, including the common
 * case where the agent is idle and the backend dispatches it milliseconds
 * later. Rendering the panel for that entry told the user their message was
 * "1 queued" when nothing was queued at all — it was already on its way, and
 * the panel only vanished once the next status poll landed.
 *
 * A prompt is genuinely queued when it has to WAIT: the agent is mid-turn, it
 * is stuck behind another queued prompt, delivery failed, or dispatch simply
 * hasn't happened within the grace window below. Anything else is in-flight
 * delivery, which the transcript itself will show.
 */

// How long a lone pending prompt may sit undispatched before we admit it is
// queued. Sized above the post-submit status refresh (usePromptHistory bursts
// a backend refresh at ~400ms/~1200ms after a new entry syncs), so an idle
// agent's prompt flips to 'sending' — and drops out of the queue entirely —
// before this expires.
export const QUEUE_DISPATCH_GRACE_MS = 2000

export interface QueueVisibilityEntry {
  status?: string
  timestamp: number
}

export interface QueueVisibilityOptions {
  // The agent is mid-turn, so anything submitted now must wait for it.
  isAgentBusy?: boolean
  // "now" in epoch ms — injectable for tests; defaults to Date.now().
  nowMs?: number
}

function isQueueSurfaced(entries: readonly QueueVisibilityEntry[], options: QueueVisibilityOptions): boolean {
  if (entries.length === 0) return false
  // A backlog is by definition a queue: only one prompt can be in flight.
  if (entries.length > 1) return true
  if (options.isAgentBusy) return true
  const only = entries[0]
  // Failed/retrying needs the user's attention (and its Restart affordance).
  if (only.status === 'failed') return true
  const now = options.nowMs ?? Date.now()
  return now - only.timestamp >= QUEUE_DISPATCH_GRACE_MS
}

/**
 * The queue entries to render — the full set, or none at all when the single
 * entry present is still inside its dispatch grace window.
 */
export function selectVisibleQueuedPrompts<T extends QueueVisibilityEntry>(
  entries: readonly T[],
  options: QueueVisibilityOptions = {},
): T[] {
  return isQueueSurfaced(entries, options) ? [...entries] : []
}

/**
 * Epoch ms at which a currently-hidden queue becomes visible, or null when
 * there is nothing waiting on the clock (already visible, empty, or destined
 * to change by a status update rather than by time). Callers use this to
 * schedule the one re-render that reveals a prompt whose dispatch never came.
 */
export function nextQueueVisibilityDeadline(
  entries: readonly QueueVisibilityEntry[],
  options: QueueVisibilityOptions = {},
): number | null {
  if (isQueueSurfaced(entries, options)) return null
  if (entries.length !== 1) return null
  return entries[0].timestamp + QUEUE_DISPATCH_GRACE_MS
}
