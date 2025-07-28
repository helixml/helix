import { useQuery } from '@tanstack/react-query'
import useApi from '../hooks/useApi';

export const sessionStepsQueryKey = (id: string) => [
  "session-steps",
  id
];

export const getSessionQueryKey = (id: string) => [
  "session",
  id
];

export const listSessionsQueryKey = () => [
  "sessions"
];

// useListSessionSteps returns the steps for a session, it includes
// steps for all interactions in the session
export function useListSessionSteps(sessionId: string, options?: { enabled?: boolean }) {
  const api = useApi()
  const apiClient = api.getApiClient()  

  return useQuery({
    queryKey: sessionStepsQueryKey(sessionId),
    queryFn: () => apiClient.v1SessionsStepInfoDetail(sessionId),
    enabled: options?.enabled ?? true
  })
}

export function useGetSession(sessionId: string, options?: { enabled?: boolean }) {
  const api = useApi()
  const apiClient = api.getApiClient()

  return useQuery({
    queryKey: getSessionQueryKey(sessionId),
    queryFn: () => apiClient.v1SessionsDetail(sessionId),
    enabled: options?.enabled ?? true
  })
}

export function useListSessions(options?: { enabled?: boolean }) {
  const api = useApi()
  const apiClient = api.getApiClient()

  return useQuery({
    queryKey: listSessionsQueryKey(),
    queryFn: () => apiClient.v1SessionsList(),
    enabled: options?.enabled ?? true
  })
}