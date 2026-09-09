import { useQuery, useQueryClient } from '@tanstack/react-query'

import type { TypesPromptHistoryEntry } from '../../api/api'
import useApi from '../../hooks/useApi'
import { listSessionPromptHistory } from '../../services/promptHistoryService'
import { selectVisibleQueuedPrompts } from '../../utils/promptQueueVisibility'

// The prompt states a plain (non spec-task) session surfaces above its
// composer. "sending" is deliberately absent: a prompt the agent has already
// been handed is the turn on screen, not something still waiting — the
// spec-task queue hides it for the same reason.
const QUEUED_STATUSES = new Set(['pending', 'failed'])

export const sessionPromptQueueKey = (sessionId: string) => ['session-prompt-queue', sessionId] as const

// Session-keyed, DB-backed queue for sessions without a spec task (org agents,
// project chats): prompts still waiting for the agent plus failed ones,
// including prompts the org graph enqueued itself.
export const useSessionPromptQueue = (sessionId: string, isAgentBusy: boolean) => {
  const api = useApi()
  const apiClient = api.getApiClient()
  const queryClient = useQueryClient()

  const { data } = useQuery({
    queryKey: sessionPromptQueueKey(sessionId),
    enabled: !!sessionId,
    // Poll while open so queued items appear and clear promptly (the
    // spec-task queue polls at the same cadence).
    refetchInterval: 2000,
    queryFn: async () => listSessionPromptHistory(apiClient, sessionId),
  })

  const queued = (data?.entries || [])
    .filter((entry): entry is TypesPromptHistoryEntry & { id: string } => !!entry.id && !!entry.status && QUEUED_STATUSES.has(entry.status))
    .map((entry) => ({ ...entry, timestamp: entry.created_at ? Date.parse(entry.created_at) : 0 }))
  // Same grace window as the spec-task queue: a lone prompt handed to an idle
  // agent is being delivered, not queued.
  const entries = selectVisibleQueuedPrompts(queued, { isAgentBusy })

  const remove = (entryId: string) => (
    apiClient.v1PromptHistoryDelete(entryId)
      .then(() => queryClient.invalidateQueries({ queryKey: sessionPromptQueueKey(sessionId) }))
  )
  const restartAgent = () => apiClient.v1SessionsRestartAgentCreate(sessionId)

  return { entries, remove, restartAgent }
}

export type SessionPromptQueueEntry = ReturnType<typeof useSessionPromptQueue>['entries'][number]
