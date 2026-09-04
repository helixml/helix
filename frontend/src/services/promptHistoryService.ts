/**
 * Durable prompt delivery queue - backend sync for cross-device recovery
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
  status: 'sent' | 'sending' | 'pending' | 'failed'
  timestamp: number
  sessionId?: string
  userId?: string           // Who queued it; absent on entries created locally before sync
  interrupt?: boolean       // If true, interrupts current conversation
  queuePosition?: number    // Position in queue for ordering
  // Retry tracking
  retryCount?: number       // Number of retry attempts
  nextRetryAt?: number      // Timestamp when retry will happen
  errorMessage?: string     // Last failure reason (server error string), shown under "Failed - retrying"
}

/**
 * Sync local prompt queue entries to backend
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
 * List prompt queue entries from backend
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
 * List the prompt queue for a session (no spec task) — used by the org-chat /
 * bot-session queue view to show what's queued for the agent.
 */
export async function listSessionPromptHistory(
  apiClient: Api<unknown>['api'],
  sessionId: string,
): Promise<TypesPromptHistoryListResponse> {
  const response = await apiClient.v1PromptHistoryList({ session_id: sessionId })
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
    // Who queued this. The queue belongs to the agent, so it carries entries from
    // teammates and bots too; the UI attributes them and keeps them read-only.
    userId: entry.user_id,
    interrupt: entry.interrupt ?? true,
    queuePosition: entry.queue_position,
    // Retry tracking
    retryCount: entry.retry_count ?? 0,
    nextRetryAt: entry.next_retry_at ? new Date(entry.next_retry_at).getTime() : undefined,
    errorMessage: entry.error_message,
  }
}
