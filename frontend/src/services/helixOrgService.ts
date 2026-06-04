// React Query hooks for the helix-org chart page. The chart is the
// only React surface helix-org exposes today; the rest of the
// org-graph (workers, roles, positions, streams, settings) is driven
// through MCP tools or the JSON REST API. Routes are scoped by
// :org_id captured from the URL by useHelixOrgBase().
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import useApi from '../hooks/useApi'
import useRouter from '../hooks/useRouter'

// ---- Wire types ----------------------------------------------------------

export interface WorkerBadge {
  id: string
  kind: string
}

export interface ChartNode {
  position_id: string
  role_id: string
  parent_id?: string
  workers?: WorkerBadge[]
  children?: ChartNode[]
}

export interface RoleBadge {
  id: string
}

export interface Chart {
  roots: ChartNode[]
  roles?: RoleBadge[]
}

export interface CreateRoleRequest {
  id: string
  content: string
  tools?: string[]
  streams?: string[]
}

export interface CreatePositionRequest {
  id: string
  role_id: string
  parent_id?: string
}

export interface PositionDTO {
  id: string
  role_id: string
  parent_id?: string
}

export interface RoleDTO {
  id: string
  content: string
  tools?: string[]
  streams?: string[]
  created_at?: string
  updated_at?: string
}

export interface WorkerDTO {
  id: string
  kind: string
  position_id?: string
  identity_content: string
  organization_id?: string
  tools?: string[]
}

export interface WorkerDetailDTO {
  worker: WorkerDTO
  role?: RoleDTO
  position?: PositionDTO
  // AgentAppID is the Helix agent app the Worker chats through. Empty
  // until the Worker has been activated at least once. The Worker
  // detail page disables the "Chat" button when missing.
  agent_app_id?: string
  // ProjectID is the Helix project that owns the per-Worker agent app.
  // The Worker detail page deep-links the chat button to the project's
  // Human Desktop session rather than the bare agent app.
  project_id?: string
}

export interface HireGrantInput {
  tool_name: string
}

export interface HireWorkerRequest {
  id?: string
  position_id: string
  kind: 'human' | 'ai'
  identity_content: string
  grants?: HireGrantInput[]
}

export interface HireWorkerResponse {
  id: string
  activation_id?: string
}

// ---- Query keys ----------------------------------------------------------

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
  // Position subscriptions are the canonical surface for "what
  // streams does this slot consume". The worker detail page resolves
  // worker → position then loads via this key.
  positionSubs: (orgID: string, positionID: string) => ['helix-org', orgID, 'positions', positionID, 'subscriptions'] as const,
}

export interface EventCard {
  id: string
  stream_id: string
  source?: string
  created_at: string
  body: string
  has_message: boolean
  from?: string
  to?: string
  subject?: string
  message_body?: string
}

export interface StreamDTO {
  id: string
  name: string
  description?: string
  kind: string
  created_by: string
  created_at: string
  // Subscribers are POSITION IDs subscribed to this stream
  // (subscriptions are position-anchored). Drives the chart's dashed
  // edges from each subscribed position to this stream's pseudo-node.
  subscribers?: string[]
  can_publish: boolean
  disable_reason?: string
  recent_events?: EventCard[]
}

export interface StreamsResponse {
  streams: StreamDTO[]
  recent?: unknown[]
}

export interface CreateStreamRequest {
  id?: string
  name: string
  description?: string
  transport?: {
    kind: string
    config?: Record<string, unknown>
  }
}

export interface SettingsSpecDTO {
  key: string
  type: string
  required: boolean
  description: string
  configured: boolean
  // value is the current REDACTED value — secrets are masked. Treat
  // as display-only; to update, send a fresh value via setSetting.
  value: string
}

export interface SettingsResponse {
  owner: string
  public_url?: string
  db_path?: string
  envs_dir?: string
  specs: SettingsSpecDTO[]
}

// HelixModelInfo is the subset of /v1/models response shape the
// settings page renders for the model dropdown. /v1/models returns
// the OpenAI-compat envelope { data: [...] } with extra fields like
// name + description we surface in the dropdown label/help.
export interface HelixModelInfo {
  id: string
  name?: string
  description?: string
  context_length?: number
}

export interface ToolDTO {
  name: string
  description?: string
}

// useHelixOrgBase resolves the current `:org_id` URL param into the
// `/api/v1/orgs/<org>` prefix. The org-graph JSON resources live
// directly under the org segment — chart, workers, … — no extra
// namespace. Returns empty string when no org segment is present so
// callers can gate their queries.
export function useHelixOrgBase(): { base: string; orgID: string } {
  const { params } = useRouter()
  const orgID = (params.org_id as string) || ''
  const base = orgID ? `/api/v1/orgs/${encodeURIComponent(orgID)}` : ''
  return { base, orgID }
}

// ---- Queries -------------------------------------------------------------

export function useHelixOrgChart(options?: { enabled?: boolean }) {
  const api = useApi()
  const { base, orgID } = useHelixOrgBase()
  return useQuery({
    queryKey: QUERY_KEYS.chart(orgID),
    queryFn: async () => {
      const data = await api.get<Chart>(`${base}/chart`)
      return data
    },
    enabled: !!orgID && (options?.enabled ?? true),
  })
}

// useEnsureWorkerChat provisions (or fast-paths) the Worker's per-
// Worker Helix project + agent app, returning the agent_app_id. The
// chart UI's "Start new chat" button calls this when the worker has
// no agent app yet (e.g. the human owner on a fresh org), then
// navigates to /agent/<agent_app_id>. Idempotent.
export function useEnsureWorkerChat() {
  const api = useApi()
  const qc = useQueryClient()
  const { base, orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async (workerId: string) => {
      const data = await api.post<unknown, { agent_app_id: string; project_id?: string }>(
        `${base}/workers/${encodeURIComponent(workerId)}/chat`,
        undefined,
      )
      return data
    },
    onSuccess: (_data, workerId) => {
      // Invalidate the worker detail cache so the page re-renders
      // with the freshly-populated agent_app_id.
      qc.invalidateQueries({ queryKey: QUERY_KEYS.worker(orgID, workerId) })
    },
  })
}

// useListHelixOrgWorkers fetches every worker in the current org.
// Drives the Workers list page. Returns WorkerDTO entries (not
// WorkerDetailDTO — the detail page hydrates that per-worker).
export function useListHelixOrgWorkers(options?: { enabled?: boolean }) {
  const api = useApi()
  const { base, orgID } = useHelixOrgBase()
  return useQuery({
    queryKey: QUERY_KEYS.workers(orgID),
    queryFn: async () => {
      const data = await api.get<WorkerDTO[]>(`${base}/workers`)
      return data ?? []
    },
    enabled: !!orgID && (options?.enabled ?? true),
  })
}

// useHelixOrgWorker drives the right-rail Worker drawer on the chart.
// Returns the full WorkerDetailDTO (worker + role + position) so the
// drawer can show identity / role markdown alongside the fire button.
export function useHelixOrgWorker(workerId: string | undefined, options?: { enabled?: boolean }) {
  const api = useApi()
  const { base, orgID } = useHelixOrgBase()
  return useQuery({
    queryKey: QUERY_KEYS.worker(orgID, workerId ?? ''),
    queryFn: async () => {
      if (!workerId) return null
      const data = await api.get<WorkerDetailDTO>(`${base}/workers/${encodeURIComponent(workerId)}`)
      return data
    },
    enabled: !!orgID && !!workerId && (options?.enabled ?? true),
  })
}

// ---- Mutations -----------------------------------------------------------

// useListHelixOrgTools returns the catalogue of every MCP tool the
// org can grant to a Role. Drives the role-editor multi-select.
// Cached aggressively because the catalogue only changes when the
// server registers a new built-in (i.e. on deploy), not in response
// to operator actions.
export function useListHelixOrgTools(options?: { enabled?: boolean }) {
  const api = useApi()
  const { base, orgID } = useHelixOrgBase()
  return useQuery({
    queryKey: QUERY_KEYS.tools(orgID),
    queryFn: async () => {
      const data = await api.get<ToolDTO[]>(`${base}/tools`)
      return data ?? []
    },
    staleTime: 5 * 60 * 1000,
    enabled: !!orgID && (options?.enabled ?? true),
  })
}

// useListHelixOrgRoles fetches every role in the current org. Drives
// the Roles list page in the helix-org middle-nav. Cached separately
// from the chart so the list view doesn't repaint when only
// position/worker rows change.
export function useListHelixOrgRoles(options?: { enabled?: boolean }) {
  const api = useApi()
  const { base, orgID } = useHelixOrgBase()
  return useQuery({
    queryKey: QUERY_KEYS.roles(orgID),
    queryFn: async () => {
      const data = await api.get<RoleDTO[]>(`${base}/roles`)
      return data ?? []
    },
    enabled: !!orgID && (options?.enabled ?? true),
  })
}

// useHelixOrgRole drives the right-rail Role drawer on the chart.
// Returns the full RoleDTO (id + content + tools + streams + audit
// stamps) so the drawer can render the role's markdown and metadata.
export function useHelixOrgRole(roleId: string | undefined, options?: { enabled?: boolean }) {
  const api = useApi()
  const { base, orgID } = useHelixOrgBase()
  return useQuery({
    queryKey: QUERY_KEYS.role(orgID, roleId ?? ''),
    queryFn: async () => {
      if (!roleId) return null
      const data = await api.get<RoleDTO>(`${base}/roles/${encodeURIComponent(roleId)}`)
      return data
    },
    enabled: !!orgID && !!roleId && (options?.enabled ?? true),
  })
}

// useHireHelixOrgWorker hires a Worker from the chart's "+" panel.
// Wraps the same hire_worker MCP tool the chat surface uses, so REST
// and chat hires produce identical store state.
export function useHireHelixOrgWorker() {
  const api = useApi()
  const qc = useQueryClient()
  const { base, orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async (payload: HireWorkerRequest) => {
      const data = await api.post<HireWorkerRequest, HireWorkerResponse>(
        `${base}/workers`,
        payload,
      )
      return data
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.chart(orgID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.workers(orgID) })
    },
  })
}

// useFireHelixOrgWorker tears a Worker down. The owner worker is
// server-side protected (409).
export function useFireHelixOrgWorker() {
  const api = useApi()
  const qc = useQueryClient()
  const { base, orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async (workerId: string) => {
      await api.delete(`${base}/workers/${encodeURIComponent(workerId)}`)
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.chart(orgID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.workers(orgID) })
    },
  })
}

// useUpdateHelixOrgRole patches an existing Role. Body fields are
// optional — omit to leave untouched. Tools/Streams are REPLACED
// wholesale when provided (pass `[]` to clear). Invalidates both the
// list cache and the single-role cache so the detail page repaints
// immediately on save.
export function useUpdateHelixOrgRole() {
  const api = useApi()
  const qc = useQueryClient()
  const { base, orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async (payload: { id: string; content?: string; tools?: string[]; streams?: string[] }) => {
      const { id, ...body } = payload
      await api.put(`${base}/roles/${encodeURIComponent(id)}`, body)
    },
    onSuccess: (_data, payload) => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.chart(orgID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.roles(orgID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.role(orgID, payload.id) })
    },
  })
}

// useCreateHelixOrgRole creates a new Role row in the current org.
export function useCreateHelixOrgRole() {
  const api = useApi()
  const qc = useQueryClient()
  const { base, orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async (payload: CreateRoleRequest) => {
      await api.post(`${base}/roles`, payload)
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.chart(orgID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.roles(orgID) })
    },
  })
}

// useCreateHelixOrgPosition creates a new Position row in the
// current org under the given Role.
export function useCreateHelixOrgPosition() {
  const api = useApi()
  const qc = useQueryClient()
  const { base, orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async (payload: CreatePositionRequest) => {
      await api.post(`${base}/positions`, payload)
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.chart(orgID) })
    },
  })
}

// useUpdateHelixOrgPosition rewires a Position. Today only parent_id
// matters — the chart's drag-and-drop hands a position a new parent
// so the org reporting structure changes without losing the position
// itself.
export function useUpdateHelixOrgPosition() {
  const api = useApi()
  const qc = useQueryClient()
  const { base, orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async (payload: { id: string; parent_id?: string; role_id?: string }) => {
      const { id, ...body } = payload
      await api.put(`${base}/positions/${encodeURIComponent(id)}`, body)
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.chart(orgID) })
    },
  })
}

// useDeleteHelixOrgRole cascades — every Position under the Role is
// deleted, every Worker in those Positions is fired. The owner Role
// is server-side protected (409).
export function useDeleteHelixOrgRole() {
  const api = useApi()
  const qc = useQueryClient()
  const { base, orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async (roleId: string) => {
      await api.delete(`${base}/roles/${encodeURIComponent(roleId)}`)
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.chart(orgID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.roles(orgID) })
    },
  })
}

// useDeleteHelixOrgPosition cascades — the Worker in the Position
// (if any) is fired. The root position is server-side protected (409).
export function useDeleteHelixOrgPosition() {
  const api = useApi()
  const qc = useQueryClient()
  const { base, orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async (positionId: string) => {
      await api.delete(`${base}/positions/${encodeURIComponent(positionId)}`)
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.chart(orgID) })
    },
  })
}

// useHelixOrgSettings reads every registered config spec + its
// redacted current value. Drives the helix-org Settings page.
export function useHelixOrgSettings(options?: { enabled?: boolean }) {
  const api = useApi()
  const { base, orgID } = useHelixOrgBase()
  return useQuery({
    queryKey: QUERY_KEYS.settings(orgID),
    queryFn: async () => {
      const data = await api.get<SettingsResponse>(`${base}/settings`)
      return data
    },
    enabled: !!orgID && (options?.enabled ?? true),
  })
}

// useSetHelixOrgSetting writes a single config row. The backend
// expects the raw JSON wire form per the spec's Type (string specs
// expect a JSON-encoded string, etc.) — the page is responsible for
// quoting strings before calling this hook.
export function useSetHelixOrgSetting() {
  const api = useApi()
  const qc = useQueryClient()
  const { base, orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async (payload: { key: string; value: string }) => {
      await api.put(`${base}/settings/${encodeURIComponent(payload.key)}`, { value: payload.value })
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.settings(orgID) })
    },
  })
}

// useDeleteHelixOrgSetting clears the explicit row — the registry then
// falls back to the spec's default (or empty if no default).
export function useDeleteHelixOrgSetting() {
  const api = useApi()
  const qc = useQueryClient()
  const { base, orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async (key: string) => {
      await api.delete(`${base}/settings/${encodeURIComponent(key)}`)
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.settings(orgID) })
    },
  })
}

// useHelixProviders fetches the catalogue of providers configured on
// this Helix instance. Powers the worker.provider dropdown on the
// settings page. Cached across orgs because the catalogue is a
// Helix-instance-wide setting, not per-org.
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

// useHelixModelsForProvider fetches the list of models a provider
// exposes. Backs the worker.model dropdown — re-fetched whenever the
// provider changes. /v1/models uses the OpenAI-compatible envelope
// ({data: [...]}). Returns an empty array on error so the dropdown
// can render a "no models" placeholder.
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

// useListHelixOrgStreams fetches every stream + its current
// subscribers + recent events. Drives the Streams list page AND the
// chart's stream-subscription edges.
export function useListHelixOrgStreams(options?: { enabled?: boolean }) {
  const api = useApi()
  const { base, orgID } = useHelixOrgBase()
  return useQuery({
    queryKey: QUERY_KEYS.streams(orgID),
    queryFn: async () => {
      const data = await api.get<StreamsResponse>(`${base}/streams`)
      return data
    },
    enabled: !!orgID && (options?.enabled ?? true),
  })
}

// useHelixOrgStream fetches a single stream + its current subscribers +
// recent_events. Drives the per-stream detail page. The SSE endpoint
// at /streams/{id}/events takes over live updates after first paint;
// this hook supplies the initial snapshot so the page can render
// immediately rather than waiting for the first SSE frame.
export function useHelixOrgStream(streamId: string | undefined, options?: { enabled?: boolean }) {
  const api = useApi()
  const { base, orgID } = useHelixOrgBase()
  return useQuery({
    queryKey: QUERY_KEYS.stream(orgID, streamId ?? ''),
    queryFn: async () => {
      if (!streamId) return null
      const data = await api.get<StreamDTO>(`${base}/streams/${encodeURIComponent(streamId)}`)
      return data
    },
    enabled: !!orgID && !!streamId && (options?.enabled ?? true),
  })
}

// useCreateHelixOrgStream creates a new Stream. Server falls back to
// s-<uuid> if `id` is omitted; pass an explicit id for stable handles.
export function useCreateHelixOrgStream() {
  const api = useApi()
  const qc = useQueryClient()
  const { base, orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async (payload: CreateStreamRequest) => {
      const data = await api.post<CreateStreamRequest, StreamDTO>(`${base}/streams`, payload)
      return data
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.streams(orgID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.chart(orgID) })
    },
  })
}

// useDeleteHelixOrgStream tears a stream down. Subscriptions and
// events are NOT cascade-deleted in this iteration; the operator
// is expected to drain them first.
export function useDeleteHelixOrgStream() {
  const api = useApi()
  const qc = useQueryClient()
  const { base, orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async (streamId: string) => {
      await api.delete(`${base}/streams/${encodeURIComponent(streamId)}`)
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.streams(orgID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.chart(orgID) })
    },
  })
}

// ---- Position subscriptions ---------------------------------------------

// PositionSubscription is one row in a position's subscription set —
// mirror of api.PositionSubscriptionDTO. The Worker detail page lists
// these for the worker's filling position, the chart's dashed
// subscription edges hang off them.
export interface PositionSubscription {
  stream_id: string
  created_at: string
}

export interface PositionSubscriptionsResponse {
  position_id: string
  subscriptions: PositionSubscription[]
}

// useListPositionSubscriptions fetches the canonical "what streams
// does this position consume" set. The Worker detail page resolves
// worker → position before calling this; null positionID disables
// the query so callers don't have to gate at the call site.
export function useListPositionSubscriptions(positionID: string | undefined, options?: { enabled?: boolean }) {
  const api = useApi()
  const { base, orgID } = useHelixOrgBase()
  return useQuery({
    queryKey: QUERY_KEYS.positionSubs(orgID, positionID ?? ''),
    queryFn: async () => {
      if (!positionID) return null
      const data = await api.get<PositionSubscriptionsResponse>(`${base}/positions/${encodeURIComponent(positionID)}/subscriptions`)
      return data
    },
    enabled: !!orgID && !!positionID && (options?.enabled ?? true),
  })
}

// useSubscribePosition adds a (position, stream) subscription.
// Idempotent server-side (returns 200 with the existing row); we
// invalidate the position's sub list + the streams list so the
// chart's dashed edges and the worker detail panel both refresh.
export function useSubscribePosition(positionID: string | undefined) {
  const api = useApi()
  const qc = useQueryClient()
  const { base, orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async (streamID: string) => {
      if (!positionID) throw new Error('positionID is required to subscribe')
      const data = await api.post<{ stream_id: string }, PositionSubscription>(
        `${base}/positions/${encodeURIComponent(positionID)}/subscriptions`,
        { stream_id: streamID },
      )
      return data
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

// useUnsubscribePosition drops a (position, stream) subscription.
export function useUnsubscribePosition(positionID: string | undefined) {
  const api = useApi()
  const qc = useQueryClient()
  const { base, orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async (streamID: string) => {
      if (!positionID) throw new Error('positionID is required to unsubscribe')
      await api.delete(`${base}/positions/${encodeURIComponent(positionID)}/subscriptions/${encodeURIComponent(streamID)}`)
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
