import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import useApi from '../hooks/useApi';
import { TypesStepInfo, ContentType } from '../api/api';

export const sessionStepsQueryKey = (id: string) => [
  "session-steps",
  id
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
