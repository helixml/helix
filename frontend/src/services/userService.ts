import { useQuery } from '@tanstack/react-query'
import useApi from '../hooks/useApi';

export const userQueryKey = (id: string) => [
  "user",
  id
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