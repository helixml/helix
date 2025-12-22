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
  }
}
