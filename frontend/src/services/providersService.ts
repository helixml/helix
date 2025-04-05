import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import useApi from '../hooks/useApi';

export const providersQueryKey = (loadModels: boolean = false) => [
  "providers",
  loadModels ? "withModels" : "withoutModels"
];

export function useListProviders(loadModels: boolean = false) {
  const api = useApi()
  const apiClient = api.getApiClient()  

  return useQuery({
    queryKey: providersQueryKey(loadModels),
    queryFn: async () => {
      const result = await apiClient.v1ProviderEndpointsList({
        with_models: loadModels,
      })
      return result.data
    },
    enabled: true,
    staleTime: 2 * 60 * 1000,
  });
}