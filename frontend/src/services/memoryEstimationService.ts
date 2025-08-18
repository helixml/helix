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

  return useQuery({
    queryKey: memoryEstimationQueryKey(modelId, numGpu),
    queryFn: async () => {
      const url = new URL('/api/v1/helix-models/memory-estimate', window.location.origin);
      url.searchParams.set('model_id', modelId);
      if (numGpu !== undefined) {
        url.searchParams.set('num_gpu', numGpu.toString());
      }

      const token = api.getToken()
      const response = await fetch(url.toString(), {
        headers: {
          'Authorization': `Bearer ${token}`,
          'Content-Type': 'application/json'
        }
      })
      
      if (!response.ok) {
        throw new Error(`Failed to fetch memory estimation: ${response.statusText}`)
      }

      return await response.json()
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
