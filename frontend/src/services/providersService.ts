import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import useApi from '../hooks/useApi';
import { TypesProviderEndpoint, RequestParams, TypesUpdateProviderEndpoint } from '../api/api';

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

export function useCreateProviderEndpoint() {
  const api = useApi()
  const apiClient = api.getApiClient()  
  const queryClient = useQueryClient();

  return useMutation({
    

    mutationFn: async (providerEndpoint: Partial<TypesProviderEndpoint>) => {
      const result = await apiClient.v1ProviderEndpointsCreate(providerEndpoint as RequestParams)
      return result.data
    },
    onSuccess: () => {
      // Invalidate provider queries to refetch the list
      queryClient.invalidateQueries({ queryKey: providersQueryKey() })
      queryClient.invalidateQueries({ queryKey: providersQueryKey(true) })
    }
  });
}

export function useUpdateProviderEndpoint(id: string, providerEndpoint: Partial<TypesUpdateProviderEndpoint>) {
  const api = useApi()
  const apiClient = api.getApiClient()  
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async () => {
      const result = await apiClient.v1ProviderEndpointsUpdate(id, providerEndpoint as RequestParams)
      return result.data
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: providersQueryKey() })
      queryClient.invalidateQueries({ queryKey: providersQueryKey(true) })
    }
  });
}