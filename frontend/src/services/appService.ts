import { useQuery } from '@tanstack/react-query'
import useApi from '../hooks/useApi';
// import { TypesStepInfo, ContentType } from '../api/api';

export const appStepsQueryKey = (id: string, interactionId: string) => [
  "app-steps",
  id,
  interactionId
];

// useListSessionSteps returns the steps for a session, it includes
// steps for all interactions in the session
export function useListAppSteps(appId: string, interactionId: string, options?: { enabled?: boolean }) {
  const api = useApi()
  const apiClient = api.getApiClient()  

  return useQuery({
    queryKey: appStepsQueryKey(appId, interactionId),
    queryFn: () => apiClient.v1AppsStepInfoDetail(appId, {interactionId}),
    enabled: options?.enabled ?? true
  })
}
