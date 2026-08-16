import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import useApi from '../hooks/useApi'
import useRouter from '../hooks/useRouter'
import { useGetOrgByName } from './orgService'
import {
  TypesCodeAgentCredentialType,
  TypesCodeAgentRuntime,
  TypesOrgCodeAgentProviderStatus,
  TypesOrgCodeAgentProviderUpdate,
} from '../api/api'

export const codeAgentProvidersQueryKey = (orgId?: string) => ['code-agent-providers', orgId]

/**
 * The organization's coding-agent allow list, as the requesting user sees it.
 *
 * `available` and `viewer_has_subscription` are viewer-scoped by design:
 * subscription-backed runtimes resolve to the acting user's own subscription at
 * run time, so whether Claude Code works differs per member. Render `available`
 * rather than `enabled` anywhere the answer is "can I use this right now".
 */
export function useOrgCodeAgentProviders(orgId?: string, options?: { enabled?: boolean }) {
  const api = useApi()
  const apiClient = api.getApiClient()

  return useQuery({
    queryKey: codeAgentProvidersQueryKey(orgId),
    queryFn: async () => {
      const result = await apiClient.v1OrganizationsCodeAgentProvidersDetail(orgId!)
      return (result.data || []) as TypesOrgCodeAgentProviderStatus[]
    },
    enabled: !!orgId && (options?.enabled ?? true),
  })
}

/**
 * Mirrors the server's availability rule (codeAgentAvailability in
 * api/pkg/server/org_code_agent_provider_handlers.go) so an optimistic row
 * carries the same status text the server will send back. Any drift is
 * corrected by the revalidation that follows every mutation; this only has to
 * be right for the few hundred milliseconds in between.
 */
function optimisticAvailability(
  status: TypesOrgCodeAgentProviderStatus,
): Pick<TypesOrgCodeAgentProviderStatus, 'available' | 'unavailable_reason'> {
  if (!status.enabled) {
    return { available: false, unavailable_reason: 'Not enabled for this organization' }
  }
  if (status.credential_type === TypesCodeAgentCredentialType.CodeAgentCredentialTypeSubscription) {
    return status.viewer_has_subscription
      ? { available: true, unavailable_reason: undefined }
      : { available: false, unavailable_reason: 'Connect your own subscription to use this agent' }
  }
  if (!status.provider_endpoint_id) {
    return { available: false, unavailable_reason: 'No provider configured' }
  }
  return { available: true, unavailable_reason: undefined }
}

function applyOptimisticUpdate(
  current: TypesOrgCodeAgentProviderStatus[],
  body: {
    providers?: TypesOrgCodeAgentProviderUpdate[]
    delete?: { runtime: string; name: string }[]
  },
): TypesOrgCodeAgentProviderStatus[] {
  const removed = new Set((body.delete || []).map((ref) => `${ref.runtime}:${ref.name}`))
  let next = current.filter((row) => !removed.has(`${row.runtime}:${row.name || ''}`))

  for (const update of body.providers || []) {
    const key = `${update.runtime}:${update.name || ''}`
    const existing = next.find((row) => `${row.runtime}:${row.name || ''}` === key)
    if (existing) {
      const merged: TypesOrgCodeAgentProviderStatus = { ...existing, ...update }
      next = next.map((row) => (row === existing
        ? { ...merged, ...optimisticAvailability(merged) }
        : row))
      continue
    }
    // A newly added flavour has no row yet; show it immediately rather than
    // waiting for the refetch to make it appear.
    const added: TypesOrgCodeAgentProviderStatus = {
      ...update,
      is_flavour: !!update.name,
      supports_subscription: next.find((row) => row.runtime === update.runtime)?.supports_subscription,
      viewer_has_subscription: next.find((row) => row.runtime === update.runtime)?.viewer_has_subscription,
    }
    next = [...next, { ...added, ...optimisticAvailability(added) }]
  }

  return next
}

export function useUpdateOrgCodeAgentProviders(orgId?: string) {
  const api = useApi()
  const apiClient = api.getApiClient()
  const queryClient = useQueryClient()

  return useMutation({
    // Sends only the rows being changed — the API leaves unnamed runtimes
    // untouched, so a single toggle cannot clear the rest of the org's config.
    mutationFn: async (body: {
      providers?: TypesOrgCodeAgentProviderUpdate[]
      delete?: { runtime: string; name: string }[]
    }) => {
      const result = await apiClient.v1OrganizationsCodeAgentProvidersUpdate(orgId!, body as never)
      return (result.data || []) as TypesOrgCodeAgentProviderStatus[]
    },
    // Optimistic, because a switch that waits for a round-trip before moving
    // reads as a flicker: click, nothing, then it jumps. The cache is written
    // immediately, rolled back if the request fails, and revalidated either way
    // — the server stays the source of truth, it just is not in the critical
    // path of the animation.
    onMutate: async (body) => {
      const queryKey = codeAgentProvidersQueryKey(orgId)
      await queryClient.cancelQueries({ queryKey })
      const previous = queryClient.getQueryData<TypesOrgCodeAgentProviderStatus[]>(queryKey)
      queryClient.setQueryData<TypesOrgCodeAgentProviderStatus[]>(
        queryKey,
        (current) => applyOptimisticUpdate(current || [], body),
      )
      return { previous }
    },
    onError: (_error, _body, context) => {
      const queryKey = codeAgentProvidersQueryKey(orgId)
      if (context?.previous) queryClient.setQueryData(queryKey, context.previous)
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: codeAgentProvidersQueryKey(orgId) })
    },
  })
}

/** The runtimes this viewer can actually run right now. */
export function availableRuntimes(
  providers: TypesOrgCodeAgentProviderStatus[] | undefined,
): TypesOrgCodeAgentProviderStatus[] {
  return (providers || []).filter((provider) => provider.available)
}

/** Look up one runtime's status. */
export function findRuntimeStatus(
  providers: TypesOrgCodeAgentProviderStatus[] | undefined,
  runtime?: TypesCodeAgentRuntime | string,
): TypesOrgCodeAgentProviderStatus | undefined {
  if (!runtime) return undefined
  return (providers || []).find((provider) => provider.runtime === runtime)
}

/**
 * True when the runtime is configured to authenticate with a personal
 * subscription. Callers use this to decide whether to route through the Helix
 * proxy (API key) or hand the agent OAuth credentials.
 */
export function usesSubscription(status?: TypesOrgCodeAgentProviderStatus): boolean {
  return status?.credential_type === TypesCodeAgentCredentialType.CodeAgentCredentialTypeSubscription
}

/**
 * Whether this viewer has any coding agent they could start a task with.
 *
 * Resolves the org itself so a caller only needs one import. `loading` matters:
 * a surface must not claim "nothing configured" while the answer is still in
 * flight, or it flashes an error state on every load.
 */
export function useHasAvailableCodeAgents(): { hasAny: boolean; loading: boolean } {
  const router = useRouter()
  const orgName = router.params.org_id
  const { data: org, isLoading: loadingOrg } = useGetOrgByName(orgName, orgName !== undefined)
  const { data: providers, isLoading, isFetching } = useOrgCodeAgentProviders(
    org?.id,
    { enabled: !loadingOrg },
  )
  return {
    hasAny: availableRuntimes(providers).length > 0,
    // Report loading while a background refetch is in flight, not only on the
    // first load. Coming back from the Providers page renders the cached answer
    // first, and a caller that treated that as settled would say "no agents"
    // about an org that just enabled one.
    loading: loadingOrg || isLoading || isFetching,
  }
}
