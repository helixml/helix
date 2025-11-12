import { useMutation, useQueryClient } from '@tanstack/react-query'
import useApi from '../hooks/useApi'
import useSnackbar from '../hooks/useSnackbar'

export function useApproveImplementation(specTaskId: string) {
  const api = useApi()
  const apiClient = api.getApiClient()
  const queryClient = useQueryClient()
  const snackbar = useSnackbar()

  return useMutation({
    mutationFn: async () => {
      const response = await apiClient.v1SpecTasksApproveImplementationCreate(specTaskId)
      return response.data
    },
    onSuccess: () => {
      snackbar.success('Implementation approved! Agent will merge to main...')
      // Invalidate queries to refetch task
      queryClient.invalidateQueries({ queryKey: ['spec-tasks', specTaskId] })
      queryClient.invalidateQueries({ queryKey: ['spec-tasks'] })
    },
    onError: (error: any) => {
      snackbar.error(error?.response?.data?.message || 'Failed to approve implementation')
    },
  })
}

export function useStopAgent(specTaskId: string) {
  const api = useApi()
  const apiClient = api.getApiClient()
  const queryClient = useQueryClient()
  const snackbar = useSnackbar()

  return useMutation({
    mutationFn: async () => {
      const response = await apiClient.v1SpecTasksStopAgentCreate(specTaskId)
      return response.data
    },
    onSuccess: () => {
      snackbar.success('Agent stop requested')
      queryClient.invalidateQueries({ queryKey: ['spec-tasks', specTaskId] })
      queryClient.invalidateQueries({ queryKey: ['spec-tasks'] })
    },
    onError: (error: any) => {
      snackbar.error(error?.response?.data?.message || 'Failed to stop agent')
    },
  })
}
