import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import useApi from '../hooks/useApi';
import { TypesModel } from '../api/api';

export const helixModelsQueryKey = (runtime: string = "") => [
  "helixModels",
  runtime
];

export function useListHelixModels(runtime: string = "", availableOnly: boolean = true) {
  const api = useApi()

  return useQuery({
    queryKey: helixModelsQueryKey(runtime),
    queryFn: async () => {
      // Build query parameters
      const params = new URLSearchParams()
      if (runtime) {
        params.append('runtime', runtime)
      }  
      if (availableOnly) {
        params.append('available_only', 'true')
      }
      
      const url = `/api/v1/helix-models?${params.toString()}`
      const result = await api.get(url)
      return result
    },
    enabled: true,
    staleTime: 3 * 1000, // 3 seconds (useful when going between pages)
  });
}

export function useCreateHelixModel() {
  const api = useApi()
  const apiClient = api.getApiClient()
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (helixModel: Partial<TypesModel>) => {
      // Assuming v1HelixModelsCreate exists and takes the model body directly
      const result = await apiClient.v1HelixModelsCreate(helixModel)
      return result.data
    },
    onSuccess: () => {
      // Invalidate queries to refetch the list
      queryClient.invalidateQueries({ queryKey: helixModelsQueryKey() })
      queryClient.invalidateQueries({ queryKey: helixModelsQueryKey("gpu") }) // Example: invalidate specific runtimes if needed
      queryClient.invalidateQueries({ queryKey: helixModelsQueryKey("cpu") }) // Example
    }
  });
}

export function useUpdateHelixModel() {
  const api = useApi()
  const apiClient = api.getApiClient()
  const queryClient = useQueryClient();

  return useMutation({
    // Update mutationFn signature to accept id and model
    mutationFn: async (data: { id: string; helixModel: Partial<TypesModel> }) => {
      const { id, helixModel } = data; // Destructure id and model
      // Use id from the data argument here
      const result = await apiClient.v1HelixModelsUpdate(id, helixModel)
      return result.data
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: helixModelsQueryKey() })
      queryClient.invalidateQueries({ queryKey: helixModelsQueryKey("gpu") })
      queryClient.invalidateQueries({ queryKey: helixModelsQueryKey("cpu") })
    }
  });
}

export function useDeleteHelixModel(id: string) {
  const api = useApi()
  const apiClient = api.getApiClient()
  const queryClient = useQueryClient();

  return useMutation({
    // Assuming v1HelixModelsDelete exists and takes id
    mutationFn: async () => {
      const result = await apiClient.v1HelixModelsDelete(id)
      return result.data // Or handle potential no-content response
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: helixModelsQueryKey() })
      queryClient.invalidateQueries({ queryKey: helixModelsQueryKey("gpu") })
      queryClient.invalidateQueries({ queryKey: helixModelsQueryKey("cpu") })
    }
  });
}
