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
  role: (orgID: string, id: string) => ['helix-org', orgID, 'roles', id] as const,
  roles: (orgID: string) => ['helix-org', orgID, 'roles'] as const,
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
