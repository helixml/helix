import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import useApi from '../hooks/useApi'
import useRouter from '../hooks/useRouter'
import {
  ApiCreateRoleRequest,
  ApiCreateStreamRequest,
  ApiEventCard,
  ApiGitHubReposResponse,
  ApiGitHubInstallationStatus,
  ApiHireWorkerRequest,
  ApiHireWorkerResponse,
  ApiInstallGitHubWebhookResponse,
  ApiOrgOverview,
  ApiRoleDTO,
  ApiRoleGroup,
  ApiSettingsResponse,
  ApiSettingsSpecDTO,
  ApiStreamDTO,
  ApiStreamsResponse,
  ApiToolDTO,
  ApiUpdateStreamRequest,
  ApiWorkerActivateDTO,
  ApiWorkerBadge,
  ApiWorkerChatDTO,
  ApiWorkerDTO,
  ApiWorkerDetailDTO,
  ApiWorkerSubscriptionDTO,
  ApiWorkerSubscriptionsResponse,
} from '../api/api'

// Re-exported aliases. Generated Api* types mark every field
// optional; consumers use them as if fields are present. strict
// null checks are off project-wide so plain aliases suffice.
export type WorkerBadge = ApiWorkerBadge
export type RoleDTO = ApiRoleDTO
export type WorkerDTO = ApiWorkerDTO
export type WorkerDetailDTO = ApiWorkerDetailDTO
export type ToolDTO = ApiToolDTO
export type StreamDTO = ApiStreamDTO
export type EventCard = ApiEventCard
export type SettingsSpecDTO = ApiSettingsSpecDTO
export type SettingsResponse = ApiSettingsResponse
export type StreamsResponse = ApiStreamsResponse
export type GitHubRepoDTO = NonNullable<ApiGitHubReposResponse['repos']>[number]
export type GitHubReposResponse = ApiGitHubReposResponse
export type GitHubInstallationStatus = ApiGitHubInstallationStatus
export type InstallGitHubWebhookResponse = ApiInstallGitHubWebhookResponse
export type WorkerSubscription = ApiWorkerSubscriptionDTO
export type WorkerSubscriptionsResponse = ApiWorkerSubscriptionsResponse
export type OrgOverview = ApiOrgOverview
export type RoleGroup = ApiRoleGroup

export interface HireWorkerRequest extends Omit<ApiHireWorkerRequest, 'kind'> {
  role_id: string
  parent_id?: string
  kind: 'human' | 'ai'
  identity_content: string
}
export type HireWorkerResponse = ApiHireWorkerResponse
export type CreateRoleRequest = ApiCreateRoleRequest & { id: string; content: string }
export type CreateStreamRequest = ApiCreateStreamRequest & { name: string }
export type UpdateStreamRequest = ApiUpdateStreamRequest

export interface HelixModelInfo {
  id: string
  name?: string
  description?: string
  context_length?: number
}

export const QUERY_KEYS = {
  overview: (orgID: string) => ['helix-org', orgID, 'overview'] as const,
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
  workerSubs: (orgID: string, workerID: string) => ['helix-org', orgID, 'workers', workerID, 'subscriptions'] as const,
}

export function useHelixOrgBase(): { base: string; orgID: string } {
  const { params } = useRouter()
  const orgID = (params.org_id as string) || ''
  const base = orgID ? `/api/v1/orgs/${encodeURIComponent(orgID)}` : ''
  return { base, orgID }
}

export function useHelixOrgOverview(options?: { enabled?: boolean }) {
  const api = useApi()
  const { orgID } = useHelixOrgBase()
  return useQuery({
    queryKey: QUERY_KEYS.overview(orgID),
    queryFn: async () => {
      const res = await api.getApiClient().v1OrgsOverviewDetail(orgID)
      return res.data as OrgOverview
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

// useActivateWorker manually triggers an activation for a Worker.
// Wired to the worker page's "Start Desktop" button so the click goes
// through the full activation pipeline (ensureProject → AttachHelixOrgMCP
// → ensureSession → container start) instead of the generic
// /sessions/{id}/resume — which doesn't re-attach the helix-org MCP
// and so leaves the desktop without the org-graph tools.
//
// The accepts an orgID override so callers that aren't running inside
// the helix-org base context (e.g. TeamDesktopPage opened in a new
// tab from the worker detail page) can pass the org slug explicitly.
export function useActivateWorker(orgIDOverride?: string) {
  const api = useApi()
  const { orgID: baseOrgID } = useHelixOrgBase()
  const orgID = orgIDOverride ?? baseOrgID
  return useMutation({
    mutationFn: async (workerId: string) => {
      const res = await api.getApiClient().v1OrgsWorkersActivateCreate(workerId, orgID)
      return res.data as ApiWorkerActivateDTO
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
      qc.invalidateQueries({ queryKey: QUERY_KEYS.overview(orgID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.workers(orgID) })
      // An AI hire mints its s-activations-<id> stream — refresh the
      // Streams list / chart stream nodes so it shows without a reload.
      qc.invalidateQueries({ queryKey: QUERY_KEYS.streams(orgID) })
    },
  })
}

// useAddWorkerParent adds a reporting line — the Worker now also
// reports to parentID. Reporting is many-to-many, so this is additive.
// Drives the chart's drag-to-report: dragging manager → subordinate
// adds the line. The topology reconciler wires the comms channels the
// edge implies (the manager's s-team-<mgr> stream and the pair's
// s-dm-<pair> channel, plus the manager observing the report's
// activation stream), so we refresh streams too — not just the worker
// list — so those new nodes render without a reload.
export function useAddWorkerParent() {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async ({ workerID, parentID }: { workerID: string; parentID: string }) => {
      await api.getApiClient().v1OrgsWorkersParentsCreate(workerID, orgID, { parent_id: parentID })
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.overview(orgID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.workers(orgID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.streams(orgID) })
    },
  })
}

// useRemoveWorkerParent drops one reporting line — the Worker no longer
// reports to parentID. Drives the chart's delete-edge flow; only the
// dragged edge's line is removed, leaving any other managers intact.
// The reconciler tears down the channels the edge implied (the manager's
// team stream when its last report leaves, and the pair's DM channel),
// so refresh streams as well as the worker list.
export function useRemoveWorkerParent() {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async ({ workerID, parentID }: { workerID: string; parentID: string }) => {
      await api.getApiClient().v1OrgsWorkersParentsDelete(workerID, parentID, orgID)
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.overview(orgID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.workers(orgID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.streams(orgID) })
    },
  })
}

// useSubscribeWorkerAtChart drives the chart's drag-to-subscribe flow.
// Each call carries its own (workerID, streamID) because the chart
// wires arbitrary Workers to arbitrary streams; useSubscribeWorker is
// bound to a single workerID and so doesn't fit the canvas's onConnect.
export function useSubscribeWorkerAtChart() {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async ({ workerID, streamID }: { workerID: string; streamID: string }) => {
      await api.getApiClient().v1OrgsWorkersSubscriptionsCreate(workerID, orgID, { stream_id: streamID })
    },
    onSuccess: (_data, { workerID }) => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.workerSubs(orgID, workerID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.streams(orgID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.overview(orgID) })
    },
  })
}

// useUnsubscribeWorkerAtChart is the chart-scoped counterpart of
// useSubscribeWorkerAtChart — it drops a (worker, stream) subscription
// when the user deletes a subscription edge on the canvas.
export function useUnsubscribeWorkerAtChart() {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async ({ workerID, streamID }: { workerID: string; streamID: string }) => {
      await api.getApiClient().v1OrgsWorkersSubscriptionsDelete(workerID, streamID, orgID)
    },
    onSuccess: (_data, { workerID }) => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.workerSubs(orgID, workerID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.streams(orgID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.overview(orgID) })
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
    onSuccess: (_data, workerId) => {
      // Evict the fired worker's own queries (the worker key prefix-
      // matches its subscriptions key, so this drops both) and cancel
      // any in-flight fetch. Without this the worker detail page would
      // refetch a now-deleted worker and log a 404 (the QA F3 finding).
      qc.removeQueries({ queryKey: QUERY_KEYS.worker(orgID, workerId) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.overview(orgID) })
      // Exact: refresh the list itself without prefix-matching (and so
      // refetching) the worker/subscriptions queries we just removed.
      qc.invalidateQueries({ queryKey: QUERY_KEYS.workers(orgID), exact: true })
      // Firing cascades away the worker's s-activations-<id> stream and
      // its direct reports' parent edge — refresh the Streams list (QA
      // F6) and any open stream detail.
      qc.invalidateQueries({ queryKey: QUERY_KEYS.streams(orgID) })
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
      qc.invalidateQueries({ queryKey: QUERY_KEYS.overview(orgID) })
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
      qc.invalidateQueries({ queryKey: QUERY_KEYS.overview(orgID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.roles(orgID) })
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
      qc.invalidateQueries({ queryKey: QUERY_KEYS.overview(orgID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.roles(orgID) })
      // Deleting a role fires every Worker holding it, which tears down
      // their activation streams — refresh both lists so neither shows
      // ghost rows (QA F6).
      qc.invalidateQueries({ queryKey: QUERY_KEYS.workers(orgID), exact: true })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.streams(orgID) })
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
      qc.invalidateQueries({ queryKey: QUERY_KEYS.overview(orgID) })
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

// Probes "is the Helix GitHub App installed for this org?" — drives the
// New Stream "Install Helix" gate. Quiet on failure (returns null) so the
// dialog renders the install CTA rather than a toast.
export function useGitHubAppInstallation(options?: { enabled?: boolean }) {
  const api = useApi()
  const { orgID } = useHelixOrgBase()
  return useQuery({
    queryKey: ['helix-org', 'github-app-installation', orgID],
    queryFn: async () => {
      try {
        const res = await api.getApiClient().v1OrgsGithubAppInstallationDetail(orgID)
        return res.data as GitHubInstallationStatus
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
      qc.invalidateQueries({ queryKey: QUERY_KEYS.overview(orgID) })
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
      qc.invalidateQueries({ queryKey: QUERY_KEYS.overview(orgID) })
    },
  })
}

export function useListWorkerSubscriptions(workerID: string | undefined, options?: { enabled?: boolean }) {
  const api = useApi()
  const { orgID } = useHelixOrgBase()
  return useQuery({
    queryKey: QUERY_KEYS.workerSubs(orgID, workerID ?? ''),
    queryFn: async () => {
      if (!workerID) return null
      const res = await api.getApiClient().v1OrgsWorkersSubscriptionsDetail(workerID, orgID)
      return res.data as WorkerSubscriptionsResponse
    },
    enabled: !!orgID && !!workerID && (options?.enabled ?? true),
  })
}

export function useSubscribeWorker(workerID: string | undefined) {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async (streamID: string) => {
      if (!workerID) throw new Error('workerID is required to subscribe')
      const res = await api.getApiClient().v1OrgsWorkersSubscriptionsCreate(workerID, orgID, { stream_id: streamID })
      return res.data as WorkerSubscription
    },
    onSuccess: () => {
      if (workerID) {
        qc.invalidateQueries({ queryKey: QUERY_KEYS.workerSubs(orgID, workerID) })
      }
      qc.invalidateQueries({ queryKey: QUERY_KEYS.streams(orgID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.overview(orgID) })
    },
  })
}

export function useUnsubscribeWorker(workerID: string | undefined) {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async (streamID: string) => {
      if (!workerID) throw new Error('workerID is required to unsubscribe')
      await api.getApiClient().v1OrgsWorkersSubscriptionsDelete(workerID, streamID, orgID)
    },
    onSuccess: () => {
      if (workerID) {
        qc.invalidateQueries({ queryKey: QUERY_KEYS.workerSubs(orgID, workerID) })
      }
      qc.invalidateQueries({ queryKey: QUERY_KEYS.streams(orgID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.overview(orgID) })
    },
  })
}
