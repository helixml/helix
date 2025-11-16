import { useQuery } from '@tanstack/react-query'
import useApi from '../hooks/useApi';

export const WOLF_HEALTH_QUERY_KEY = () => ['wolf-health'];

/**
 * useWolfHealth - Get Wolf system health including thread heartbeat status
 * Returns thread heartbeat information and deadlock detection status
 */
export function useWolfHealth(options?: {
  enabled?: boolean;
  refetchInterval?: number | false;
}) {
  const api = useApi()
  const apiClient = api.getApiClient()

  return useQuery({
    queryKey: WOLF_HEALTH_QUERY_KEY(),
    queryFn: async () => {
      const result = await apiClient.v1WolfHealthList()
      // The generated client returns Axios response, need to extract .data
      return result.data
    },
    // Poll every 5 seconds for live monitoring
    refetchInterval: options?.refetchInterval ?? 5000,
    enabled: options?.enabled ?? true,
    // Don't retry on error - if Wolf is down, retrying won't help
    retry: false,
    // Cache data for 10 seconds (reduce stale data during rapid polling)
    staleTime: 10000,
  })
}
