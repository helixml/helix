/**
 * Prompt History Service - Backend sync for cross-device prompt history
 */

import {
  TypesPromptHistoryEntry,
  TypesPromptHistoryListResponse,
  TypesPromptHistorySyncResponse,
  Api,
} from '../api/api'

export interface LocalPromptHistoryEntry {
  id: string
  content: string
  status: 'sent' | 'pending' | 'failed'
  timestamp: number
  sessionId?: string
  interrupt?: boolean       // If true, interrupts current conversation
  queuePosition?: number    // Position in queue for ordering
  // Retry tracking
  retryCount?: number       // Number of retry attempts
  nextRetryAt?: number      // Timestamp when retry will happen
  // Library features
  pinned?: boolean          // User pinned this prompt for quick access
  usageCount?: number       // How many times this prompt was reused
  lastUsedAt?: number       // Timestamp when last reused
  tags?: string[]           // User-defined tags
}

/**
 * Sync local prompt history entries to backend
 * Uses union merge - new entries are added, existing ones are skipped
 */
export async function syncPromptHistory(
  apiClient: Api<unknown>['api'],
  projectId: string,
  specTaskId: string,
  entries: LocalPromptHistoryEntry[]
): Promise<TypesPromptHistorySyncResponse> {
  const response = await apiClient.v1PromptHistorySyncCreate({
    project_id: projectId,
    spec_task_id: specTaskId,
    entries: entries.map(e => ({
      id: e.id,
      session_id: e.sessionId,
      content: e.content,
      status: e.status,
      timestamp: e.timestamp,
      interrupt: e.interrupt,
      queue_position: e.queuePosition,
    })),
  })
  return response.data
}

/**
 * List prompt history entries from backend
 * Used for initial sync when loading a spec task
 */
export async function listPromptHistory(
  apiClient: Api<unknown>['api'],
  specTaskId: string,
  options?: {
    projectId?: string
    sessionId?: string
    since?: number
    limit?: number
  }
): Promise<TypesPromptHistoryListResponse> {
  const response = await apiClient.v1PromptHistoryList({
    spec_task_id: specTaskId,
    project_id: options?.projectId,
    session_id: options?.sessionId,
    since: options?.since,
    limit: options?.limit,
  })
  return response.data
}

/**
 * Convert backend entry to local format
 */
export function backendToLocal(entry: TypesPromptHistoryEntry): LocalPromptHistoryEntry {
  return {
    id: entry.id || '',
    content: entry.content || '',
    status: (entry.status as 'sent' | 'pending' | 'failed') || 'sent',
    timestamp: entry.created_at ? new Date(entry.created_at).getTime() : Date.now(),
    sessionId: entry.session_id,
    interrupt: entry.interrupt ?? true,
    queuePosition: entry.queue_position,
    // Retry tracking
    retryCount: entry.retry_count ?? 0,
    nextRetryAt: entry.next_retry_at ? new Date(entry.next_retry_at).getTime() : undefined,
    // Library features
    pinned: entry.pinned ?? false,
    usageCount: entry.usage_count ?? 0,
    lastUsedAt: entry.last_used_at ? new Date(entry.last_used_at).getTime() : undefined,
    tags: entry.tags ? JSON.parse(entry.tags) : [],
  }
}

/**
 * Update prompt pin status
 */
export async function updatePromptPin(
  apiClient: Api<unknown>['api'],
  promptId: string,
  pinned: boolean
): Promise<{ pinned: boolean }> {
  const response = await apiClient.v1PromptHistoryPinUpdate(promptId, { pinned })
  return response.data as { pinned: boolean }
}

/**
 * Update prompt tags
 */
export async function updatePromptTags(
  apiClient: Api<unknown>['api'],
  promptId: string,
  tags: string[]
): Promise<{ tags: string }> {
  const response = await apiClient.v1PromptHistoryTagsUpdate(promptId, { tags: JSON.stringify(tags) })
  return response.data as { tags: string }
}

/**
 * Increment prompt usage count (called when reusing a prompt)
 */
export async function incrementPromptUsage(
  apiClient: Api<unknown>['api'],
  promptId: string
): Promise<{ success: boolean }> {
  const response = await apiClient.v1PromptHistoryUseCreate(promptId)
  return response.data as { success: boolean }
}

/**
 * List pinned prompts for quick access
 */
export async function listPinnedPrompts(
  apiClient: Api<unknown>['api'],
  specTaskId?: string
): Promise<TypesPromptHistoryEntry[]> {
  const response = await apiClient.v1PromptHistoryPinnedList({ spec_task_id: specTaskId })
  return response.data
}

/**
 * Search prompts by content
 */
export async function searchPrompts(
  apiClient: Api<unknown>['api'],
  query: string,
  limit?: number
): Promise<TypesPromptHistoryEntry[]> {
  const response = await apiClient.v1PromptHistorySearchList({ q: query, limit })
  return response.data
}
