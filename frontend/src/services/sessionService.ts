import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import useApi from '../hooks/useApi';
import { TypesSession } from '../api/api';
import { QueryClient } from '@tanstack/react-query';

export const SESSION_STEPS_QUERY_KEY = (id: string) => [
  "session-steps",
  id
];

export const GET_SESSION_QUERY_KEY = (id: string) => [
  "session",
  id
];

export const LIST_SESSIONS_QUERY_KEY = (orgId?: string, page?: number, pageSize?: number, search?: string, questionSetExecutionId?: string, projectId?: string, appId?: string) => [
  "sessions",
  orgId,
  page,
  pageSize,
  search,
  questionSetExecutionId,
  projectId,
  appId,
];

export const LIST_INTERACTIONS_QUERY_KEY = (sessionId: string, page?: number, perPage?: number, order?: string) => [
  "interactions",
  sessionId,
  page,
  perPage,
  order,
];

// useListSessionSteps returns the steps for a session, it includes
// steps for all interactions in the session
export function useListSessionSteps(sessionId: string, options?: { enabled?: boolean }) {
  const api = useApi()
  const apiClient = api.getApiClient()  

  return useQuery({
    queryKey: SESSION_STEPS_QUERY_KEY(sessionId),
    queryFn: () => apiClient.v1SessionsStepInfoDetail(sessionId),
    enabled: options?.enabled ?? true
  })
}

export function useGetSession(sessionId: string, options?: { enabled?: boolean; refetchInterval?: number | false }) {
  const api = useApi()
  const apiClient = api.getApiClient()

  return useQuery({
    queryKey: GET_SESSION_QUERY_KEY(sessionId),
    queryFn: () => apiClient.v1SessionsDetail(sessionId),
    enabled: options?.enabled ?? true,
    refetchInterval: options?.refetchInterval,
    // Prevent immediate refetches when multiple consumers share this query.
    // E.g. useSandboxState (3s) and EmbeddedSessionView (5s) both poll the same
    // session — without staleTime, mounting a new consumer would trigger an
    // immediate redundant fetch even if data was fetched <1s ago.
    staleTime: 2000,
  })
}

export function useListSessions(orgId?: string, search?: string, questionSetExecutionId?: string, projectId?: string, page?: number, pageSize?: number, options?: { enabled?: boolean }, appId?: string) {
  const api = useApi()
  const apiClient = api.getApiClient()
  
  return useQuery({
    queryKey: LIST_SESSIONS_QUERY_KEY(orgId, page ?? 0, pageSize ?? 0, search ?? '', questionSetExecutionId ?? '', projectId ?? '', appId ?? ''),
    queryFn: () => apiClient.v1SessionsList({
      org_id: orgId,
      search: search,
      question_set_execution_id: questionSetExecutionId,
      page: page,
      page_size: pageSize,
      project_id: projectId,
      app_id: appId,
    }),
    enabled: options?.enabled ?? true
  })
}

export function useUpdateSession(sessionId: string, options?: { enabled?: boolean }) {
  const api = useApi()
  const apiClient = api.getApiClient()
  const queryClient = useQueryClient()
  
  return useMutation({
    mutationFn: (sessionData: TypesSession) => apiClient.v1SessionsUpdate(sessionId, sessionData),
    onSuccess: (updatedSession) => {
      // Invalidate the specific session query to refresh the session data
      queryClient.invalidateQueries({ queryKey: GET_SESSION_QUERY_KEY(sessionId) })
      // Invalidate all sessions queries to refresh the list after update
      queryClient.invalidateQueries({ queryKey: ["sessions"] })
      // Optionally update the cache directly with the new data
      queryClient.setQueryData(GET_SESSION_QUERY_KEY(sessionId), updatedSession)
    }
  })
}


export function useDeleteSession(sessionId: string, options?: { enabled?: boolean }) {
  const api = useApi()
  const apiClient = api.getApiClient()
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: () => apiClient.v1SessionsDelete(sessionId),
    onSuccess: () => {
      // Invalidate all sessions queries to refresh the list after deletion
      queryClient.invalidateQueries({ queryKey: ["sessions"] })
    }
  })
}

export function useStopExternalAgent(sessionId: string) {
  const api = useApi()
  const apiClient = api.getApiClient()
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: () => apiClient.v1SessionsStopExternalAgentDelete(sessionId),
    onSuccess: () => {
      // Invalidate session query to refresh wolf-app-state
      queryClient.invalidateQueries({ queryKey: GET_SESSION_QUERY_KEY(sessionId) })
    }
  })
}

export function useGetSessionIdleStatus(sessionId: string, options?: { enabled?: boolean }) {
  const api = useApi()
  const apiClient = api.getApiClient()

  return useQuery({
    queryKey: ['session-idle-status', sessionId],
    queryFn: () => apiClient.v1SessionsSandboxStateDetail(sessionId),
    enabled: options?.enabled ?? true,
    refetchInterval: 30000, // Refetch every 30 seconds to update idle time
  })
}

export function invalidateSessionsQuery(queryClient: QueryClient) {
  queryClient.invalidateQueries({ queryKey: ["sessions"] })
}

/**
 * Hook to list interactions for a session with pagination
 * @param sessionId The session ID
 * @param page Page number (0-indexed)
 * @param perPage Number of interactions per page (default 20)
 * @param order Sort order: 'asc' (oldest first) or 'desc' (newest first)
 * @param options Query options
 */
export function useListInteractions(
  sessionId: string,
  page?: number,
  perPage?: number,
  order?: 'asc' | 'desc',
  options?: { enabled?: boolean; refetchInterval?: number | false }
) {
  const api = useApi()
  const apiClient = api.getApiClient()

  return useQuery({
    queryKey: LIST_INTERACTIONS_QUERY_KEY(sessionId, page, perPage, order),
    queryFn: () => apiClient.v1SessionsInteractionsDetail(sessionId, {
      page: page ?? 0,
      per_page: perPage ?? 20,
      order: order,
    }),
    enabled: options?.enabled ?? true,
    refetchInterval: options?.refetchInterval,
  })
}
