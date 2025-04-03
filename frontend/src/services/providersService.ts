import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import useApi from '../hooks/useApi';

export const providersQueryKey = () => [
  "providers",
];

export function useListProviders() {
  const api = useApi()
  const apiClient = api.getApiClient()  

  return useQuery({
    queryKey: providersQueryKey(),
    queryFn: async () => {
      const result = await apiClient.v1ProviderEndpointsList()
      return result.data
    },
    enabled: true,
    staleTime: 2 * 60 * 1000,
  });
}