export interface RunnerSlot {
  id?: string
  model?: string
  runtime?: string
  command_line?: string
}

export interface RunnerModelStatus {
  model_id?: string
  download_in_progress?: boolean
  download_percent?: number
  error?: string
}

export interface DashboardRunner {
  id?: string
  slots?: RunnerSlot[]
  models?: RunnerModelStatus[]
}

export interface DashboardData {
  runners: DashboardRunner[]
}

export interface MemoryEstimate {
  total_size?: number
  vram_size?: number
  weights?: number
  kv_cache?: number
  requires_fallback?: boolean
}

export interface MemoryEstimationResponse {
  estimate?: MemoryEstimate
  estimates?: Record<string, MemoryEstimate>
  error?: string
}
