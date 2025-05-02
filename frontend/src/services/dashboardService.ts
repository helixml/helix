import { useQuery } from '@tanstack/react-query'
import useApi from '../hooks/useApi';

export const dashboardQueryKey = () => [
  "dashboard"
];

export function useGetDashboardData() {
  const api = useApi()
  const apiClient = api.getApiClient()  

  return useQuery({
    queryKey: dashboardQueryKey(),
    queryFn: async () => {
      const result = await apiClient.v1DashboardList()
      return result.data
    },
    enabled: true,
    staleTime: 1000, // 1 second
    refetchInterval: 1000, // Refetch every 1 second
  });
}
