import { useQuery, useQueries, useMutation, useQueryClient } from '@tanstack/react-query'
import useApi from '../hooks/useApi'
import useRouter from '../hooks/useRouter'
import {
  ApiCreateRoleRequest,
  ApiCreateTopicRequest,
  ApiEventCard,
  ApiGitHubReposResponse,
  ApiGitHubInstallationStatus,
  ApiGitHubManifestStartResponse,
  ApiHireWorkerRequest,
  ApiHireWorkerResponse,
  ApiInstallGitHubWebhookResponse,
  ApiGitHubWebhookStatusResponse,
  ApiOrgOverview,
  ApiRoleDTO,
  ApiRoleGroup,
  ApiSettingsResponse,
  ApiSettingsSpecDTO,
  ApiTopicDTO,
  ApiTopicsResponse,
  ApiToolDTO,
  ApiUpdateTopicRequest,
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
export type TopicDTO = ApiTopicDTO
export type EventCard = ApiEventCard
export type SettingsSpecDTO = ApiSettingsSpecDTO
export type SettingsResponse = ApiSettingsResponse
export type TopicsResponse = ApiTopicsResponse
export type GitHubRepoDTO = NonNullable<ApiGitHubReposResponse['repos']>[number]
export type GitHubReposResponse = ApiGitHubReposResponse
export type GitHubInstallationStatus = ApiGitHubInstallationStatus
export type GitHubManifestStartResponse = ApiGitHubManifestStartResponse
export type InstallGitHubWebhookResponse = ApiInstallGitHubWebhookResponse
export type GitHubWebhookStatusResponse = ApiGitHubWebhookStatusResponse
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
export type CreateTopicRequest = ApiCreateTopicRequest & { name: string }
export type UpdateTopicRequest = ApiUpdateTopicRequest

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
  topics: (orgID: string) => ['helix-org', orgID, 'topics'] as const,
  topic: (orgID: string, id: string) => ['helix-org', orgID, 'topics', id] as const,
  webhookStatus: (orgID: string, id: string) => ['helix-org', orgID, 'topics', id, 'webhook-status'] as const,
  topicMessageCount: (orgID: string, id: string) => ['helix-org', orgID, 'topics', id, 'message-count'] as const,
  workerSubs: (orgID: string, workerID: string) => ['helix-org', orgID, 'workers', workerID, 'subscriptions'] as const,
  processors: (orgID: string) => ['helix-org', orgID, 'processors'] as const,
  processor: (orgID: string, id: string) => ['helix-org', orgID, 'processors', id] as const,
}

// ---- Processors ---------------------------------------------------------
// A Processor is a transform/filter node interposed on the edge between a
// Topic and its subscribers. Its REST surface is JSON:API, so the hooks
// flatten {data:{id,attributes}} resources into flat ProcessorDTOs.

export interface ProcessorOutput {
  topic_id: string
  match?: string
  label?: string
  owned?: boolean
}

export interface ProcessorDTO {
  id: string
  name: string
  input_topic_id: string
  kind: string
  config?: Record<string, unknown>
  outputs: ProcessorOutput[]
  created_by?: string
  created_at?: string
}

interface JsonApiResource<T> { id: string; type: string; attributes: T }
interface JsonApiDoc<T> { data: T }

function flattenProcessor(res: JsonApiResource<Omit<ProcessorDTO, 'id'>>): ProcessorDTO {
  return { id: res.id, ...res.attributes }
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

// useRestartWorkerAgent recreates the worker's desktop container from
// scratch. Wired to the worker page's "Restart agent session" button.
// Unlike useActivateWorker (which continues the existing session via
// SendMessage and so can't recover a stuck container), this hits the
// dedicated worker restart endpoint, which resolves the worker's session
// and delegates to the shared backend restart primitive (StopDesktop →
// recreate → reset crashed prompts), falling back to a fresh activation
// when the worker has no live session.
export function useRestartWorkerAgent(orgIDOverride?: string) {
  const api = useApi()
  const { orgID: baseOrgID } = useHelixOrgBase()
  const orgID = orgIDOverride ?? baseOrgID
  return useMutation({
    mutationFn: async (workerId: string) => {
      const res = await api.getApiClient().v1OrgsWorkersRestartAgentCreate(workerId, orgID)
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

// useUpdateWorkerIdentity rewrites a Worker's identity markdown. The
// Spawner projects the new content into the Worker's identity.md on the
// next activation. Drives the editable Identity panel on the worker
// detail page.
export function useUpdateWorkerIdentity() {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async ({ workerId, identity }: { workerId: string; identity: string }) => {
      await api.getApiClient().v1OrgsWorkersIdentityCreate(workerId, orgID, { identity })
    },
    onSuccess: (_data, vars) => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.worker(orgID, vars.workerId) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.workers(orgID) })
    },
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
      // An AI hire mints its s-transcript-<id> topic — refresh the
      // Topics list / chart topic nodes so it shows without a reload.
      qc.invalidateQueries({ queryKey: QUERY_KEYS.topics(orgID) })
    },
  })
}

// useAddWorkerParent adds a reporting line — the Worker now also
// reports to parentID. Reporting is many-to-many, so this is additive.
// Drives the chart's drag-to-report: dragging manager → subordinate
// adds the line. The topology reconciler wires the comms channels the
// edge implies (the manager's s-team-<mgr> topic and the pair's
// s-dm-<pair> channel, plus the manager observing the report's
// transcript), so we refresh topics too — not just the worker
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
      qc.invalidateQueries({ queryKey: QUERY_KEYS.topics(orgID) })
    },
  })
}

// useRemoveWorkerParent drops one reporting line — the Worker no longer
// reports to parentID. Drives the chart's delete-edge flow; only the
// dragged edge's line is removed, leaving any other managers intact.
// The reconciler tears down the channels the edge implied (the manager's
// team topic when its last report leaves, and the pair's DM channel),
// so refresh topics as well as the worker list.
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
      qc.invalidateQueries({ queryKey: QUERY_KEYS.topics(orgID) })
    },
  })
}

// useSubscribeWorkerAtChart drives the chart's drag-to-subscribe flow.
// Each call carries its own (workerID, topicID) because the chart
// wires arbitrary Workers to arbitrary topics; useSubscribeWorker is
// bound to a single workerID and so doesn't fit the canvas's onConnect.
export function useSubscribeWorkerAtChart() {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async ({ workerID, topicID }: { workerID: string; topicID: string }) => {
      await api.getApiClient().v1OrgsWorkersSubscriptionsCreate(workerID, orgID, { topic_id: topicID })
    },
    onSuccess: (_data, { workerID }) => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.workerSubs(orgID, workerID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.topics(orgID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.overview(orgID) })
    },
  })
}

// useUnsubscribeWorkerAtChart is the chart-scoped counterpart of
// useSubscribeWorkerAtChart — it drops a (worker, topic) subscription
// when the user deletes a subscription edge on the canvas.
export function useUnsubscribeWorkerAtChart() {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async ({ workerID, topicID }: { workerID: string; topicID: string }) => {
      await api.getApiClient().v1OrgsWorkersSubscriptionsDelete(workerID, topicID, orgID)
    },
    onSuccess: (_data, { workerID }) => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.workerSubs(orgID, workerID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.topics(orgID) })
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
      // Firing cascades away the worker's s-transcript-<id> topic and
      // its direct reports' parent edge — refresh the Topics list (QA
      // F6) and any open topic detail.
      qc.invalidateQueries({ queryKey: QUERY_KEYS.topics(orgID) })
    },
  })
}

export function useUpdateHelixOrgRole() {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async (payload: { id: string; content?: string; tools?: string[]; topics?: string[] }) => {
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
      // their transcripts — refresh both lists so neither shows
      // ghost rows (QA F6).
      qc.invalidateQueries({ queryKey: QUERY_KEYS.workers(orgID), exact: true })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.topics(orgID) })
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

export function useListHelixOrgTopics(options?: { enabled?: boolean }) {
  const api = useApi()
  const { orgID } = useHelixOrgBase()
  return useQuery({
    queryKey: QUERY_KEYS.topics(orgID),
    queryFn: async () => {
      const res = await api.getApiClient().v1OrgsTopicsDetail(orgID)
      return res.data as TopicsResponse
    },
    enabled: !!orgID && (options?.enabled ?? true),
  })
}

export function useHelixOrgTopic(topicId: string | undefined, options?: { enabled?: boolean }) {
  const api = useApi()
  const { orgID } = useHelixOrgBase()
  return useQuery({
    queryKey: QUERY_KEYS.topic(orgID, topicId ?? ''),
    queryFn: async () => {
      if (!topicId) return null
      const res = await api.getApiClient().v1OrgsTopicsDetail2(topicId, orgID)
      return res.data as TopicDTO
    },
    enabled: !!orgID && !!topicId && (options?.enabled ?? true),
  })
}

// useGitHubWebhookStatus reports the LIVE state of a github topic's repo
// webhook as seen on GitHub (state: "installed" | "missing" | "unknown"), so
// the detail page can link to the real hook or offer a re-install. Read-only;
// safe to refetch. Invalidate QUERY_KEYS.webhookStatus after an install.
export function useGitHubWebhookStatus(topicId: string | undefined, options?: { enabled?: boolean }) {
  const api = useApi()
  const { orgID } = useHelixOrgBase()
  return useQuery({
    queryKey: QUERY_KEYS.webhookStatus(orgID, topicId ?? ''),
    queryFn: async () => {
      if (!topicId) return null
      const res = await api.getApiClient().v1OrgsTopicsGithubWebhookStatusDetail(topicId, orgID)
      return res.data as GitHubWebhookStatusResponse
    },
    enabled: !!orgID && !!topicId && (options?.enabled ?? true),
  })
}

// useTopicMessageCount reports the total number of messages waiting on
// a single topic via the paginated JSON:API messages endpoint. We only
// need meta.total, so we request the smallest possible page (size 1) and
// ignore the body — the count is the cheap part the server computes
// independently of the page slice. Used by the topic detail metric card.
export function useTopicMessageCount(topicId: string | undefined, options?: { enabled?: boolean }) {
  const api = useApi()
  const { orgID } = useHelixOrgBase()
  return useQuery({
    queryKey: QUERY_KEYS.topicMessageCount(orgID, topicId ?? ''),
    queryFn: async () => {
      if (!topicId) return 0
      const res = await api.getApiClient().v1OrgsTopicsMessagesDetail(topicId, orgID, {
        'page[number]': 1,
        'page[size]': 1,
      })
      return res.data?.meta?.total ?? 0
    },
    enabled: !!orgID && !!topicId && (options?.enabled ?? true),
  })
}

// useTopicMessageCounts fans the same per-topic count query out across
// every topic id so the org chart can label each topic card. Returns a
// topicId → total map; missing/in-flight ids resolve to 0 (the caller
// renders 0 rather than flickering). One React Query per id keeps each
// count independently cached + invalidated, shared with the detail
// page's single-topic hook above (same query key).
export function useTopicMessageCounts(topicIds: string[], options?: { enabled?: boolean }): Record<string, number> {
  const api = useApi()
  const { orgID } = useHelixOrgBase()
  const enabled = !!orgID && (options?.enabled ?? true)
  const results = useQueries({
    queries: topicIds.map((id) => ({
      queryKey: QUERY_KEYS.topicMessageCount(orgID, id),
      queryFn: async () => {
        const res = await api.getApiClient().v1OrgsTopicsMessagesDetail(id, orgID, {
          'page[number]': 1,
          'page[size]': 1,
        })
        return res.data?.meta?.total ?? 0
      },
      enabled,
    })),
  })
  const counts: Record<string, number> = {}
  topicIds.forEach((id, i) => {
    counts[id] = (results[i]?.data as number | undefined) ?? 0
  })
  return counts
}

export function useCreateHelixOrgTopic() {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async (payload: CreateTopicRequest) => {
      const res = await api.getApiClient().v1OrgsTopicsCreate(orgID, payload)
      return res.data as TopicDTO
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.topics(orgID) })
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
// New Topic "Install Helix" gate. Quiet on failure (returns null) so the
// dialog renders the install CTA rather than a toast.
export function useGitHubAppInstallation(options?: { enabled?: boolean; pollWhileNotInstalled?: boolean }) {
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
    // Poll until installed: the GitHub popup's postMessage is severed by
    // GitHub's COOP headers, so polling is how the dialog reliably detects
    // create→install completing.
    refetchInterval: options?.pollWhileNotInstalled
      ? (query) => ((query.state.data as GitHubInstallationStatus | null)?.installed ? false : 4000)
      : false,
  })
}

// Starts the GitHub App Manifest flow: the backend returns the GitHub POST
// URL + a Helix-authored manifest + CSRF state, which the dialog submits as a
// form so GitHub creates the app on the user's behalf.
export function useGitHubManifestStart() {
  const api = useApi()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async (input: { github_org: string; origin: string }) => {
      const res = await api.getApiClient().v1OrgsGithubAppManifestCreate(orgID, { body: input } as any)
      return res.data as GitHubManifestStartResponse
    },
  })
}

// Throws InstallWebhookFailedError on non-2xx so callers can detect
// the "snackbar already shown" sentinel and skip their own toast.
export function useInstallGitHubWebhook() {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async (topicId: string) => {
      try {
        const res = await api.getApiClient().v1OrgsTopicsGithubInstallWebhookCreate(topicId, orgID)
        return res.data as InstallGitHubWebhookResponse
      } catch (e: any) {
        const msg = e?.response?.data?.error ?? e?.message ?? 'install webhook failed'
        const failed = new InstallWebhookFailedError(msg)
        throw failed
      }
    },
    onSuccess: (_data, topicId) => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.topic(orgID, topicId) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.topics(orgID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.webhookStatus(orgID, topicId) })
    },
  })
}

export class InstallWebhookFailedError extends Error {
  constructor(message = 'install webhook failed') {
    super(message)
    this.name = 'InstallWebhookFailedError'
  }
}

export function useUpdateHelixOrgTopic() {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async ({ topicId, payload }: { topicId: string; payload: UpdateTopicRequest }) => {
      const res = await api.getApiClient().v1OrgsTopicsUpdate(topicId, orgID, payload)
      return res.data as TopicDTO
    },
    onSuccess: (_data, vars) => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.topics(orgID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.topic(orgID, vars.topicId) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.overview(orgID) })
    },
  })
}

export function useDeleteHelixOrgTopic() {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async (topicId: string) => {
      await api.getApiClient().v1OrgsTopicsDelete(topicId, orgID)
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.topics(orgID) })
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
    mutationFn: async (topicID: string) => {
      if (!workerID) throw new Error('workerID is required to subscribe')
      const res = await api.getApiClient().v1OrgsWorkersSubscriptionsCreate(workerID, orgID, { topic_id: topicID })
      return res.data as WorkerSubscription
    },
    onSuccess: () => {
      if (workerID) {
        qc.invalidateQueries({ queryKey: QUERY_KEYS.workerSubs(orgID, workerID) })
      }
      qc.invalidateQueries({ queryKey: QUERY_KEYS.topics(orgID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.overview(orgID) })
    },
  })
}

export function useUnsubscribeWorker(workerID: string | undefined) {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async (topicID: string) => {
      if (!workerID) throw new Error('workerID is required to unsubscribe')
      await api.getApiClient().v1OrgsWorkersSubscriptionsDelete(workerID, topicID, orgID)
    },
    onSuccess: () => {
      if (workerID) {
        qc.invalidateQueries({ queryKey: QUERY_KEYS.workerSubs(orgID, workerID) })
      }
      qc.invalidateQueries({ queryKey: QUERY_KEYS.topics(orgID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.overview(orgID) })
    },
  })
}

// useTopicSampleMessage fetches the single most recent REAL message on a
// topic (newest first, page size 1) so the processor drawer can show
// "what a message on this topic looks like". Returns null when the topic
// has no messages — no synthetic/fake data.
export function useTopicSampleMessage(topicId: string | undefined, options?: { enabled?: boolean }) {
  const api = useApi()
  const { orgID } = useHelixOrgBase()
  return useQuery({
    queryKey: ['helix-org', orgID, 'topics', topicId ?? '', 'sample-message'] as const,
    queryFn: async () => {
      if (!topicId) return null
      const res = await api.getApiClient().v1OrgsTopicsMessagesDetail(topicId, orgID, {
        'page[number]': 1,
        'page[size]': 1,
      })
      const doc = res.data as unknown as { data?: { attributes?: { from?: string; subject?: string; body?: string } }[] }
      const first = doc.data?.[0]
      return first?.attributes ?? null
    },
    enabled: !!orgID && !!topicId && (options?.enabled ?? true),
  })
}

export function useListHelixOrgProcessors(options?: { enabled?: boolean }) {
  const api = useApi()
  const { orgID } = useHelixOrgBase()
  return useQuery({
    queryKey: QUERY_KEYS.processors(orgID),
    queryFn: async () => {
      const res = await api.getApiClient().v1OrgsProcessorsDetail(orgID)
      const doc = res.data as unknown as { data: JsonApiResource<Omit<ProcessorDTO, 'id'>>[] }
      return (doc.data ?? []).map(flattenProcessor)
    },
    enabled: !!orgID && (options?.enabled ?? true),
  })
}

export function useHelixOrgProcessor(id: string | undefined, options?: { enabled?: boolean }) {
  const api = useApi()
  const { orgID } = useHelixOrgBase()
  return useQuery({
    queryKey: QUERY_KEYS.processor(orgID, id ?? ''),
    queryFn: async () => {
      if (!id) return null
      const res = await api.getApiClient().v1OrgsProcessorsDetail2(orgID, id)
      const doc = res.data as unknown as JsonApiDoc<JsonApiResource<Omit<ProcessorDTO, 'id'>>>
      return flattenProcessor(doc.data)
    },
    enabled: !!orgID && !!id && (options?.enabled ?? true),
  })
}

export interface ProcessorWriteAttrs {
  name: string
  input_topic_id?: string
  kind: string
  config?: Record<string, unknown>
  created_by?: string
  outputs?: { topic_id?: string; match?: string; label?: string }[]
}

export function useCreateHelixOrgProcessor() {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async (attrs: ProcessorWriteAttrs) => {
      const res = await api.getApiClient().v1OrgsProcessorsCreate(orgID, {
        data: { type: 'processors', attributes: attrs },
      })
      const doc = res.data as unknown as JsonApiDoc<JsonApiResource<Omit<ProcessorDTO, 'id'>>>
      return flattenProcessor(doc.data)
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.processors(orgID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.topics(orgID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.overview(orgID) })
    },
  })
}

export function useUpdateHelixOrgProcessor() {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async ({ id, attrs }: { id: string; attrs: ProcessorWriteAttrs }) => {
      const res = await api.getApiClient().v1OrgsProcessorsUpdate(orgID, id, {
        data: { type: 'processors', attributes: attrs },
      })
      const doc = res.data as unknown as JsonApiDoc<JsonApiResource<Omit<ProcessorDTO, 'id'>>>
      return flattenProcessor(doc.data)
    },
    onSuccess: (_data, vars) => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.processors(orgID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.processor(orgID, vars.id) })
    },
  })
}

export function useDeleteHelixOrgProcessor() {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async (id: string) => {
      await api.getApiClient().v1OrgsProcessorsDelete(orgID, id)
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.processors(orgID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.topics(orgID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.overview(orgID) })
    },
  })
}

