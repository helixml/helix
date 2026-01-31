import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import useApi from '../hooks/useApi';
import { TypesDynamicModelInfo } from '../api/api';

export const modelInfoQueryKey = (id?: string) => [
  "model-info",
  ...(id ? [id] : [])
];

export const modelInfoListQueryKey = (provider?: string, name?: string) => [
  "model-info",
  "list",
  { provider, name }
];

export function useListModelInfos(provider?: string, name?: string) {
  const api = useApi()
  const apiClient = api.getApiClient()

  return useQuery({
    queryKey: modelInfoListQueryKey(provider, name),
    queryFn: async () => {
      const result = await apiClient.v1ModelInfoList({ provider, name })
      return result.data
    },
    staleTime: 5 * 60 * 1000, // 5 minutes
    gcTime: 10 * 60 * 1000, // 10 minutes
  });
}

export function useModelInfo(id: string, enabled: boolean = true) {
  const api = useApi()
  const apiClient = api.getApiClient()

  return useQuery({
    queryKey: modelInfoQueryKey(id),
    queryFn: async () => {
      const result = await apiClient.v1ModelInfoDetail(id)
      return result.data
    },
    enabled: enabled && !!id,
    staleTime: 5 * 60 * 1000, // 5 minutes
    gcTime: 10 * 60 * 1000, // 10 minutes
  });
}

export function useCreateModelInfo() {
  const api = useApi()
  const apiClient = api.getApiClient()
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (modelInfo: TypesDynamicModelInfo) => {
      const result = await apiClient.v1ModelInfoCreate(modelInfo)
      return result.data
    },
    onSuccess: (data) => {
      // Invalidate queries to refetch the list
      queryClient.invalidateQueries({ queryKey: modelInfoListQueryKey() })
      queryClient.invalidateQueries({ queryKey: modelInfoListQueryKey(data.provider, data.name) })
    }
  });
}

export function useUpdateModelInfo() {
  const api = useApi()
  const apiClient = api.getApiClient()
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (data: { id: string; modelInfo: TypesDynamicModelInfo }) => {
      const { id, modelInfo } = data;
      const result = await apiClient.v1ModelInfoUpdate(id, modelInfo)
      return result.data
    },
    onSuccess: (data) => {
      // Invalidate specific model info and list queries
      queryClient.invalidateQueries({ queryKey: modelInfoQueryKey(data.id) })
      queryClient.invalidateQueries({ queryKey: modelInfoListQueryKey() })
      queryClient.invalidateQueries({ queryKey: modelInfoListQueryKey(data.provider, data.name) })
    }
  });
}

export function useDeleteModelInfo() {
  const api = useApi()
  const apiClient = api.getApiClient()
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (id: string) => {
      const result = await apiClient.v1ModelInfoDelete(id)
      return result.data
    },
    onSuccess: (data, id) => {
      // Remove the deleted model info from cache and invalidate list queries
      queryClient.removeQueries({ queryKey: modelInfoQueryKey(id) })
      queryClient.invalidateQueries({ queryKey: modelInfoListQueryKey() })
      queryClient.invalidateQueries({ queryKey: modelInfoListQueryKey(id) })
    }
  });
}
