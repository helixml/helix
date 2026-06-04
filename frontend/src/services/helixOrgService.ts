import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import useApi from '../hooks/useApi'
import useRouter from '../hooks/useRouter'
import {
  ApiChart,
  ApiChartNode,
  ApiCreatePositionRequest,
  ApiCreateRoleRequest,
  ApiCreateStreamRequest,
  ApiEventCard,
  ApiGitHubReposResponse,
  ApiHireGrantInput,
  ApiHireWorkerRequest,
  ApiHireWorkerResponse,
  ApiInstallGitHubWebhookResponse,
  ApiPositionDTO,
  ApiPositionSubscriptionDTO,
  ApiPositionSubscriptionsResponse,
  ApiRoleDTO,
  ApiSettingsResponse,
  ApiSettingsSpecDTO,
  ApiStreamDTO,
  ApiStreamsResponse,
  ApiToolDTO,
  ApiUpdateStreamRequest,
  ApiWorkerBadge,
  ApiWorkerChatDTO,
  ApiWorkerDTO,
  ApiWorkerDetailDTO,
} from '../api/api'

// Re-exported aliases. Generated Api* types mark every field
// optional; consumers use them as if fields are present. strict
// null checks are off project-wide so plain aliases suffice.
export type WorkerBadge = ApiWorkerBadge
export type ChartNode = ApiChartNode
export type RoleDTO = ApiRoleDTO
export type WorkerDTO = ApiWorkerDTO
export type WorkerDetailDTO = ApiWorkerDetailDTO
export type ToolDTO = ApiToolDTO
export type StreamDTO = ApiStreamDTO
export type EventCard = ApiEventCard
export type SettingsSpecDTO = ApiSettingsSpecDTO
export type PositionDTO = ApiPositionDTO
export type SettingsResponse = ApiSettingsResponse
export type StreamsResponse = ApiStreamsResponse
export type GitHubRepoDTO = NonNullable<ApiGitHubReposResponse['repos']>[number]
export type GitHubReposResponse = ApiGitHubReposResponse
export type InstallGitHubWebhookResponse = ApiInstallGitHubWebhookResponse
export type PositionSubscription = ApiPositionSubscriptionDTO
export type PositionSubscriptionsResponse = ApiPositionSubscriptionsResponse
export type Chart = ApiChart

export type HireGrantInput = ApiHireGrantInput
export interface HireWorkerRequest extends Omit<ApiHireWorkerRequest, 'kind'> {
  position_id: string
  kind: 'human' | 'ai'
  identity_content: string
}
export type HireWorkerResponse = ApiHireWorkerResponse
export type CreateRoleRequest = ApiCreateRoleRequest & { id: string; content: string }
export type CreatePositionRequest = ApiCreatePositionRequest & { id: string; role_id: string }
export type CreateStreamRequest = ApiCreateStreamRequest & { name: string }
export type UpdateStreamRequest = ApiUpdateStreamRequest

export interface HelixModelInfo {
  id: string
  name?: string
  description?: string
  context_length?: number
}

export const QUERY_KEYS = {
  chart: (orgID: string) => ['helix-org', orgID, 'chart'] as const,
  worker: (orgID: string, id: string) => ['helix-org', orgID, 'workers', id] as const,
  workers: (orgID: string) => ['helix-org', orgID, 'workers'] as const,
  role: (orgID: string, id: string) => ['helix-org', orgID, 'roles', id] as const,
  roles: (orgID: string) => ['helix-org', orgID, 'roles'] as const,
  tools: (orgID: string) => ['helix-org', orgID, 'tools'] as const,
  settings: (orgID: string) => ['helix-org', orgID, 'settings'] as const,
  providers: () => ['helix-org', 'providers'] as const,
  modelsForProvider: (provider: string) => ['helix-org', 'models', provider] as const,
  streams: (orgID: string) => ['helix-org', orgID, 'streams'] as const,
  stream: (orgID: string, id: string) => ['helix-org', orgID, 'streams', id] as const,
  positionSubs: (orgID: string, positionID: string) => ['helix-org', orgID, 'positions', positionID, 'subscriptions'] as const,
}

export function useHelixOrgBase(): { base: string; orgID: string } {
  const { params } = useRouter()
  const orgID = (params.org_id as string) || ''
  const base = orgID ? `/api/v1/orgs/${encodeURIComponent(orgID)}` : ''
  return { base, orgID }
}

export function useHelixOrgChart(options?: { enabled?: boolean }) {
  const api = useApi()
  const { orgID } = useHelixOrgBase()
  return useQuery({
    queryKey: QUERY_KEYS.chart(orgID),
    queryFn: async () => {
      const res = await api.getApiClient().v1OrgsChartDetail(orgID)
      return res.data as Chart
    },
    enabled: !!orgID && (options?.enabled ?? true),
  })
}

export function useEnsureWorkerChat() {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async (workerId: string) => {
      const res = await api.getApiClient().v1OrgsWorkersChatCreate(workerId, orgID)
      return res.data as ApiWorkerChatDTO & { agent_app_id: string }
    },
    onSuccess: (_data, workerId) => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.worker(orgID, workerId) })
    },
  })
}

export function useListHelixOrgWorkers(options?: { enabled?: boolean }) {
  const api = useApi()
  const { orgID } = useHelixOrgBase()
  return useQuery({
    queryKey: QUERY_KEYS.workers(orgID),
    queryFn: async () => {
      const res = await api.getApiClient().v1OrgsWorkersDetail(orgID)
      return (res.data ?? []) as WorkerDTO[]
    },
    enabled: !!orgID && (options?.enabled ?? true),
  })
}

export function useHelixOrgWorker(workerId: string | undefined, options?: { enabled?: boolean }) {
  const api = useApi()
  const { orgID } = useHelixOrgBase()
  return useQuery({
    queryKey: QUERY_KEYS.worker(orgID, workerId ?? ''),
    queryFn: async () => {
      if (!workerId) return null
      const res = await api.getApiClient().v1OrgsWorkersDetail2(workerId, orgID)
      return res.data as WorkerDetailDTO
    },
    enabled: !!orgID && !!workerId && (options?.enabled ?? true),
  })
}

export function useListHelixOrgTools(options?: { enabled?: boolean }) {
  const api = useApi()
  const { orgID } = useHelixOrgBase()
  return useQuery({
    queryKey: QUERY_KEYS.tools(orgID),
    queryFn: async () => {
      const res = await api.getApiClient().v1OrgsToolsDetail(orgID)
      return (res.data ?? []) as ToolDTO[]
    },
    staleTime: 5 * 60 * 1000,
    enabled: !!orgID && (options?.enabled ?? true),
  })
}

export function useListHelixOrgRoles(options?: { enabled?: boolean }) {
  const api = useApi()
  const { orgID } = useHelixOrgBase()
  return useQuery({
    queryKey: QUERY_KEYS.roles(orgID),
    queryFn: async () => {
      const res = await api.getApiClient().v1OrgsRolesDetail(orgID)
      return (res.data ?? []) as RoleDTO[]
    },
    enabled: !!orgID && (options?.enabled ?? true),
  })
}

export function useHelixOrgRole(roleId: string | undefined, options?: { enabled?: boolean }) {
  const api = useApi()
  const { orgID } = useHelixOrgBase()
  return useQuery({
    queryKey: QUERY_KEYS.role(orgID, roleId ?? ''),
    queryFn: async () => {
      if (!roleId) return null
      const res = await api.getApiClient().v1OrgsRolesDetail2(orgID, roleId)
      return res.data as RoleDTO
    },
    enabled: !!orgID && !!roleId && (options?.enabled ?? true),
  })
}

export function useHireHelixOrgWorker() {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async (payload: HireWorkerRequest) => {
      const res = await api.getApiClient().v1OrgsWorkersCreate(orgID, payload as ApiHireWorkerRequest)
      return res.data as HireWorkerResponse
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.chart(orgID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.workers(orgID) })
    },
  })
}

export function useFireHelixOrgWorker() {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async (workerId: string) => {
      await api.getApiClient().v1OrgsWorkersDelete(workerId, orgID)
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.chart(orgID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.workers(orgID) })
    },
  })
}

export function useUpdateHelixOrgRole() {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async (payload: { id: string; content?: string; tools?: string[]; streams?: string[] }) => {
      const { id, ...body } = payload
      await api.getApiClient().v1OrgsRolesUpdate(orgID, id, body)
    },
    onSuccess: (_data, payload) => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.chart(orgID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.roles(orgID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.role(orgID, payload.id) })
    },
  })
}

export function useCreateHelixOrgRole() {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async (payload: CreateRoleRequest) => {
      await api.getApiClient().v1OrgsRolesCreate(orgID, payload)
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.chart(orgID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.roles(orgID) })
    },
  })
}

export function useCreateHelixOrgPosition() {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async (payload: CreatePositionRequest) => {
      await api.getApiClient().v1OrgsPositionsCreate(orgID, payload)
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.chart(orgID) })
    },
  })
}

export function useUpdateHelixOrgPosition() {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async (payload: { id: string; parent_id?: string; role_id?: string }) => {
      const { id, ...body } = payload
      await api.getApiClient().v1OrgsPositionsUpdate(orgID, id, body)
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.chart(orgID) })
    },
  })
}

export function useDeleteHelixOrgRole() {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async (roleId: string) => {
      await api.getApiClient().v1OrgsRolesDelete(orgID, roleId)
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.chart(orgID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.roles(orgID) })
    },
  })
}

export function useDeleteHelixOrgPosition() {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async (positionId: string) => {
      await api.getApiClient().v1OrgsPositionsDelete(orgID, positionId)
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.chart(orgID) })
    },
  })
}

export function useHelixOrgSettings(options?: { enabled?: boolean }) {
  const api = useApi()
  const { orgID } = useHelixOrgBase()
  return useQuery({
    queryKey: QUERY_KEYS.settings(orgID),
    queryFn: async () => {
      const res = await api.getApiClient().v1OrgsSettingsDetail(orgID)
      return res.data as SettingsResponse
    },
    enabled: !!orgID && (options?.enabled ?? true),
  })
}

export function useSetHelixOrgSetting() {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async (payload: { key: string; value: string }) => {
      await api.getApiClient().v1OrgsSettingsUpdate(payload.key, orgID, { value: payload.value })
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.settings(orgID) })
    },
  })
}

export function useDeleteHelixOrgSetting() {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async (key: string) => {
      await api.getApiClient().v1OrgsSettingsDelete(key, orgID)
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.settings(orgID) })
    },
  })
}

// /api/v1/providers and /v1/models are not currently in the generated
// HelixOrg-tagged client surface; left on raw api.get for now.
export function useHelixProviders(options?: { enabled?: boolean }) {
  const api = useApi()
  return useQuery({
    queryKey: QUERY_KEYS.providers(),
    queryFn: async () => {
      const data = await api.get<string[]>('/api/v1/providers')
      return data ?? []
    },
    staleTime: 5 * 60 * 1000,
    enabled: options?.enabled ?? true,
  })
}

export function useHelixModelsForProvider(provider: string | undefined, options?: { enabled?: boolean }) {
  const api = useApi()
  return useQuery({
    queryKey: QUERY_KEYS.modelsForProvider(provider ?? ''),
    queryFn: async () => {
      if (!provider) return [] as HelixModelInfo[]
      const data = await api.get<{ data: HelixModelInfo[] }>(`/v1/models?provider=${encodeURIComponent(provider)}`)
      return data?.data ?? []
    },
    staleTime: 5 * 60 * 1000,
    enabled: !!provider && (options?.enabled ?? true),
  })
}

export function useListHelixOrgStreams(options?: { enabled?: boolean }) {
  const api = useApi()
  const { orgID } = useHelixOrgBase()
  return useQuery({
    queryKey: QUERY_KEYS.streams(orgID),
    queryFn: async () => {
      const res = await api.getApiClient().v1OrgsStreamsDetail(orgID)
      return res.data as StreamsResponse
    },
    enabled: !!orgID && (options?.enabled ?? true),
  })
}

export function useHelixOrgStream(streamId: string | undefined, options?: { enabled?: boolean }) {
  const api = useApi()
  const { orgID } = useHelixOrgBase()
  return useQuery({
    queryKey: QUERY_KEYS.stream(orgID, streamId ?? ''),
    queryFn: async () => {
      if (!streamId) return null
      const res = await api.getApiClient().v1OrgsStreamsDetail2(streamId, orgID)
      return res.data as StreamDTO
    },
    enabled: !!orgID && !!streamId && (options?.enabled ?? true),
  })
}

export function useCreateHelixOrgStream() {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async (payload: CreateStreamRequest) => {
      const res = await api.getApiClient().v1OrgsStreamsCreate(orgID, payload)
      return res.data as StreamDTO
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.streams(orgID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.chart(orgID) })
    },
  })
}

// Probes "is GitHub connected?" — must stay quiet on failure (the
// caller renders the disabled-transport hint, not a toast). The
// generated client throws on non-2xx, so we swallow here.
export function useListGitHubRepos(options?: { enabled?: boolean }) {
  const api = useApi()
  const { orgID } = useHelixOrgBase()
  return useQuery({
    queryKey: ['helix-org', 'github-repos', orgID],
    queryFn: async () => {
      try {
        const res = await api.getApiClient().v1OrgsGithubReposDetail(orgID)
        return res.data as GitHubReposResponse
      } catch {
        return null
      }
    },
    enabled: !!orgID && (options?.enabled ?? true),
    staleTime: 0,
    refetchOnMount: 'always',
  })
}

// Throws InstallWebhookFailedError on non-2xx so callers can detect
// the "snackbar already shown" sentinel and skip their own toast.
export function useInstallGitHubWebhook() {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async (streamId: string) => {
      try {
        const res = await api.getApiClient().v1OrgsStreamsGithubInstallWebhookCreate(streamId, orgID)
        return res.data as InstallGitHubWebhookResponse
      } catch (e: any) {
        const msg = e?.response?.data?.error ?? e?.message ?? 'install webhook failed'
        const failed = new InstallWebhookFailedError(msg)
        throw failed
      }
    },
    onSuccess: (_data, streamId) => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.stream(orgID, streamId) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.streams(orgID) })
    },
  })
}

export class InstallWebhookFailedError extends Error {
  constructor(message = 'install webhook failed') {
    super(message)
    this.name = 'InstallWebhookFailedError'
  }
}

export function useUpdateHelixOrgStream() {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async ({ streamId, payload }: { streamId: string; payload: UpdateStreamRequest }) => {
      const res = await api.getApiClient().v1OrgsStreamsUpdate(streamId, orgID, payload)
      return res.data as StreamDTO
    },
    onSuccess: (_data, vars) => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.streams(orgID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.stream(orgID, vars.streamId) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.chart(orgID) })
    },
  })
}

export function useDeleteHelixOrgStream() {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async (streamId: string) => {
      await api.getApiClient().v1OrgsStreamsDelete(streamId, orgID)
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.streams(orgID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.chart(orgID) })
    },
  })
}

export function useListPositionSubscriptions(positionID: string | undefined, options?: { enabled?: boolean }) {
  const api = useApi()
  const { orgID } = useHelixOrgBase()
  return useQuery({
    queryKey: QUERY_KEYS.positionSubs(orgID, positionID ?? ''),
    queryFn: async () => {
      if (!positionID) return null
      const res = await api.getApiClient().v1OrgsPositionsSubscriptionsDetail(positionID, orgID)
      return res.data as PositionSubscriptionsResponse
    },
    enabled: !!orgID && !!positionID && (options?.enabled ?? true),
  })
}

export function useSubscribePosition(positionID: string | undefined) {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async (streamID: string) => {
      if (!positionID) throw new Error('positionID is required to subscribe')
      const res = await api.getApiClient().v1OrgsPositionsSubscriptionsCreate(positionID, orgID, { stream_id: streamID })
      return res.data as PositionSubscription
    },
    onSuccess: () => {
      if (positionID) {
        qc.invalidateQueries({ queryKey: QUERY_KEYS.positionSubs(orgID, positionID) })
      }
      qc.invalidateQueries({ queryKey: QUERY_KEYS.streams(orgID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.chart(orgID) })
    },
  })
}

export function useUnsubscribePosition(positionID: string | undefined) {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async (streamID: string) => {
      if (!positionID) throw new Error('positionID is required to unsubscribe')
      await api.getApiClient().v1OrgsPositionsSubscriptionsDelete(positionID, streamID, orgID)
    },
    onSuccess: () => {
      if (positionID) {
        qc.invalidateQueries({ queryKey: QUERY_KEYS.positionSubs(orgID, positionID) })
      }
      qc.invalidateQueries({ queryKey: QUERY_KEYS.streams(orgID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.chart(orgID) })
    },
  })
}
