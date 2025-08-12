import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import useApi from '../hooks/useApi';
import { TypesProviderEndpoint, RequestParams, TypesUpdateProviderEndpoint, ContentType } from '../api/api';

export const providersQueryKey = (loadModels: boolean = false) => [
  "providers",
  loadModels ? "withModels" : "withoutModels"
];

export interface ListProvidersOptions {
  loadModels?: boolean;
  orgId?: string;
  all?: boolean;
  enabled?: boolean;
}

export function useListProviders(options: ListProvidersOptions = {}) {
  const { loadModels = false, orgId, all = false, enabled = true } = options;
  const api = useApi()
  const apiClient = api.getApiClient()  

  return useQuery({
    queryKey: providersQueryKey(loadModels),
    queryFn: async () => {
      const result = await apiClient.v1ProviderEndpointsList({
        with_models: loadModels,
        org_id: orgId,
        all: all,
      })
      return result.data
    },  
    enabled: enabled,
    staleTime: 3 * 1000, // 3 seconds (useful when going between pages)
  });
}

export function useCreateProviderEndpoint() {
  const api = useApi()
  const apiClient = api.getApiClient()  
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (providerEndpoint: Partial<TypesProviderEndpoint>) => {
      const result = await apiClient.v1ProviderEndpointsCreate({
        body: providerEndpoint,
        type: ContentType.Json
      } as RequestParams)
      return result.data
    },
    onSuccess: () => {
      // Invalidate provider queries to refetch the list
      queryClient.invalidateQueries({ queryKey: providersQueryKey() })
      queryClient.invalidateQueries({ queryKey: providersQueryKey(true) })
    }
  });
}

export function useUpdateProviderEndpoint(id: string) {
  const api = useApi()
  const apiClient = api.getApiClient()  
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (providerEndpoint: Partial<TypesUpdateProviderEndpoint>) => {
      const result = await apiClient.v1ProviderEndpointsUpdate(id, {
        body: providerEndpoint,
        type: ContentType.Json
      } as RequestParams)
      return result.data
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: providersQueryKey() })
      queryClient.invalidateQueries({ queryKey: providersQueryKey(true) })
    }
  });
}

export function useDeleteProviderEndpoint() {
  const api = useApi()
  const apiClient = api.getApiClient()  
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (id: string) => {
      const result = await apiClient.v1ProviderEndpointsDelete(id)
      return result.data
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: providersQueryKey() })
      queryClient.invalidateQueries({ queryKey: providersQueryKey(true) })
    }
  });
}