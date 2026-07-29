import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import useApi from '../hooks/useApi'
import { TypesEvaluationSuite, TypesEvaluationRun } from '../api/api'

export const evaluationSuitesQueryKey = (appId: string) => ['evaluation-suites', appId]
export const evaluationRunsQueryKey = (appId: string, suiteId: string) => ['evaluation-runs', appId, suiteId]
export const evaluationRunQueryKey = (appId: string, runId: string) => ['evaluation-run', appId, runId]

export function useListEvaluationSuites(appId: string) {
  const api = useApi()
  const apiClient = api.getApiClient()

  return useQuery({
    queryKey: evaluationSuitesQueryKey(appId),
    queryFn: () => apiClient.v1AgentsEvaluationSuitesDetail(appId),
    enabled: !!appId,
  })
}

export function useCreateEvaluationSuite(appId: string) {
  const api = useApi()
  const apiClient = api.getApiClient()
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (suite: TypesEvaluationSuite) =>
      apiClient.v1AgentsEvaluationSuitesCreate(appId, suite),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: evaluationSuitesQueryKey(appId) })
    },
  })
}

export function useUpdateEvaluationSuite(appId: string) {
  const api = useApi()
  const apiClient = api.getApiClient()
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (suite: TypesEvaluationSuite) =>
      apiClient.v1AgentsEvaluationSuitesUpdate(appId, suite.id!, suite),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: evaluationSuitesQueryKey(appId) })
    },
  })
}

export function useDeleteEvaluationSuite(appId: string) {
  const api = useApi()
  const apiClient = api.getApiClient()
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (suiteId: string) =>
      apiClient.v1AgentsEvaluationSuitesDelete(appId, suiteId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: evaluationSuitesQueryKey(appId) })
    },
  })
}

export function useStartEvaluationRun(appId: string) {
  const api = useApi()
  const apiClient = api.getApiClient()
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (suiteId: string) =>
      apiClient.v1AgentsEvaluationSuitesRunsCreate(appId, suiteId),
    onSuccess: (_data, suiteId) => {
      queryClient.invalidateQueries({ queryKey: evaluationRunsQueryKey(appId, suiteId) })
    },
  })
}

export function useListEvaluationRuns(appId: string, suiteId: string) {
  const api = useApi()
  const apiClient = api.getApiClient()

  return useQuery({
    queryKey: evaluationRunsQueryKey(appId, suiteId),
    queryFn: () => apiClient.v1AgentsEvaluationSuitesRunsDetail(appId, suiteId),
    enabled: !!appId && !!suiteId,
  })
}

export function useDeleteEvaluationRun(appId: string, suiteId: string) {
  const api = useApi()
  const apiClient = api.getApiClient()
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (runId: string) =>
      apiClient.v1AgentsEvaluationRunsDelete(appId, runId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: evaluationRunsQueryKey(appId, suiteId) })
    },
  })
}

export function useGetEvaluationRun(appId: string, runId: string) {
  const api = useApi()
  const apiClient = api.getApiClient()

  return useQuery({
    queryKey: evaluationRunQueryKey(appId, runId),
    queryFn: () => apiClient.v1AgentsEvaluationRunsDetail(appId, runId),
    enabled: !!appId && !!runId,
    refetchInterval: (query) => {
      const data = query.state.data?.data
      if (data?.status === 'running' || data?.status === 'pending') {
        return 2000
      }
      return false
    },
  })
}
