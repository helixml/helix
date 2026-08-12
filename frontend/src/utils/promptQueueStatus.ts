/**
 * Shared classification for a failed prompt-queue entry: is it a transient
 * failure that will auto-recover, or a wedged/crashed one the user must
 * Restart out of? This is the single source of truth for the Restart
 * affordance, used by both queue renderers (RobustPromptInput's spec-task
 * queue and SessionPromptQueue's session queue) so they behave identically.
 */

// A transient failure is one we expect to recover from automatically (agent
// booting, prior turn draining, WebSocket reconnecting). Shown as a soft
// "still working" state, not a hard error — the user need not act.
const transientErrorMarkers = [
  'no WebSocket',
  'no external agent WebSocket',
  'became busy',
  'deferring queue prompt',
  'channel full',
  'connection replaced',
  'empty response',
]

// Terminal Claude Agent process death. The backend's MarkPromptAsCrashed pins
// next_retry_at to a far-future sentinel (year 9999) to suppress auto-retry;
// detecting that is robust to the many transport error wordings. The string
// markers are a fast path so Restart can render before the sentinel syncs.
const crashedErrorMarkers = [
  'Claude Agent process exited',
  'Session not found',
  'ede_diagnostic',
  'response channel cancelled',
  'receiver is gone',
]

const ONE_YEAR_MS = 365 * 24 * 60 * 60 * 1000

// A transient failure that has retried this many times is NOT recovering — the
// thread is wedged. Escalate to surface Restart; auto-retry keeps running
// underneath as the manual escape hatch.
const STUCK_TRANSIENT_RETRY_THRESHOLD = 4

export interface PromptQueueStatusInput {
  status?: string
  errorMessage?: string
  // Epoch ms of the scheduled retry, or undefined/0 if none.
  nextRetryAtMs?: number
  retryCount?: number
  // "now" in epoch ms — injectable for tests; defaults to Date.now().
  nowMs?: number
}

export interface PromptQueueStatus {
  isFailed: boolean
  isCrashed: boolean
  isTransientFailure: boolean
  isStuckTransient: boolean
  showRestart: boolean
}

export function classifyPromptQueueEntry(input: PromptQueueStatusInput): PromptQueueStatus {
  const { status, errorMessage, nextRetryAtMs, retryCount } = input
  const now = input.nowMs ?? Date.now()
  const isFailed = status === 'failed'

  const crashedBySentinel = isFailed && !!nextRetryAtMs && nextRetryAtMs > now + ONE_YEAR_MS
  const crashedByMarker =
    isFailed && !!errorMessage && crashedErrorMarkers.some((m) => errorMessage.includes(m))
  const isCrashed = crashedBySentinel || crashedByMarker

  const isTransientFailure =
    !isCrashed && isFailed && !!errorMessage && transientErrorMarkers.some((m) => errorMessage.includes(m))
  const isStuckTransient = isTransientFailure && (retryCount ?? 0) >= STUCK_TRANSIENT_RETRY_THRESHOLD

  return {
    isFailed,
    isCrashed,
    isTransientFailure,
    isStuckTransient,
    showRestart: isCrashed || isStuckTransient,
  }
}
