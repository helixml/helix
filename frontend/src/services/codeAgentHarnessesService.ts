import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import useApi from '../hooks/useApi'
import useRouter from '../hooks/useRouter'
import { useGetOrgByName } from './orgService'
import {
  TypesCodeAgentRuntime,
  TypesOrgCodeAgentHarnessStatus,
  TypesOrgCodeAgentHarnessUpdate,
} from '../api/api'

export const codeAgentHarnessesQueryKey = (orgId?: string) => ['code-agent-harnesses', orgId]

export function useOrgCodeAgentHarnesses(orgId?: string, options?: { enabled?: boolean }) {
  const apiClient = useApi().getApiClient()
  return useQuery({
    queryKey: codeAgentHarnessesQueryKey(orgId),
    queryFn: async () => {
      const result = await apiClient.v1OrganizationsCodeAgentHarnessesDetail(orgId!)
      return (result.data || []) as TypesOrgCodeAgentHarnessStatus[]
    },
    enabled: !!orgId && (options?.enabled ?? true),
  })
}

export function useUpdateOrgCodeAgentHarnesses(orgId?: string) {
  const apiClient = useApi().getApiClient()
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (harnesses: TypesOrgCodeAgentHarnessUpdate[]) => {
      const result = await apiClient.v1OrganizationsCodeAgentHarnessesUpdate(
        orgId!,
        { harnesses },
      )
      return (result.data || []) as TypesOrgCodeAgentHarnessStatus[]
    },
    onMutate: async (updates) => {
      const queryKey = codeAgentHarnessesQueryKey(orgId)
      await queryClient.cancelQueries({ queryKey })
      const previous = queryClient.getQueryData<TypesOrgCodeAgentHarnessStatus[]>(queryKey)
      queryClient.setQueryData<TypesOrgCodeAgentHarnessStatus[]>(queryKey, (current) =>
        (current || []).map((harness) => {
          const update = updates.find((candidate) => candidate.runtime === harness.runtime)
          return update ? { ...harness, enabled: update.enabled } : harness
        }),
      )
      return { previous }
    },
    onError: (_error, _updates, context) => {
      if (context?.previous) {
        queryClient.setQueryData(codeAgentHarnessesQueryKey(orgId), context.previous)
      }
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: codeAgentHarnessesQueryKey(orgId) })
    },
  })
}

export function findHarnessStatus(
  harnesses: TypesOrgCodeAgentHarnessStatus[] | undefined,
  runtime?: TypesCodeAgentRuntime | string,
): TypesOrgCodeAgentHarnessStatus | undefined {
  return runtime ? (harnesses || []).find((harness) => harness.runtime === runtime) : undefined
}

export function useHasEnabledCodeAgentHarnesses(): { hasAny: boolean; loading: boolean } {
  const router = useRouter()
  const orgName = router.params.org_id
  const { data: org, isLoading: loadingOrg } = useGetOrgByName(orgName, orgName !== undefined)
  const { data: harnesses, isLoading, isFetching } = useOrgCodeAgentHarnesses(
    org?.id,
    { enabled: !loadingOrg },
  )
  return {
    hasAny: !orgName || (harnesses || []).some((harness) => harness.enabled),
    loading: loadingOrg || isLoading || isFetching,
  }
}
