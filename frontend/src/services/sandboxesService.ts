import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import useApi from '../hooks/useApi'
import {
  TypesSandbox,
  TypesCreateSandboxRequest,
  TypesUpdateSandboxRequest,
  TypesRunSandboxCommandRequest,
} from '../api/api'

// React Query keys for sandboxes. Keep one helper per shape so call sites stay
// consistent and so invalidations only touch the right slice of cache.
export const sandboxesListQueryKey = (orgId: string) => ['sandboxes', 'list', orgId]
export const sandboxQueryKey = (orgId: string, id: string) => ['sandboxes', 'detail', orgId, id]
export const sandboxCommandsQueryKey = (orgId: string, id: string) => [
  'sandboxes',
  'commands',
  orgId,
  id,
]
export const sandboxFilesQueryKey = (orgId: string, id: string, path?: string) => [
  'sandboxes',
  'files',
  orgId,
  id,
  path ?? '/root',
]
export const sandboxBillingQueryKey = (orgId: string, id: string) => [
  'sandboxes',
  'billing',
  orgId,
  id,
]
export const sandboxTerminalSessionsQueryKey = (orgId: string, id: string) => [
  'sandboxes',
  'terminal-sessions',
  orgId,
  id,
]

// Lists the runtime names (e.g. "headless-ubuntu", "node22", "ubuntu-desktop")
// the operator has registered on this server. The picker uses this to build
// the runtime tile grid so adding a runtime via HELIX_SANDBOX_RUNTIMES is
// reflected in the UI without a code change.
export function useListSandboxRuntimes(options?: { enabled?: boolean }) {
  const api = useApi()
  const apiClient = api.getApiClient()
  return useQuery({
    queryKey: ['sandboxes', 'runtimes'],
    queryFn: async () => {
      const result = await apiClient.v1SandboxRuntimesList()
      return (result.data as unknown as { runtimes?: string[] })?.runtimes ?? []
    },
    enabled: options?.enabled ?? true,
    staleTime: 60_000,
  })
}

export function useListSandboxes(orgId: string | undefined, options?: { enabled?: boolean }) {
  const api = useApi()
  const apiClient = api.getApiClient()

  return useQuery({
    queryKey: sandboxesListQueryKey(orgId ?? ''),
    queryFn: async () => {
      if (!orgId) return { sandboxes: [], total: 0 }
      const result = await apiClient.v1OrganizationsSandboxesDetail(orgId)
      return result.data
    },
    enabled: !!orgId && (options?.enabled ?? true),
    refetchInterval: 5000, // poll while sandboxes are pending/running so the UI stays fresh
  })
}

export function useSandbox(
  orgId: string | undefined,
  id: string | undefined,
  options?: { enabled?: boolean }
) {
  const api = useApi()
  const apiClient = api.getApiClient()

  return useQuery({
    queryKey: sandboxQueryKey(orgId ?? '', id ?? ''),
    queryFn: async () => {
      if (!orgId || !id) return undefined as unknown as TypesSandbox
      const result = await apiClient.v1OrganizationsSandboxesDetail2(orgId, id)
      return result.data
    },
    enabled: !!orgId && !!id && (options?.enabled ?? true),
    refetchInterval: (query) => {
      const status = query.state.data?.status
      if (!status || status === 'pending' || status === 'stopping') return 2000
      if (status === 'running') return 5000
      return false
    },
  })
}

export function useCreateSandbox(orgId: string) {
  const api = useApi()
  const apiClient = api.getApiClient()
  const qc = useQueryClient()

  return useMutation({
    mutationFn: async (payload: TypesCreateSandboxRequest) => {
      const result = await apiClient.v1OrganizationsSandboxesCreate(orgId, payload)
      return result.data
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: sandboxesListQueryKey(orgId) })
    },
  })
}

export function useUpdateSandbox(orgId: string) {
  const api = useApi()
  const apiClient = api.getApiClient()
  const qc = useQueryClient()

  return useMutation({
    mutationFn: async (input: { id: string; payload: TypesUpdateSandboxRequest }) => {
      const result = await apiClient.v1OrganizationsSandboxesPartialUpdate(
        orgId,
        input.id,
        input.payload,
      )
      return result.data
    },
    onSuccess: (data) => {
      qc.invalidateQueries({ queryKey: sandboxesListQueryKey(orgId) })
      if (data?.id) qc.invalidateQueries({ queryKey: sandboxQueryKey(orgId, data.id) })
    },
  })
}

export function useDeleteSandbox(orgId: string) {
  const api = useApi()
  const apiClient = api.getApiClient()
  const qc = useQueryClient()

  return useMutation({
    mutationFn: async (id: string) => {
      await apiClient.v1OrganizationsSandboxesDelete(orgId, id)
    },
    onSuccess: (_, id) => {
      qc.invalidateQueries({ queryKey: sandboxesListQueryKey(orgId) })
      qc.invalidateQueries({ queryKey: sandboxQueryKey(orgId, id) })
    },
  })
}

export function useSandboxCommands(
  orgId: string | undefined,
  id: string | undefined,
  options?: { enabled?: boolean },
) {
  const api = useApi()
  const apiClient = api.getApiClient()

  return useQuery({
    queryKey: sandboxCommandsQueryKey(orgId ?? '', id ?? ''),
    queryFn: async () => {
      if (!orgId || !id) return { commands: [] }
      const result = await apiClient.v1OrganizationsSandboxesCommandsDetail(orgId, id)
      return result.data as unknown as {
        commands: Array<{
          id: string
          cmd: string
          args?: string[]
          status: string
          exit_code?: number
          started_at?: string
          finished_at?: string
        }>
      }
    },
    enabled: !!orgId && !!id && (options?.enabled ?? true),
    refetchInterval: 3000,
  })
}

export function useRunSandboxCommand(orgId: string, id: string) {
  const api = useApi()
  const apiClient = api.getApiClient()
  const qc = useQueryClient()

  return useMutation({
    mutationFn: async (payload: TypesRunSandboxCommandRequest) => {
      const result = await apiClient.v1OrganizationsSandboxesCommandsCreate(orgId, id, payload)
      return result.data
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: sandboxCommandsQueryKey(orgId, id) })
    },
  })
}

export function useKillSandboxCommand(orgId: string, id: string) {
  const api = useApi()
  const apiClient = api.getApiClient()
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (input: { cmdId: string; signal?: string }) => {
      await apiClient.v1OrganizationsSandboxesCommandsKillCreate(orgId, id, input.cmdId, {
        signal: input.signal,
      } as any)
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: sandboxCommandsQueryKey(orgId, id) })
    },
  })
}

export function useSandboxFiles(
  orgId: string | undefined,
  id: string | undefined,
  path: string,
  options?: { enabled?: boolean },
) {
  const api = useApi()
  const apiClient = api.getApiClient()

  return useQuery({
    queryKey: sandboxFilesQueryKey(orgId ?? '', id ?? '', path),
    queryFn: async () => {
      if (!orgId || !id) return { path, entries: [] }
      const result = await apiClient.v1OrganizationsSandboxesFilesListDetail(orgId, id, { path })
      return result.data as unknown as {
        path: string
        entries: Array<{
          name: string
          path: string
          is_dir: boolean
          size: number
          mode: string
          mod_time: string
        }>
      }
    },
    enabled: !!orgId && !!id && (options?.enabled ?? true),
  })
}

// Billing summary for a single sandbox. Returned by /sandboxes/{id}/billing.
// Backend computes price from the sandbox runtime + the configured per-second
// rate, and `total_credits_charged` from the magnitude of all transactions
// tagged with the sandbox id (so we count every interval the reaper has
// debited so far).
export interface SandboxBilling {
  enabled: boolean
  // Effective per-second rate for this sandbox (already multiplied by vCPUs
  // server-side, so display directly).
  price_credits_per_second: number
  // Sum of completed-minute charges already debited from the wallet.
  total_credits_charged: number
  // Live partial-minute accrual since the last reaper tick — non-zero only
  // while the sandbox is running and billing is enabled. Useful for showing
  // immediate feedback before the next minute boundary.
  pending_credits: number
  runtime: string
  vcpus: number
}

export function useSandboxBilling(
  orgId: string | undefined,
  id: string | undefined,
  options?: { enabled?: boolean; refetchInterval?: number | false },
) {
  const api = useApi()
  const apiClient = api.getApiClient()

  return useQuery({
    queryKey: sandboxBillingQueryKey(orgId ?? '', id ?? ''),
    queryFn: async (): Promise<SandboxBilling | null> => {
      if (!orgId || !id) return null
      const result = await apiClient.v1OrganizationsSandboxesBillingDetail(orgId, id, {
        snackbar: false,
      } as any)
      return {
        enabled: result.data.enabled ?? false,
        price_credits_per_second: result.data.price_credits_per_second ?? 0,
        total_credits_charged: result.data.total_credits_charged ?? 0,
        pending_credits: result.data.pending_credits ?? 0,
        runtime: result.data.runtime ?? '',
        vcpus: result.data.vcpus ?? 0,
      }
    },
    enabled: !!orgId && !!id && (options?.enabled ?? true),
    refetchInterval: options?.refetchInterval ?? 10000,
  })
}

// List of tmux sessions running in a sandbox, used by the terminal UI to
// surface a session switcher. The backend shells out to `tmux list-sessions`
// inside the sandbox container.
export interface SandboxTerminalSession {
  name: string
  attached: boolean
  windows?: number
  created?: number
}

export function useSandboxTerminalSessions(
  orgId: string | undefined,
  id: string | undefined,
  options?: { enabled?: boolean; refetchInterval?: number },
) {
  const api = useApi()
  const apiClient = api.getApiClient()

  return useQuery({
    queryKey: sandboxTerminalSessionsQueryKey(orgId ?? '', id ?? ''),
    queryFn: async (): Promise<{ sessions: SandboxTerminalSession[] } | null> => {
      if (!orgId || !id) return null
      const result = await apiClient.v1OrganizationsSandboxesTerminalSessionsDetail(orgId, id, {
        snackbar: false,
      } as any)
      return {
        sessions: (result.data.sessions ?? []).map((session) => ({
          name: session.name ?? '',
          attached: session.attached ?? false,
          windows: session.windows,
          created: session.created,
        })),
      }
    },
    enabled: !!orgId && !!id && (options?.enabled ?? true),
    refetchInterval: options?.refetchInterval ?? 5000,
  })
}

// Build a websocket URL for the sandbox terminal. We always use the same
// origin so cookies/API keys flow through. When `sessionName` is provided
// the backend wraps the shell in `tmux new-session -A -s helix-<name>` so
// reconnects (e.g. on browser refresh) reattach to the same tmux session.
export function sandboxTerminalUrl(orgId: string, id: string, sessionName?: string): string {
  const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  const base = `${proto}//${window.location.host}/api/v1/organizations/${orgId}/sandboxes/${id}/terminal`
  return sessionName ? `${base}?session=${encodeURIComponent(sessionName)}` : base
}

// Build a fetch URL for reading a file. Caller can use fetch+blob.
export function sandboxFileUrl(orgId: string, id: string, path: string): string {
  return `/api/v1/organizations/${orgId}/sandboxes/${id}/files?path=${encodeURIComponent(path)}`
}

// Build the SSE log stream URL.
export function sandboxLogStreamUrl(
  orgId: string,
  id: string,
  cmdId: string,
  follow: boolean,
): string {
  return `/api/v1/organizations/${orgId}/sandboxes/${id}/commands/${cmdId}/logs?stream=both${
    follow ? '&follow=1' : ''
  }`
}
