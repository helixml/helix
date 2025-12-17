import { useMutation, useQueryClient } from '@tanstack/react-query'
import useApi from '../hooks/useApi'
import useSnackbar from '../hooks/useSnackbar'
import { TypesSpecTask } from '../api/api'

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
    onSuccess: (response: TypesSpecTask) => {
      if (response.pull_request_url) {
        // External repo (ADO) - show link to PR
        snackbar.success(`Pull request opened! View PR: ${response.pull_request_url}`)
      } else if (response.pull_request_id) {
        // PR exists but no URL
        snackbar.success('Pull request #' + response.pull_request_id + ' opened - awaiting merge')
      } else if (response.status === 'pull_request') {
        // External repo - task moved to pull_request status, waiting for agent to push
        snackbar.success('Agent will push changes to open a pull request...')
      } else {
        // Internal repo - agent will merge
        snackbar.success('Implementation approved! Agent will merge to your primary branch...')
      }
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
