/**
 * usePromptHistory - A robust hook for managing prompt history and drafts
 *
 * Features:
 * - Auto-save drafts to localStorage on every keystroke (debounced)
 * - Persist sent prompts with timestamps
 * - Navigate history with arrow keys
 * - Recover drafts on page reload
 * - Track pending/failed sends for retry
 * - Backend sync for cross-device history (optional)
 */

import { useState, useEffect, useCallback, useRef } from 'react'
import { Api } from '../api/api'
import {
  syncPromptHistory,
  listPromptHistory,
  backendToLocal,
  updatePromptPin as apiUpdatePromptPin,
  updatePromptTags as apiUpdatePromptTags,
  updatePromptTemplate as apiUpdatePromptTemplate,
  incrementPromptUsage as apiIncrementPromptUsage,
  listPinnedPrompts as apiListPinnedPrompts,
  listPromptTemplates as apiListPromptTemplates,
  searchPrompts as apiSearchPrompts,
} from '../services/promptHistoryService'

const HISTORY_STORAGE_KEY = 'helix_prompt_history'
const DRAFT_STORAGE_KEY = 'helix_prompt_draft'
const LAST_SYNC_KEY = 'helix_prompt_last_sync'
const MAX_HISTORY_SIZE = 100
const SYNC_DEBOUNCE_MS = 5000 // Sync to backend every 5 seconds when there are changes

export interface PromptHistoryEntry {
  id: string
  content: string
  timestamp: number
  sessionId: string
  status: 'sent' | 'pending' | 'failed'
  interrupt?: boolean       // If true, this message interrupts current conversation
  queuePosition?: number    // Position in queue for ordering
  syncedToBackend?: boolean // If true, this entry has been synced to the backend
  // Library features
  pinned?: boolean          // User pinned this prompt for quick access
  usageCount?: number       // How many times this prompt was reused
  lastUsedAt?: number       // Timestamp when last reused
  tags?: string[]           // User-defined tags
  isTemplate?: boolean      // Saved as a reusable template
}

interface PromptDraft {
  content: string
  sessionId: string
  timestamp: number
}

interface UsePromptHistoryOptions {
  sessionId: string
  specTaskId?: string  // Required for backend sync
  projectId?: string   // Required for backend sync
  apiClient?: Api<unknown>['api']  // Required for backend sync
  onHistoryChange?: (history: PromptHistoryEntry[]) => void
}

interface UsePromptHistoryReturn {
  // Current draft
  draft: string
  setDraft: (value: string) => void

  // History
  history: PromptHistoryEntry[]
  historyIndex: number

  // Navigation
  navigateUp: () => boolean  // Returns true if navigation occurred
  navigateDown: () => boolean
  resetNavigation: () => void

  // Actions
  saveToHistory: (content: string, interrupt?: boolean) => PromptHistoryEntry
  markAsSent: (id: string) => void
  markAsFailed: (id: string) => void
  retryFailed: (id: string) => string | null  // Returns content to retry
  updateContent: (id: string, content: string) => void  // Update content of queued message
  updateInterrupt: (id: string, interrupt: boolean) => void  // Toggle interrupt flag
  removeFromQueue: (id: string) => void  // Remove a message from queue
  reorderQueue: (activeId: string, overId: string) => void  // Reorder messages in queue

  // Library features
  pinPrompt: (id: string, pinned: boolean) => Promise<void>  // Pin/unpin a prompt
  setTags: (id: string, tags: string[]) => Promise<void>  // Set tags on a prompt
  setTemplate: (id: string, isTemplate: boolean) => Promise<void>  // Mark as template
  reusePrompt: (id: string) => Promise<string | null>  // Reuse prompt and increment usage
  getPinnedPrompts: () => Promise<PromptHistoryEntry[]>  // List pinned prompts
  getTemplates: () => Promise<PromptHistoryEntry[]>  // List templates
  searchHistory: (query: string, limit?: number) => Promise<PromptHistoryEntry[]>  // Search prompts

  // Pending/failed prompts
  pendingPrompts: PromptHistoryEntry[]
  failedPrompts: PromptHistoryEntry[]

  // Clear
  clearDraft: () => void
  clearHistory: () => void
}

function generateId(): string {
  return `${Date.now()}-${Math.random().toString(36).substr(2, 9)}`
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
      // Only restore drafts less than 24 hours old
      if (Date.now() - draft.timestamp < 24 * 60 * 60 * 1000) {
        return draft.content
      }
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
      timestamp: Date.now(),
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

export function usePromptHistory({
  sessionId,
  specTaskId,
  projectId,
  apiClient,
  onHistoryChange,
}: UsePromptHistoryOptions): UsePromptHistoryReturn {
  // Load initial state from localStorage
  const [history, setHistory] = useState<PromptHistoryEntry[]>(() => loadHistory(specTaskId))
  const [draft, setDraftState] = useState<string>(() => loadDraft(sessionId))
  const [historyIndex, setHistoryIndex] = useState(-1)  // -1 = current draft, 0+ = history

  // Keep track of the original draft when navigating
  const originalDraftRef = useRef<string>('')
  const debounceTimerRef = useRef<NodeJS.Timeout | null>(null)
  const syncTimerRef = useRef<NodeJS.Timeout | null>(null)
  const hasSyncedRef = useRef(false)
  const pendingSyncRef = useRef(false)

  // Filter history for current session (for display purposes)
  const sessionHistory = history.filter(h => h.sessionId === sessionId)
  const pendingPrompts = sessionHistory.filter(h => h.status === 'pending')
  const failedPrompts = sessionHistory.filter(h => h.status === 'failed')

  // Perform union merge with backend entries (entries from backend are marked as synced)
  const mergeWithBackend = useCallback((backendEntries: PromptHistoryEntry[]) => {
    setHistory(prev => {
      // Create a map of existing entries by ID
      const existingIds = new Set(prev.map(e => e.id))
      const backendIds = new Set(backendEntries.map(e => e.id))

      // Mark existing entries that are in backend as synced
      const updatedPrev = prev.map(e =>
        backendIds.has(e.id) ? { ...e, syncedToBackend: true } : e
      )

      // Add any backend entries that don't exist locally (mark as synced)
      const newEntries = backendEntries
        .filter(e => !existingIds.has(e.id))
        .map(e => ({ ...e, syncedToBackend: true }))

      if (newEntries.length === 0) {
        // Still save if we updated sync status
        saveHistory(updatedPrev, specTaskId)
        return updatedPrev
      }

      // Merge and sort by timestamp
      const merged = [...updatedPrev, ...newEntries].sort((a, b) => a.timestamp - b.timestamp)

      // Keep only recent entries
      const trimmed = merged.slice(-MAX_HISTORY_SIZE)
      saveHistory(trimmed, specTaskId)

      console.log(`[PromptHistory] Merged ${newEntries.length} entries from backend`)
      return trimmed
    })
  }, [specTaskId])

  // Sync to backend
  const syncToBackend = useCallback(async () => {
    if (!apiClient || !specTaskId || !projectId) return
    if (!navigator.onLine) return

    try {
      // Get entries to sync (all non-synced entries with sent/pending/failed status)
      const toSync = history.filter(h => !h.syncedToBackend)
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
          const updated = prev.map(h => {
            const backendEntry = backendEntriesMap.get(h.id)
            if (backendEntry) {
              // Merge status from backend - this is critical for queue items to disappear
              // when the backend marks them as 'sent' after processing
              return { ...h, status: backendEntry.status, syncedToBackend: true }
            }
            return h
          })

          // Also merge any new entries from backend
          const existingIds = new Set(updated.map(e => e.id))
          const newEntries = backendEntries.filter(e => !existingIds.has(e.id))

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

  // Debounced draft save
  const setDraft = useCallback((value: string) => {
    setDraftState(value)

    // Reset history navigation when typing
    if (historyIndex !== -1) {
      setHistoryIndex(-1)
    }

    // Debounced save to localStorage
    if (debounceTimerRef.current) {
      clearTimeout(debounceTimerRef.current)
    }
    debounceTimerRef.current = setTimeout(() => {
      saveDraft(sessionId, value)
    }, 300)
  }, [sessionId, historyIndex])

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
    setHistoryIndex(-1)
    originalDraftRef.current = ''
  }, [sessionId])

  // Reload history when specTaskId changes
  useEffect(() => {
    const loaded = loadHistory(specTaskId)
    setHistory(loaded)
    hasSyncedRef.current = false // Allow re-sync for new specTaskId
  }, [specTaskId])

  // Notify on history change
  useEffect(() => {
    onHistoryChange?.(history)
  }, [history, onHistoryChange])

  // Navigate up in history (older prompts)
  const navigateUp = useCallback((): boolean => {
    const sentHistory = sessionHistory.filter(h => h.status === 'sent')
    if (sentHistory.length === 0) return false

    if (historyIndex === -1) {
      // Save current draft before navigating
      originalDraftRef.current = draft
      setHistoryIndex(0)
      setDraftState(sentHistory[sentHistory.length - 1].content)
      return true
    } else if (historyIndex < sentHistory.length - 1) {
      const newIndex = historyIndex + 1
      setHistoryIndex(newIndex)
      setDraftState(sentHistory[sentHistory.length - 1 - newIndex].content)
      return true
    }
    return false
  }, [sessionHistory, historyIndex, draft])

  // Navigate down in history (newer prompts)
  const navigateDown = useCallback((): boolean => {
    if (historyIndex <= 0) {
      if (historyIndex === 0) {
        // Return to original draft
        setHistoryIndex(-1)
        setDraftState(originalDraftRef.current)
        return true
      }
      return false
    }

    const sentHistory = sessionHistory.filter(h => h.status === 'sent')
    const newIndex = historyIndex - 1
    setHistoryIndex(newIndex)
    setDraftState(sentHistory[sentHistory.length - 1 - newIndex].content)
    return true
  }, [sessionHistory, historyIndex])

  // Reset navigation
  const resetNavigation = useCallback(() => {
    setHistoryIndex(-1)
    originalDraftRef.current = ''
  }, [])

  // Save prompt to history (called before sending)
  const saveToHistory = useCallback((content: string, interrupt: boolean = true): PromptHistoryEntry => {
    // Calculate queue position based on existing pending/failed messages
    let queuePosition: number
    setHistory(prev => {
      // Find max queue position of pending/failed messages
      const pendingMessages = prev.filter(h => h.status === 'pending' || h.status === 'failed')
      const maxPos = pendingMessages.reduce((max, h) => Math.max(max, h.queuePosition ?? 0), 0)
      queuePosition = maxPos + 1
      return prev // Just reading, actual update happens below
    })

    const entry: PromptHistoryEntry = {
      id: generateId(),
      content,
      timestamp: Date.now(),
      sessionId,
      status: 'pending',
      interrupt,
      queuePosition: queuePosition!,
    }

    setHistory(prev => {
      // Recalculate position in case of race
      const pendingMessages = prev.filter(h => h.status === 'pending' || h.status === 'failed')
      const maxPos = pendingMessages.reduce((max, h) => Math.max(max, h.queuePosition ?? 0), 0)
      entry.queuePosition = maxPos + 1

      const updated = [...prev, entry]
      saveHistory(updated, specTaskId)
      return updated
    })

    return entry
  }, [sessionId, specTaskId])

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
        h.id === id ? { ...h, content } : h
      )
      saveHistory(updated, specTaskId)
      return updated
    })
  }, [specTaskId])

  // Toggle interrupt flag of a queued message
  const updateInterrupt = useCallback((id: string, interrupt: boolean) => {
    setHistory(prev => {
      const updated = prev.map(h =>
        h.id === id ? { ...h, interrupt } : h
      )
      saveHistory(updated, specTaskId)
      return updated
    })
  }, [specTaskId])

  // Remove a message from queue entirely
  const removeFromQueue = useCallback((id: string) => {
    setHistory(prev => {
      const updated = prev.filter(h => h.id !== id)
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
    setDraftState('')
    clearDraftStorage(sessionId)
    setHistoryIndex(-1)
    originalDraftRef.current = ''
  }, [sessionId])

  // Clear all history
  const clearHistoryStorage = useCallback(() => {
    setHistory([])
    try {
      localStorage.removeItem(getStorageKey(specTaskId))
    } catch (e) {
      console.warn('Failed to clear history:', e)
    }
  }, [specTaskId])

  // Pin/unpin a prompt (library feature)
  const pinPrompt = useCallback(async (id: string, pinned: boolean): Promise<void> => {
    if (!apiClient) {
      console.warn('[PromptHistory] Cannot pin prompt without API client')
      return
    }
    try {
      await apiUpdatePromptPin(apiClient, id, pinned)
      // Update local state
      setHistory(prev => {
        const updated = prev.map(h =>
          h.id === id ? { ...h, pinned } : h
        )
        saveHistory(updated, specTaskId)
        return updated
      })
    } catch (e) {
      console.warn('[PromptHistory] Failed to pin prompt:', e)
    }
  }, [apiClient, specTaskId])

  // Set tags on a prompt (library feature)
  const setTags = useCallback(async (id: string, tags: string[]): Promise<void> => {
    if (!apiClient) {
      console.warn('[PromptHistory] Cannot set tags without API client')
      return
    }
    try {
      await apiUpdatePromptTags(apiClient, id, tags)
      // Update local state
      setHistory(prev => {
        const updated = prev.map(h =>
          h.id === id ? { ...h, tags } : h
        )
        saveHistory(updated, specTaskId)
        return updated
      })
    } catch (e) {
      console.warn('[PromptHistory] Failed to set tags:', e)
    }
  }, [apiClient, specTaskId])

  // Mark prompt as template (library feature)
  const setTemplate = useCallback(async (id: string, isTemplate: boolean): Promise<void> => {
    if (!apiClient) {
      console.warn('[PromptHistory] Cannot set template without API client')
      return
    }
    try {
      await apiUpdatePromptTemplate(apiClient, id, isTemplate)
      // Update local state
      setHistory(prev => {
        const updated = prev.map(h =>
          h.id === id ? { ...h, isTemplate } : h
        )
        saveHistory(updated, specTaskId)
        return updated
      })
    } catch (e) {
      console.warn('[PromptHistory] Failed to set template:', e)
    }
  }, [apiClient, specTaskId])

  // Reuse a prompt (increments usage count, returns content)
  const reusePrompt = useCallback(async (id: string): Promise<string | null> => {
    const entry = history.find(h => h.id === id)
    if (!entry) return null

    if (apiClient) {
      try {
        await apiIncrementPromptUsage(apiClient, id)
        // Update local state
        setHistory(prev => {
          const updated = prev.map(h =>
            h.id === id ? {
              ...h,
              usageCount: (h.usageCount || 0) + 1,
              lastUsedAt: Date.now()
            } : h
          )
          saveHistory(updated, specTaskId)
          return updated
        })
      } catch (e) {
        console.warn('[PromptHistory] Failed to increment usage:', e)
      }
    }

    return entry.content
  }, [apiClient, specTaskId, history])

  // List pinned prompts (library feature)
  const getPinnedPrompts = useCallback(async (): Promise<PromptHistoryEntry[]> => {
    if (!apiClient) {
      // Fall back to local filter
      return history.filter(h => h.pinned)
    }
    try {
      const entries = await apiListPinnedPrompts(apiClient, specTaskId)
      return entries.map(backendToLocal)
    } catch (e) {
      console.warn('[PromptHistory] Failed to get pinned prompts:', e)
      return history.filter(h => h.pinned)
    }
  }, [apiClient, specTaskId, history])

  // List templates (library feature)
  const getTemplates = useCallback(async (): Promise<PromptHistoryEntry[]> => {
    if (!apiClient) {
      // Fall back to local filter
      return history.filter(h => h.isTemplate)
    }
    try {
      const entries = await apiListPromptTemplates(apiClient)
      return entries.map(backendToLocal)
    } catch (e) {
      console.warn('[PromptHistory] Failed to get templates:', e)
      return history.filter(h => h.isTemplate)
    }
  }, [apiClient, history])

  // Search prompts by content (library feature)
  const searchHistory = useCallback(async (query: string, limit?: number): Promise<PromptHistoryEntry[]> => {
    if (!apiClient) {
      // Fall back to local search
      const lowerQuery = query.toLowerCase()
      return history
        .filter(h => h.content.toLowerCase().includes(lowerQuery))
        .slice(0, limit || 50)
    }
    try {
      const entries = await apiSearchPrompts(apiClient, query, limit)
      return entries.map(backendToLocal)
    } catch (e) {
      console.warn('[PromptHistory] Failed to search prompts:', e)
      // Fall back to local search
      const lowerQuery = query.toLowerCase()
      return history
        .filter(h => h.content.toLowerCase().includes(lowerQuery))
        .slice(0, limit || 50)
    }
  }, [apiClient, history])

  return {
    draft,
    setDraft,
    history: sessionHistory,
    historyIndex,
    navigateUp,
    navigateDown,
    resetNavigation,
    saveToHistory,
    markAsSent,
    markAsFailed,
    retryFailed,
    updateContent,
    updateInterrupt,
    removeFromQueue,
    reorderQueue,
    // Library features
    pinPrompt,
    setTags,
    setTemplate,
    reusePrompt,
    getPinnedPrompts,
    getTemplates,
    searchHistory,
    // Status
    pendingPrompts,
    failedPrompts,
    clearDraft,
    clearHistory: clearHistoryStorage,
  }
}
