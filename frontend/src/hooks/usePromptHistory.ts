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
import { syncPromptHistory, listPromptHistory, backendToLocal } from '../services/promptHistoryService'

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
  saveToHistory: (content: string) => PromptHistoryEntry
  markAsSent: (id: string) => void
  markAsFailed: (id: string) => void
  retryFailed: (id: string) => string | null  // Returns content to retry
  updateContent: (id: string, content: string) => void  // Update content of queued message
  removeFromQueue: (id: string) => void  // Remove a message from queue

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

  // Perform union merge with backend entries
  const mergeWithBackend = useCallback((backendEntries: PromptHistoryEntry[]) => {
    setHistory(prev => {
      // Create a map of existing entries by ID
      const existingIds = new Set(prev.map(e => e.id))

      // Add any backend entries that don't exist locally
      const newEntries = backendEntries.filter(e => !existingIds.has(e.id))

      if (newEntries.length === 0) {
        return prev // No changes needed
      }

      // Merge and sort by timestamp
      const merged = [...prev, ...newEntries].sort((a, b) => a.timestamp - b.timestamp)

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
      // Get sent entries to sync
      const toSync = history.filter(h => h.status === 'sent')
      if (toSync.length === 0) return

      const response = await syncPromptHistory(apiClient, projectId, specTaskId, toSync)

      if (response.synced && response.synced > 0) {
        console.log(`[PromptHistory] Synced ${response.synced} entries to backend`)
      }

      // Merge any entries from backend we don't have locally
      if (response.entries && response.entries.length > 0) {
        const backendEntries = response.entries.map(backendToLocal)
        mergeWithBackend(backendEntries)
      }

      pendingSyncRef.current = false
    } catch (e) {
      console.warn('[PromptHistory] Failed to sync to backend:', e)
    }
  }, [apiClient, specTaskId, projectId, history, mergeWithBackend])

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
  const saveToHistory = useCallback((content: string): PromptHistoryEntry => {
    const entry: PromptHistoryEntry = {
      id: generateId(),
      content,
      timestamp: Date.now(),
      sessionId,
      status: 'pending',
    }

    setHistory(prev => {
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

  // Remove a message from queue entirely
  const removeFromQueue = useCallback((id: string) => {
    setHistory(prev => {
      const updated = prev.filter(h => h.id !== id)
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
    removeFromQueue,
    pendingPrompts,
    failedPrompts,
    clearDraft,
    clearHistory: clearHistoryStorage,
  }
}
