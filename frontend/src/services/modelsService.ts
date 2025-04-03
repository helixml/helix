import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import useApi from '../hooks/useApi';

export const modelsQueryKey = (provider: string) => [
  "models",
  provider,
];

export function useListModels(provider: string) {
  const api = useApi()
  const apiClient = api.getApiClient()  

  return useQuery({
    queryKey: modelsQueryKey(provider),
    queryFn: async () => {
      const result = await apiClient.v1ModelsList({ provider })
      return result.data
    },
    enabled: true,
    staleTime: 2 * 60 * 1000,
  });
}