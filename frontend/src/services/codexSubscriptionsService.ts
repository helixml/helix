import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  ContentType,
  TypesCreateCodexSubscriptionRequest,
  TypesStartCodexLoginRequest,
} from '../api/api'
import useApi from '../hooks/useApi'
import { codeAgentHarnessesQueryKey } from './codeAgentHarnessesService'

export const codexSubscriptionsQueryKey = ['codex-subscriptions']

export function useCodexSubscriptions() {
  const apiClient = useApi().getApiClient()
  return useQuery({
    queryKey: codexSubscriptionsQueryKey,
    queryFn: async () => (await apiClient.v1CodexSubscriptionsList()).data,
  })
}

export function useCreateCodexSubscription() {
  const apiClient = useApi().getApiClient()
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (body: TypesCreateCodexSubscriptionRequest) => (
      await apiClient.v1CodexSubscriptionsCreate(body, { type: ContentType.Json })
    ).data,
    onSuccess: (_subscription, request) => {
      queryClient.invalidateQueries({ queryKey: codexSubscriptionsQueryKey })
      if (request.organization_id) {
        queryClient.invalidateQueries({ queryKey: codeAgentHarnessesQueryKey(request.organization_id) })
      }
    },
  })
}

export function useDeleteCodexSubscription() {
  const apiClient = useApi().getApiClient()
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (id: string) => (await apiClient.v1CodexSubscriptionsDelete(id)).data,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: codexSubscriptionsQueryKey }),
  })
}

export function useStartCodexLogin() {
  const apiClient = useApi().getApiClient()
  return useMutation({
    mutationFn: async (body: TypesStartCodexLoginRequest) => (
      await apiClient.v1CodexSubscriptionsStartLoginCreate(body, { type: ContentType.Json })
    ).data,
  })
}

export function useCancelCodexLogin() {
  const apiClient = useApi().getApiClient()
  return useMutation({
    mutationFn: async (sessionId: string) => (
      await apiClient.v1CodexSubscriptionsLoginDelete(sessionId)
    ).data,
  })
}

export function usePollCodexLogin(sessionId: string) {
  const apiClient = useApi().getApiClient()
  return useQuery({
    queryKey: ['codex-subscriptions', 'login', sessionId],
    queryFn: async () => (await apiClient.v1CodexSubscriptionsPollLoginDetail(sessionId)).data,
    enabled: !!sessionId,
    refetchInterval: sessionId ? 1000 : false,
  })
}
