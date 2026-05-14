import { useQuery } from '@tanstack/react-query'
import useApi from '../hooks/useApi'

// Shared filter shape passed from the UI to both endpoints. Empty
// strings are sent to the server as omitted params - the underlying
// client tolerates undefined fields.
export interface UsageFilter {
  from?: string // RFC3339
  to?: string
  org_id?: string
  user_id?: string
  project_id?: string
  app_id?: string
  session_id?: string
  provider?: string
  model?: string
  sort_by?: string
  sort_dir?: 'asc' | 'desc'
  page?: number
  page_size?: number
}

export type UsageGrouping = 'org' | 'user' | 'project' | 'session' | 'model'

const stableKey = (label: string, f: UsageFilter, extra?: Record<string, unknown>) =>
  [label, f.from, f.to, f.org_id, f.user_id, f.project_id, f.app_id, f.session_id,
    f.provider, f.model, f.sort_by, f.sort_dir, f.page, f.page_size,
    JSON.stringify(extra ?? {})]

export function useUsageSummary(filter: UsageFilter, enabled = true) {
  const api = useApi()
  const apiClient = api.getApiClient()

  return useQuery({
    queryKey: stableKey('usage_summary', filter),
    enabled,
    queryFn: async () => {
      const res = await apiClient.api.v1UsageAggregateSummaryList(filter as Record<string, string>)
      return res.data
    },
  })
}

export function useUsageGrouped(groupBy: UsageGrouping, filter: UsageFilter, enabled = true) {
  const api = useApi()
  const apiClient = api.getApiClient()

  return useQuery({
    queryKey: stableKey(`usage_grouped_${groupBy}`, filter, { groupBy }),
    enabled,
    queryFn: async () => {
      const res = await apiClient.api.v1UsageAggregateGroupedList({
        ...(filter as Record<string, string>),
        group_by: groupBy,
      })
      return res.data
    },
  })
}
