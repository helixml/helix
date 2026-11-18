import { useQuery } from '@tanstack/react-query'
import useApi from '../hooks/useApi'

// GET /api/v1/sessions/{id}/resources — the session's sandbox utilization
// (types.SessionResources in api/pkg/types/session_resources.go). The
// generated type marks every field optional; the API always sends them.

export interface SessionResourceSummary {
  from: number
  to: number
  samples: number
  cpu_limit_cores: number
  cpu_cores_p50: number
  cpu_cores_p95: number
  cpu_cores_max: number
  cpu_throttled_pct: number
  memory_limit_bytes: number
  memory_max_bytes: number
  memory_peak_bytes: number
  cpu_pressure_pct: number
  memory_pressure_pct: number
  io_pressure_pct: number
  io_read_bytes: number
  io_write_bytes: number
  memory_high_events: number
  oom_kills: number
}

export interface SessionResourcePoint {
  timestamp: number
  cpu_cores: number
  cpu_throttled_pct: number
  memory_bytes: number
  cpu_pressure_pct: number
  memory_pressure_pct: number
  io_pressure_pct: number
  io_read_bps: number
  io_write_bps: number
  disk_used_bytes: number
  oom_kills: number
}

export interface SessionFilesystem {
  paths: string
  dedicated: boolean
  size_bytes: number
  used_bytes: number
  inodes: number
  inodes_used: number
}

export interface SessionResources {
  session_id: string
  summary: SessionResourceSummary
  points: SessionResourcePoint[]
  filesystems: SessionFilesystem[]
}

export function useSessionResources(sessionId: string | undefined, hours: number) {
  const api = useApi()
  return useQuery({
    queryKey: ['session-resources', sessionId ?? '', hours],
    queryFn: async () => {
      if (!sessionId) return null
      const res = await api.getApiClient().v1SessionsResourcesDetail(sessionId, { hours })
      return res.data as SessionResources
    },
    enabled: !!sessionId,
    refetchInterval: 30000,
  })
}
