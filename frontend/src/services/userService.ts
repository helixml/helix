import { useQuery } from '@tanstack/react-query'
import useApi from '../hooks/useApi';

export const userQueryKey = (id: string) => [
  "user",
  id
];

export const userUsageQueryKey = (id: string) => [
  "user",
  id,
  "usage"
];

export function useGetUserTokenUsage() {
  const api = useApi()
  const apiClient = api.getApiClient()  

  return useQuery({
    queryKey: userQueryKey("token-usage"),
    queryFn: async () => {
      const response = await apiClient.v1UsersTokenUsageList()
      return response.data
    },
    refetchInterval: 30000, // 30 seconds
  })
}

export function useGetUserUsage(enabled?: boolean) {
  const api = useApi()
  const apiClient = api.getApiClient()  

  return useQuery({
    queryKey: userUsageQueryKey("current"),
    queryFn: async () => {
      const response = await apiClient.v1UsageList()
      return response.data
    },
    refetchInterval: 30000, // 30 seconds
  })
}