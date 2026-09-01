/**
 * usePromptHistory - A robust hook for managing prompt history and drafts
 *
 * Features:
 * - Auto-save drafts to localStorage on every keystroke (debounced)
 * - Recover drafts on page reload
 * - Track pending/failed sends for retry
 * - Backend sync for cross-device history (optional)
 */

import { useState, useEffect, useCallback, useRef } from 'react'
import { Api } from '../api/api'
import { createRandomId } from '../utils/randomId'
import {
  syncPromptHistory,
  listPromptHistory,
  backendToLocal,
} from '../services/promptHistoryService'

const HISTORY_STORAGE_KEY = 'helix_prompt_history'
const DRAFT_STORAGE_KEY = 'helix_prompt_draft'
const LAST_SYNC_KEY = 'helix_prompt_last_sync'
const MAX_HISTORY_SIZE = 100
const SYNC_DEBOUNCE_MS = 100 // Debounce for batching rapid changes (100ms feels instant)
// Status refreshes fired after a new prompt syncs, catching the backend's
// immediate dispatch well inside the composer's queue-visibility grace window.
const POST_SYNC_REFRESH_DELAYS_MS = [400, 1200]

export interface PromptHistoryEntry {
  id: string
  content: string
  timestamp: number
  sessionId?: string
  // Who queued this entry. The queue is the AGENT's, so it also carries prompts
  // queued by teammates and by bots via the session-messages API; those are
  // displayed with an owner indicator and are read-only to everyone else.
  // Undefined on a local entry that has not round-tripped through the backend yet.
  userId?: string
  // 'pending'  → in queue, not yet dispatched to Zed
  // 'sending'  → backend has dispatched to Zed but Zed hasn't started streaming yet
  //              (the queue UI keeps these visible — that's the user-visible "in flight" state)
  // 'sent'     → Zed has actually started processing (first message_added received)
  // 'failed'   → dispatch failed or Zed bounced — eligible for retry
  status: 'sent' | 'sending' | 'pending' | 'failed'
  interrupt?: boolean       // If true, this message interrupts current conversation
  queuePosition?: number    // Position in queue for ordering
  syncedToBackend?: boolean // If true, this entry has been synced to the backend
  deleted?: boolean          // Tombstone: locally deleted, prevents re-import from backend
  // Retry tracking
  retryCount?: number       // Number of retry attempts
  nextRetryAt?: number      // Timestamp when retry will happen
  errorMessage?: string     // Last failure reason from server (shown under "Failed - retrying")
}

interface PromptDraft {
  content: string
  sessionId: string
}

interface UsePromptHistoryOptions {
  sessionId: string
  specTaskId?: string  // Required for backend sync
  projectId?: string   // Required for backend sync
  apiClient?: Api<unknown>['api']  // Required for backend sync
}

interface UsePromptHistoryReturn {
  // Current draft
  draft: string
  setDraft: (value: string) => void

  // Actions
  saveToHistory: (content: string, interrupt?: boolean) => PromptHistoryEntry
  markAsSent: (id: string) => void
  markAsFailed: (id: string) => void
  retryFailed: (id: string) => string | null  // Returns content to retry
  updateContent: (id: string, content: string) => void  // Update content of queued message
  updateInterrupt: (id: string, interrupt: boolean) => void  // Toggle interrupt flag
  removeFromQueue: (id: string) => void  // Remove a message from queue
  reorderQueue: (activeId: string, overId: string) => void  // Reorder messages in queue

  // Pending/failed prompts
  pendingPrompts: PromptHistoryEntry[]
  failedPrompts: PromptHistoryEntry[]

  // Clear
  clearDraft: () => void
}

function getStorageKey(specTaskId?: string): string {
  if (specTaskId) {
    return `${HISTORY_STORAGE_KEY}_${specTaskId}`
  }
  return HISTORY_STORAGE_KEY
}

function loadHistory(specTaskId?: string): PromptHistoryEntry[] {
  try {
    const stored = localStorage.getItem(getStorageKey(specTaskId))
    if (stored) {
      return JSON.parse(stored)
    }
  } catch (e) {
    console.warn('Failed to load prompt history:', e)
  }
  return []
}

function saveHistory(history: PromptHistoryEntry[], specTaskId?: string): void {
  try {
    // Keep only the most recent entries
    const trimmed = history.slice(-MAX_HISTORY_SIZE)
    localStorage.setItem(getStorageKey(specTaskId), JSON.stringify(trimmed))
  } catch (e) {
    console.warn('Failed to save prompt history:', e)
  }
}

function loadDraft(sessionId: string): string {
  try {
    const stored = localStorage.getItem(`${DRAFT_STORAGE_KEY}_${sessionId}`)
    if (stored) {
      const draft: PromptDraft = JSON.parse(stored)
      return draft.content
    }
  } catch (e) {
    console.warn('Failed to load draft:', e)
  }
  return ''
}

function saveDraft(sessionId: string, content: string): void {
  try {
    const draft: PromptDraft = {
      content,
      sessionId,
    }
    localStorage.setItem(`${DRAFT_STORAGE_KEY}_${sessionId}`, JSON.stringify(draft))
  } catch (e) {
    console.warn('Failed to save draft:', e)
  }
}

function clearDraftStorage(sessionId: string): void {
  try {
    localStorage.removeItem(`${DRAFT_STORAGE_KEY}_${sessionId}`)
  } catch (e) {
    console.warn('Failed to clear draft:', e)
  }
}

// reconcileEntry is the SINGLE source of truth for merging a backend view of a
// prompt into its local copy. It encodes one invariant that, when scattered
// across the merge/poll/push sites, broke twice and queued interrupts that the
// UI showed as live:
//
//   - Backend-owned fields (status, retry, error) always reflect the backend —
//     it is authoritative for them.
//   - The dirty flag (syncedToBackend) is cleared (true) ONLY by a successful
//     push of THIS entry (pushed=true). A pull (pushed=false) must PRESERVE a
//     pending local edit: if the entry is dirty (syncedToBackend === false) it
//     stays dirty, so the next push actually sends it. Clearing it on a pull —
//     based on mere backend presence — silently drops the user's un-pushed change
//     (e.g. promoting a queued prompt to interrupt).
//
// All reconciliation sites route through here so the invariant cannot diverge.
// See design/2026-06-19-incident-interrupt-during-boot-context-loss.md.
export function reconcileEntry(
  local: PromptHistoryEntry,
  backend: PromptHistoryEntry,
  pushed: boolean,
): PromptHistoryEntry {
  return {
    ...local,
    status: backend.status,
    retryCount: backend.retryCount,
    nextRetryAt: backend.nextRetryAt,
    errorMessage: backend.errorMessage,
    syncedToBackend: pushed ? true : local.syncedToBackend === false ? false : true,
  }
}

export function usePromptHistory({
  sessionId,
  specTaskId,
  projectId,
  apiClient,
}: UsePromptHistoryOptions): UsePromptHistoryReturn {
  // Load initial state from localStorage
  const [history, setHistory] = useState<PromptHistoryEntry[]>(() => loadHistory(specTaskId))
  // Mirror of the latest history for synchronous reads in event handlers (e.g.
  // updateInterrupt's immediate push) without depending on a stale closure.
  const historyRef = useRef<PromptHistoryEntry[]>(history)
  historyRef.current = history
  const [draft, setDraftState] = useState<string>(() => loadDraft(sessionId))
  const debounceTimerRef = useRef<NodeJS.Timeout | null>(null)
  const syncTimerRef = useRef<NodeJS.Timeout | null>(null)
  const hasSyncedRef = useRef(false)
  const pendingSyncRef = useRef(false)
  const burstRefreshTimersRef = useRef<NodeJS.Timeout[]>([])

  // Filter history for current session (for display purposes), excluding tombstoned entries
  const sessionHistory = history.filter(h => h.sessionId === sessionId && !h.deleted)
  // 'sending' is grouped with 'pending' so dispatched-but-not-yet-acknowledged
  // prompts stay visible in the queue UI until Zed actually starts streaming.
  const pendingPrompts = sessionHistory.filter(h => h.status === 'pending' || h.status === 'sending')
  const failedPrompts = sessionHistory.filter(h => h.status === 'failed')

  // Perform union merge with backend entries (entries from backend are marked as synced)
  const mergeWithBackend = useCallback((backendEntries: PromptHistoryEntry[]) => {
    setHistory(prev => {
      // Create a map of existing entries by ID
      const existingIds = new Set(prev.map(e => e.id))
      const backendIds = new Set(backendEntries.map(e => e.id))
      // IDs that are locally tombstoned — never re-import these from backend
      const deletedIds = new Set(prev.filter(e => e.deleted).map(e => e.id))

      // Reconcile existing local entries against the backend view. This is a
      // pull (pushed=false), so reconcileEntry preserves any un-pushed local edit.
      const backendMap = new Map(backendEntries.map(e => [e.id, e]))
      const updatedPrev = prev.map(e => {
        if (e.deleted) return e
        const backendEntry = backendMap.get(e.id)
        return backendEntry ? reconcileEntry(e, backendEntry, false) : e
      })

      // Add any backend entries that don't exist locally (mark as synced)
      // Also skip entries whose ID is locally tombstoned
      const newEntries = backendEntries
        .filter(e => !existingIds.has(e.id) && !deletedIds.has(e.id))
        .map(e => ({ ...e, syncedToBackend: true }))

      // Clean up tombstones: if a deleted entry is no longer in the backend,
      // the backend has confirmed the deletion — we can remove the tombstone.
      const cleanedPrev = updatedPrev.filter(e =>
        !e.deleted || backendIds.has(e.id)
      )

      if (newEntries.length === 0) {
        // Still save if we updated sync status or cleaned tombstones
        saveHistory(cleanedPrev, specTaskId)
        return cleanedPrev
      }

      // Merge and sort by timestamp
      const merged = [...cleanedPrev, ...newEntries].sort((a, b) => a.timestamp - b.timestamp)

      // Keep only recent entries
      const trimmed = merged.slice(-MAX_HISTORY_SIZE)
      saveHistory(trimmed, specTaskId)

      console.log(`[PromptHistory] Merged ${newEntries.length} entries from backend`)
      return trimmed
    })
  }, [specTaskId])

  // Pull backend-owned status for the queue and reconcile it into local state.
  // Shared by the periodic poll below and the burst refresh fired right after a
  // new prompt syncs.
  const refreshStatusesFromBackend = useCallback(async () => {
    if (!apiClient || !specTaskId || !projectId) return
    if (!navigator.onLine) return

    try {
      const response = await listPromptHistory(apiClient, specTaskId, { projectId })

      if (response.entries && response.entries.length > 0) {
        const backendEntries = response.entries.map(backendToLocal)
        // Create a map for quick lookup
        const backendEntriesMap = new Map(backendEntries.map(e => [e.id, e]))

        // Update status of pending messages from backend
        setHistory(prev => {
          let updated = false
          const newHistory = prev.map(h => {
            if (h.deleted) return h // Don't update tombstoned entries
            const backendEntry = backendEntriesMap.get(h.id)
            // Check if status or retry info changed (covers "errored on backend":
            // the backend marks it failed/crashed and we reflect that here).
            // This poll is a pull: reflect backend-owned status but preserve any
            // un-pushed local edit (reconcileEntry, pushed=false). Keep the
            // changed-check so we don't churn state when nothing moved.
            if (backendEntry && (
              h.status !== backendEntry.status ||
              h.retryCount !== backendEntry.retryCount ||
              h.nextRetryAt !== backendEntry.nextRetryAt ||
              h.errorMessage !== backendEntry.errorMessage
            )) {
              updated = true
              return reconcileEntry(h, backendEntry, false)
            }
            // Reconcile against the source of truth: a queue entry we previously
            // synced to the backend that is no longer in the authoritative list has
            // been removed server-side (deleted/expired) and will never send.
            // Surface it as failed instead of letting it sit as a perpetual
            // "waiting to send", so the user can delete it. Guards:
            //  - syncedToBackend: a freshly-created local entry not yet pushed is
            //    legitimately absent — don't misclassify it.
            //  - pending/sending only: don't touch 'sent'/'failed' rows.
            //  - the `response.entries.length > 0` check above means we never act
            //    on a transient empty response, and the poll passes no limit so the
            //    backend returns the full set (no pagination false-positives).
            if (
              !backendEntry &&
              h.syncedToBackend &&
              (h.status === 'pending' || h.status === 'sending')
            ) {
              updated = true
              return {
                ...h,
                status: 'failed' as const,
                errorMessage: 'This queued message is no longer on the server (it was removed). Delete it to clear.',
              }
            }
            return h
          })

          if (updated) {
            saveHistory(newHistory, specTaskId)
            return newHistory
          }
          return prev
        })
      }
    } catch (e) {
      // Silently ignore polling errors - not critical
      console.debug('[PromptHistory] Poll failed:', e)
    }
  }, [apiClient, specTaskId, projectId])

  // The backend dispatches a synced prompt from a goroutine kicked off by the
  // sync request itself, so an idle agent takes it almost immediately. Refresh
  // twice, quickly, instead of waiting for the 2s poll: until the status lands
  // the entry looks 'pending', and a prompt that is already on its way must not
  // linger in the composer's queue panel as though it were waiting.
  const scheduleBurstRefresh = useCallback(() => {
    burstRefreshTimersRef.current.forEach(clearTimeout)
    burstRefreshTimersRef.current = POST_SYNC_REFRESH_DELAYS_MS.map(delay =>
      setTimeout(() => { void refreshStatusesFromBackend() }, delay)
    )
  }, [refreshStatusesFromBackend])

  useEffect(() => () => {
    burstRefreshTimersRef.current.forEach(clearTimeout)
    burstRefreshTimersRef.current = []
  }, [])

  // Sync a single entry immediately (for new prompts - no debounce)
  const syncEntryImmediately = useCallback(async (entry: PromptHistoryEntry) => {
    if (!apiClient || !specTaskId || !projectId) return
    if (!navigator.onLine) return

    try {
      console.log(`[PromptHistory] Immediate sync for entry ${entry.id}`)
      const response = await syncPromptHistory(apiClient, projectId, specTaskId, [entry])

      if (response.synced && response.synced > 0) {
        console.log(`[PromptHistory] Immediately synced entry ${entry.id}`)
      }

      // Mark this entry as synced
      setHistory(prev => {
        const updated = prev.map(h =>
          h.id === entry.id ? { ...h, syncedToBackend: true } : h
        )
        saveHistory(updated, specTaskId)
        return updated
      })

      // Watch for the dispatch the sync just triggered server-side.
      scheduleBurstRefresh()
    } catch (e) {
      console.warn('[PromptHistory] Failed immediate sync:', e)
    }
  }, [apiClient, specTaskId, projectId, scheduleBurstRefresh])

  // Sync to backend (debounced - for status updates and edits)
  const syncToBackend = useCallback(async () => {
    if (!apiClient || !specTaskId || !projectId) return
    if (!navigator.onLine) return

    try {
      // Get entries to sync (all non-synced, non-deleted entries)
      const toSync = history.filter(h => !h.syncedToBackend && !h.deleted)
      if (toSync.length === 0) {
        pendingSyncRef.current = false
        return
      }

      const response = await syncPromptHistory(apiClient, projectId, specTaskId, toSync)

      if (response.synced && response.synced > 0) {
        console.log(`[PromptHistory] Synced ${response.synced} entries to backend`)
      }

      // Mark synced entries and merge any from backend
      if (response.entries && response.entries.length > 0) {
        const backendEntries = response.entries.map(backendToLocal)
        // Create a map for quick lookup of backend entries by ID
        const backendEntriesMap = new Map(backendEntries.map(e => [e.id, e]))

        // Merge backend entry status into local entries (especially important for 'sent' status)
        setHistory(prev => {
          const deletedIds = new Set(prev.filter(e => e.deleted).map(e => e.id))
          // The entries in `toSync` are the ones we just pushed — they are now
          // acknowledged, so reconcileEntry clears their dirty flag. Any other
          // entries in the response are a pull and keep their pending local edit.
          const pushedIds = new Set(toSync.map(e => e.id))
          const updated = prev.map(h => {
            if (h.deleted) return h // Don't update tombstoned entries
            const backendEntry = backendEntriesMap.get(h.id)
            return backendEntry ? reconcileEntry(h, backendEntry, pushedIds.has(h.id)) : h
          })

          // Also merge any new entries from backend (skip tombstoned IDs)
          const existingIds = new Set(updated.map(e => e.id))
          const newEntries = backendEntries.filter(e => !existingIds.has(e.id) && !deletedIds.has(e.id))

          if (newEntries.length > 0) {
            const merged = [...updated, ...newEntries.map(e => ({ ...e, syncedToBackend: true }))]
              .sort((a, b) => a.timestamp - b.timestamp)
              .slice(-MAX_HISTORY_SIZE)
            saveHistory(merged, specTaskId)
            return merged
          }

          saveHistory(updated, specTaskId)
          return updated
        })
      }

      pendingSyncRef.current = false
    } catch (e) {
      console.warn('[PromptHistory] Failed to sync to backend:', e)
    }
  }, [apiClient, specTaskId, projectId, history])

  // Initial sync from backend
  useEffect(() => {
    if (!apiClient || !specTaskId || !projectId) return
    if (hasSyncedRef.current) return
    if (!navigator.onLine) return

    hasSyncedRef.current = true

    const fetchBackendHistory = async () => {
      try {
        const response = await listPromptHistory(apiClient, specTaskId, { projectId })

        if (response.entries && response.entries.length > 0) {
          const backendEntries = response.entries.map(backendToLocal)
          mergeWithBackend(backendEntries)
          console.log(`[PromptHistory] Loaded ${backendEntries.length} entries from backend`)
        }
      } catch (e) {
        console.warn('[PromptHistory] Failed to fetch history from backend:', e)
      }
    }

    fetchBackendHistory()
  }, [apiClient, specTaskId, projectId, mergeWithBackend])

  // Schedule sync when history changes
  useEffect(() => {
    if (!apiClient || !specTaskId || !projectId) return

    // Mark that we have changes to sync
    pendingSyncRef.current = true

    // Debounce sync to backend
    if (syncTimerRef.current) {
      clearTimeout(syncTimerRef.current)
    }

    syncTimerRef.current = setTimeout(() => {
      if (pendingSyncRef.current && navigator.onLine) {
        syncToBackend()
      }
    }, SYNC_DEBOUNCE_MS)

    return () => {
      if (syncTimerRef.current) {
        clearTimeout(syncTimerRef.current)
      }
    }
  }, [history, apiClient, specTaskId, projectId, syncToBackend])

  // Sync when coming back online
  useEffect(() => {
    const handleOnline = () => {
      if (pendingSyncRef.current) {
        syncToBackend()
      }
    }

    window.addEventListener('online', handleOnline)
    return () => window.removeEventListener('online', handleOnline)
  }, [syncToBackend])

  // Poll for status updates from backend when there are pending messages
  // This ensures we know when the backend has processed messages and marked them as 'sent'
  useEffect(() => {
    if (!apiClient || !specTaskId || !projectId) return
    if (!navigator.onLine) return

    // Only poll if there are pending/failed messages that need status updates
    const hasPendingMessages = pendingPrompts.length > 0 || failedPrompts.length > 0
    if (!hasPendingMessages) return

    // Poll every 2 seconds while there are pending messages
    const pollInterval = setInterval(() => { void refreshStatusesFromBackend() }, 2000)

    return () => clearInterval(pollInterval)
  }, [apiClient, specTaskId, projectId, pendingPrompts.length, failedPrompts.length, refreshStatusesFromBackend])

  // Debounced draft save
  const setDraft = useCallback((value: string) => {
    setDraftState(value)

    // Debounced save to localStorage
    if (debounceTimerRef.current) {
      clearTimeout(debounceTimerRef.current)
    }
    debounceTimerRef.current = setTimeout(() => {
      saveDraft(sessionId, value)
    }, 300)
  }, [sessionId])

  // Clean up debounce timer
  useEffect(() => {
    return () => {
      if (debounceTimerRef.current) {
        clearTimeout(debounceTimerRef.current)
      }
    }
  }, [])

  // Reload draft when session changes
  useEffect(() => {
    const loaded = loadDraft(sessionId)
    setDraftState(loaded)
  }, [sessionId])

  // Reload history when specTaskId changes
  useEffect(() => {
    const loaded = loadHistory(specTaskId)
    setHistory(loaded)
    hasSyncedRef.current = false // Allow re-sync for new specTaskId
  }, [specTaskId])

  // Save prompt to history (called before sending)
  const saveToHistory = useCallback((content: string, interrupt: boolean = true): PromptHistoryEntry => {
    // Calculate queue position based on existing pending/failed messages
    let queuePosition: number
    setHistory(prev => {
      // Find max queue position of pending/failed messages
      const pendingMessages = prev.filter(h => h.status === 'pending' || h.status === 'sending' || h.status === 'failed')
      const maxPos = pendingMessages.reduce((max, h) => Math.max(max, h.queuePosition ?? 0), 0)
      queuePosition = maxPos + 1
      return prev // Just reading, actual update happens below
    })

    const entry: PromptHistoryEntry = {
      id: createRandomId(),
      content,
      timestamp: Date.now(),
      sessionId,
      status: 'pending',
      interrupt,
      queuePosition: queuePosition!,
    }

    setHistory(prev => {
      // Recalculate position in case of race
      const pendingMessages = prev.filter(h => h.status === 'pending' || h.status === 'sending' || h.status === 'failed')
      const maxPos = pendingMessages.reduce((max, h) => Math.max(max, h.queuePosition ?? 0), 0)
      entry.queuePosition = maxPos + 1

      const updated = [...prev, entry]
      saveHistory(updated, specTaskId)
      return updated
    })

    // IMMEDIATE SYNC: Send to backend right away (no debounce for new prompts)
    syncEntryImmediately(entry)

    return entry
  }, [sessionId, specTaskId, syncEntryImmediately])

  // Mark prompt as successfully sent
  const markAsSent = useCallback((id: string) => {
    setHistory(prev => {
      const updated = prev.map(h =>
        h.id === id ? { ...h, status: 'sent' as const } : h
      )
      saveHistory(updated, specTaskId)
      return updated
    })
  }, [specTaskId])

  // Mark prompt as failed
  const markAsFailed = useCallback((id: string) => {
    setHistory(prev => {
      const updated = prev.map(h =>
        h.id === id ? { ...h, status: 'failed' as const } : h
      )
      saveHistory(updated, specTaskId)
      return updated
    })
  }, [specTaskId])

  // Retry a failed prompt
  const retryFailed = useCallback((id: string): string | null => {
    const entry = history.find(h => h.id === id)
    if (entry && entry.status === 'failed') {
      // Mark as pending again
      setHistory(prev => {
        const updated = prev.map(h =>
          h.id === id ? { ...h, status: 'pending' as const } : h
        )
        saveHistory(updated, specTaskId)
        return updated
      })
      return entry.content
    }
    return null
  }, [history, specTaskId])

  // Update content of a queued message
  const updateContent = useCallback((id: string, content: string) => {
    setHistory(prev => {
      const updated = prev.map(h =>
        h.id === id ? { ...h, content, syncedToBackend: false } : h
      )
      saveHistory(updated, specTaskId)
      return updated
    })
  }, [specTaskId])

  // Toggle interrupt flag of a queued message
  const updateInterrupt = useCallback((id: string, interrupt: boolean) => {
    setHistory(prev => {
      const updated = prev.map(h =>
        h.id === id ? { ...h, interrupt, syncedToBackend: false } : h
      )
      saveHistory(updated, specTaskId)
      return updated
    })

    // Persist the interrupt flag IMMEDIATELY rather than waiting for the
    // SYNC_DEBOUNCE_MS batched sync. Interrupt is an intent-bearing escalation:
    // if the backend drains the queue (current turn completes) before the
    // debounced sync lands, the escalation is silently lost and the prompt is
    // dispatched as a plain queue message — which can also reorder it relative
    // to a sibling. A targeted single-entry push closes that window.
    // See design/2026-06-23-queue-drain-out-of-order-dispatch.md.
    const existing = historyRef.current.find(h => h.id === id)
    if (existing && apiClient && specTaskId && projectId && navigator.onLine) {
      syncPromptHistory(apiClient, projectId, specTaskId, [{ ...existing, interrupt }])
        .then(() => {
          setHistory(prev =>
            prev.map(h => (h.id === id ? { ...h, syncedToBackend: true } : h))
          )
        })
        .catch(e => {
          // Leave syncedToBackend=false so the debounced sync retries.
          console.warn('[PromptHistory] Failed to flush interrupt toggle immediately:', e)
        })
    }
  }, [specTaskId, apiClient, projectId])

  // Remove a message from queue by marking it as deleted (tombstone).
  // The entry stays in localStorage to prevent re-import from backend on sync.
  const removeFromQueue = useCallback((id: string) => {
    setHistory(prev => {
      const updated = prev.map(h =>
        h.id === id ? { ...h, deleted: true, syncedToBackend: true } : h
      )
      saveHistory(updated, specTaskId)
      return updated
    })
  }, [specTaskId])

  // Reorder messages in the queue (for drag and drop)
  const reorderQueue = useCallback((activeId: string, overId: string) => {
    if (activeId === overId) return

    setHistory(prev => {
      const activeIndex = prev.findIndex(h => h.id === activeId)
      const overIndex = prev.findIndex(h => h.id === overId)

      if (activeIndex === -1 || overIndex === -1) return prev

      // Create a new array with the item moved
      const updated = [...prev]
      const [removed] = updated.splice(activeIndex, 1)
      updated.splice(overIndex, 0, removed)

      saveHistory(updated, specTaskId)
      return updated
    })
  }, [specTaskId])

  // Clear current draft
  const clearDraft = useCallback(() => {
    // Cancel any pending debounced save — otherwise it fires after clearDraft
    // and writes the sent content back to localStorage
    if (debounceTimerRef.current) {
      clearTimeout(debounceTimerRef.current)
      debounceTimerRef.current = null
    }
    setDraftState('')
    clearDraftStorage(sessionId)
  }, [sessionId])

  return {
    draft,
    setDraft,
    saveToHistory,
    markAsSent,
    markAsFailed,
    retryFailed,
    updateContent,
    updateInterrupt,
    removeFromQueue,
    reorderQueue,
    // Status
    pendingPrompts,
    failedPrompts,
    clearDraft,
  }
}
