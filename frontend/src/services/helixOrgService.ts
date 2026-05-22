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

export interface PublishResponse {
  event_id: string
}

// ---- Query keys ----------------------------------------------------------

export const QUERY_KEYS = {
  chart: ['helix-org', 'chart'] as const,
  workers: ['helix-org', 'workers'] as const,
  worker: (id: string) => ['helix-org', 'workers', id] as const,
  settings: ['helix-org', 'settings'] as const,
  streams: ['helix-org', 'streams'] as const,
}

const BASE = '/api/v1/org'

// ---- Queries -------------------------------------------------------------

export function useHelixOrgChart(options?: { enabled?: boolean }) {
  const api = useApi()
  return useQuery({
    queryKey: QUERY_KEYS.chart,
    queryFn: async () => {
      const data = await api.get<Chart>(`${BASE}/chart`)
      return data
    },
    enabled: options?.enabled ?? true,
  })
}

export function useHelixOrgWorkers(options?: { enabled?: boolean }) {
  const api = useApi()
  return useQuery({
    queryKey: QUERY_KEYS.workers,
    queryFn: async () => {
      const data = await api.get<WorkerDTO[]>(`${BASE}/workers`)
      return data ?? []
    },
    enabled: options?.enabled ?? true,
  })
}

export function useHelixOrgWorker(workerId: string | undefined, options?: { enabled?: boolean }) {
  const api = useApi()
  return useQuery({
    queryKey: QUERY_KEYS.worker(workerId ?? ''),
    queryFn: async () => {
      if (!workerId) return null
      const data = await api.get<WorkerDetailDTO>(`${BASE}/workers/${encodeURIComponent(workerId)}`)
      return data
    },
    enabled: !!workerId && (options?.enabled ?? true),
  })
}

export function useHelixOrgSettings(options?: { enabled?: boolean }) {
  const api = useApi()
  return useQuery({
    queryKey: QUERY_KEYS.settings,
    queryFn: async () => {
      const data = await api.get<SettingsResponse>(`${BASE}/settings`)
      return data
    },
    enabled: options?.enabled ?? true,
  })
}

export function useHelixOrgStreams(options?: { enabled?: boolean }) {
  const api = useApi()
  return useQuery({
    queryKey: QUERY_KEYS.streams,
    queryFn: async () => {
      const data = await api.get<StreamsResponse>(`${BASE}/streams`)
      return data
    },
    enabled: options?.enabled ?? true,
  })
}

// ---- Mutations -----------------------------------------------------------

export function useUpdateHelixOrgWorkerIdentity(workerId: string) {
  const api = useApi()
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (identity: string) => {
      await api.post(`${BASE}/workers/${encodeURIComponent(workerId)}/identity`, { identity })
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.worker(workerId) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.workers })
    },
  })
}

export function useUpdateHelixOrgWorkerRole(workerId: string) {
  const api = useApi()
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (content: string) => {
      await api.post(`${BASE}/workers/${encodeURIComponent(workerId)}/role`, { content })
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.worker(workerId) })
    },
  })
}

export function useSetHelixOrgSetting() {
  const api = useApi()
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (input: { key: string; value: string }) => {
      await api.put(`${BASE}/settings/${encodeURIComponent(input.key)}`, { value: input.value })
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.settings })
    },
  })
}

export function useDeleteHelixOrgSetting() {
  const api = useApi()
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (key: string) => {
      await api.delete(`${BASE}/settings/${encodeURIComponent(key)}`)
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.settings })
    },
  })
}

export function usePublishHelixOrgStream(streamId: string) {
  const api = useApi()
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (payload: PublishRequest) => {
      const data = await api.post<PublishRequest, PublishResponse>(
        `${BASE}/streams/${encodeURIComponent(streamId)}/publish`,
        payload,
      )
      return data
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.streams })
    },
  })
}

// ---- SSE helpers ---------------------------------------------------------

// helixOrgStreamEventsUrl returns the SSE URL for a given stream id. Callers
// instantiate an EventSource themselves — EventSource doesn't accept custom
// headers, but it sends cookies on same-origin, so the session cookie carries
// the auth.
export function helixOrgStreamEventsUrl(streamId: string): string {
  return `${BASE}/streams/${encodeURIComponent(streamId)}/events`
}
