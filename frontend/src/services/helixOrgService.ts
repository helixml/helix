// Hand-rolled service for the helix-org JSON API mounted under
// /api/v1/org/*. The endpoints are not yet in the generated client because
// the OpenAPI regen step hasn't been run since Phase A added them — this
// file is the bridge until `./stack update_openapi` is rerun, after which
// the calls can be migrated to `api.getApiClient()`.
//
// All endpoints are auth-gated by `requireUser` + `requireFeature(helix-org)`
// in api/pkg/server/server.go, so the same session cookie that authorises
// the rest of the React app gates these routes too.
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

export interface Chart {
  roots: ChartNode[]
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
}

export interface SettingsSpecDTO {
  key: string
  type: string
  required: boolean
  description: string
  configured: boolean
  value: string
}

export interface SettingsResponse {
  owner: string
  public_url?: string
  db_path?: string
  envs_dir?: string
  specs: SettingsSpecDTO[]
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
  subscribers?: string[]
  can_publish: boolean
  disable_reason?: string
  recent_events?: EventCard[]
}

export interface StreamsResponse {
  streams: StreamDTO[]
  recent?: EventCard[]
}

export interface PublishRequest {
  body: string
  subject?: string
  to?: string[]
}

export interface HireWorkerRequest {
  id?: string
  position_id: string
  kind: 'human' | 'ai'
  identity_content: string
  grants?: { tool_name: string }[]
}

export interface HireWorkerResponse {
  id: string
  activation_id?: string
}

export interface PublishResponse {
  event_id: string
}

// ---- Query keys ----------------------------------------------------------

// Query keys are now per-org so cross-org navigation doesn't reuse
// the wrong tenant's cached data.
export const QUERY_KEYS = {
  chart: (orgID: string) => ['helix-org', orgID, 'chart'] as const,
  workers: (orgID: string) => ['helix-org', orgID, 'workers'] as const,
  worker: (orgID: string, id: string) => ['helix-org', orgID, 'workers', id] as const,
  settings: (orgID: string) => ['helix-org', orgID, 'settings'] as const,
  streams: (orgID: string) => ['helix-org', orgID, 'streams'] as const,
}

// useHelixOrgBase resolves the current `:org_id` URL param into the
// `/api/v1/orgs/<org>` prefix. The org-graph JSON resources live
// directly under the org segment — chart, positions, roles, workers,
// streams, settings — no extra namespace. Returns empty string when
// no org segment is present so callers can gate their queries.
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

export function useHelixOrgWorkers(options?: { enabled?: boolean }) {
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

export function useHelixOrgStreams(options?: { enabled?: boolean }) {
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

// ---- Mutations -----------------------------------------------------------

// useHireHelixOrgWorker hires a Worker. Server runs the same hire_worker
// tool MCP uses, so chart-driven hires produce identical store state to
// chat-driven hires (env dir, activation stream, hire dispatch).
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
      qc.invalidateQueries({ queryKey: QUERY_KEYS.workers(orgID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.chart(orgID) })
    },
  })
}

// useFireHelixOrgWorker tears a Worker down: stops sessions, deletes
// the Helix project + agent app, clears runtime state, removes the
// env dir + grants + subscriptions, finally the worker row. The owner
// worker is server-side protected (409).
export function useFireHelixOrgWorker() {
  const api = useApi()
  const qc = useQueryClient()
  const { base, orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async (workerId: string) => {
      await api.delete(`${base}/workers/${encodeURIComponent(workerId)}`)
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.workers(orgID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.chart(orgID) })
    },
  })
}

export function useUpdateHelixOrgWorkerIdentity(workerId: string) {
  const api = useApi()
  const qc = useQueryClient()
  const { base, orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async (identity: string) => {
      await api.post(`${base}/workers/${encodeURIComponent(workerId)}/identity`, { identity })
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.worker(orgID, workerId) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.workers(orgID) })
    },
  })
}

export function useUpdateHelixOrgWorkerRole(workerId: string) {
  const api = useApi()
  const qc = useQueryClient()
  const { base, orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async (content: string) => {
      await api.post(`${base}/workers/${encodeURIComponent(workerId)}/role`, { content })
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.worker(orgID, workerId) })
    },
  })
}

export function useSetHelixOrgSetting() {
  const api = useApi()
  const qc = useQueryClient()
  const { base, orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async (input: { key: string; value: string }) => {
      await api.put(`${base}/settings/${encodeURIComponent(input.key)}`, { value: input.value })
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.settings(orgID) })
    },
  })
}

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

export function usePublishHelixOrgStream(streamId: string) {
  const api = useApi()
  const qc = useQueryClient()
  const { base, orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async (payload: PublishRequest) => {
      const data = await api.post<PublishRequest, PublishResponse>(
        `${base}/streams/${encodeURIComponent(streamId)}/publish`,
        payload,
      )
      return data
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.streams(orgID) })
    },
  })
}

// ---- SSE helpers ---------------------------------------------------------

// helixOrgStreamEventsUrl returns the SSE URL for a given stream id within
// the current org scope. Callers instantiate EventSource themselves — they
// already know the orgID from the URL.
export function helixOrgStreamEventsUrl(orgID: string, streamId: string): string {
  return `/api/v1/orgs/${encodeURIComponent(orgID)}/streams/${encodeURIComponent(streamId)}/events`
}
