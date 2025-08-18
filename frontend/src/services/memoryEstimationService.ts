import { useQuery } from '@tanstack/react-query'
import useApi from '../hooks/useApi'
import { ControllerMemoryEstimationResponse } from '../api/api'

export const memoryEstimationQueryKey = (modelId: string, numGpu?: number) => [
  "memoryEstimation",
  modelId,
  numGpu
]

export function useModelMemoryEstimation(modelId: string, numGpu?: number, options?: { enabled?: boolean }) {
  const api = useApi()
  const apiClient = api.getApiClient()

  return useQuery({
    queryKey: memoryEstimationQueryKey(modelId, numGpu),
    queryFn: async () => {
      const query: { num_gpu?: number; context_length?: number; batch_size?: number } = {}
      if (numGpu !== undefined) {
        query.num_gpu = numGpu
      }
      
      const response = await apiClient.v1HelixModelsMemoryEstimateDetail(modelId, query)
      return response.data
    },
    enabled: options?.enabled !== false && !!modelId,
    staleTime: 5 * 60 * 1000, // 5 minutes
    refetchInterval: 30 * 1000, // Refetch every 30 seconds for real-time updates
  })
}

export function useAllModelsMemoryEstimation(numGpu?: number) {
  const api = useApi()
  const apiClient = api.getApiClient()

  return useQuery({
    queryKey: ["memoryEstimation", "all", numGpu],
    queryFn: async () => {
      const query: { model_ids?: string; num_gpu?: number } = {}
      if (numGpu !== undefined) {
        query.num_gpu = numGpu
      }
      
      const response = await apiClient.v1HelixModelsMemoryEstimatesList(query)
      return response.data
    },
    staleTime: 5 * 60 * 1000, // 5 minutes
    refetchInterval: 60 * 1000, // Refetch every minute for background updates
  })
}

// Helper function to format memory size
export function formatMemorySize(bytes: number): string {
  if (bytes === 0) return '0 B'
  
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i]
}
