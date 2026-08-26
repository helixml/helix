import { useQuery } from '@tanstack/react-query'
import { TransportDescriptor, TransportField } from '../api/api'
import useApi from '../hooks/useApi'
import { useHelixOrgBase } from './helixOrgService'

export type TriggerKindDescriptor = TransportDescriptor
export type TriggerField = TransportField

export const TRIGGER_KIND_QUERY_KEYS = {
  all: (orgID: string) => ['helix-org', orgID, 'trigger-kinds'] as const,
}

export function useTriggerKinds(options?: { enabled?: boolean }) {
  const api = useApi()
  const { orgID } = useHelixOrgBase()
  return useQuery({
    queryKey: TRIGGER_KIND_QUERY_KEYS.all(orgID),
    queryFn: async () => {
      const res = await api.getApiClient().v1OrgsTriggerKindsDetail(orgID)
      return (res.data?.kinds ?? []) as TriggerKindDescriptor[]
    },
    enabled: !!orgID && (options?.enabled ?? true),
    staleTime: 5 * 60 * 1000,
  })
}

export function useTriggerKind(kind: string | undefined) {
  const { data } = useTriggerKinds()
  return data?.find((d) => d.kind === kind)
}
