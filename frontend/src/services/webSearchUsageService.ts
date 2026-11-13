import { useQuery, keepPreviousData } from '@tanstack/react-query'

import useApi from '../hooks/useApi'
import { TypesWebSearchEndpoint } from '../api/api'

export const webSearchUsageQueryKey = (orgId: string, from: string, to: string) => [
  'websearch',
  'usage',
  orgId,
  from,
  to,
]

export const webSearchRequestsQueryKey = (
  orgId: string,
  from: string,
  to: string,
  endpoint: TypesWebSearchEndpoint | undefined,
  limit: number,
  offset: number,
) => ['websearch', 'requests', orgId, from, to, endpoint ?? 'all', limit, offset]

// orgId is the URL segment — the backend resolves an org slug or id.
export function useWebSearchUsage(orgId: string | undefined, from: string, to: string) {
  const api = useApi()
  const apiClient = api.getApiClient()
  return useQuery({
    queryKey: webSearchUsageQueryKey(orgId ?? '', from, to),
    queryFn: async () => {
      const result = await apiClient.v1OrganizationsWebsearchUsageDetail(orgId!, { from, to })
      return result.data
    },
    enabled: !!orgId,
    placeholderData: keepPreviousData,
  })
}

export function useWebSearchRequests(
  orgId: string | undefined,
  options: {
    from: string
    to: string
    endpoint?: TypesWebSearchEndpoint
    limit: number
    offset: number
  },
) {
  const api = useApi()
  const apiClient = api.getApiClient()
  const { from, to, endpoint, limit, offset } = options
  return useQuery({
    queryKey: webSearchRequestsQueryKey(orgId ?? '', from, to, endpoint, limit, offset),
    queryFn: async () => {
      const result = await apiClient.v1OrganizationsWebsearchRequestsDetail(orgId!, {
        from,
        to,
        endpoint,
        limit,
        offset,
      })
      return result.data
    },
    enabled: !!orgId,
    placeholderData: keepPreviousData,
  })
}
