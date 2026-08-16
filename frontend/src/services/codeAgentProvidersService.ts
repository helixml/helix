import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import useApi from '../hooks/useApi'
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
      const result = await apiClient.v1OrgsCodeAgentProvidersDetail(orgId!)
      return (result.data || []) as TypesOrgCodeAgentProviderStatus[]
    },
    enabled: !!orgId && (options?.enabled ?? true),
  })
}

export function useUpdateOrgCodeAgentProviders(orgId?: string) {
  const api = useApi()
  const apiClient = api.getApiClient()
  const queryClient = useQueryClient()

  return useMutation({
    // Sends only the rows being changed — the API leaves unnamed runtimes
    // untouched, so a single toggle cannot clear the rest of the org's config.
    mutationFn: async (providers: TypesOrgCodeAgentProviderUpdate[]) => {
      const result = await apiClient.v1OrgsCodeAgentProvidersUpdate(orgId!, { providers })
      return (result.data || []) as TypesOrgCodeAgentProviderStatus[]
    },
    onSuccess: () => {
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
