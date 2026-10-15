import { TypesSandboxDisplay } from '../api/api'

export interface DisplaySettings {
  width: number
  height: number
  fps: number
}

export type SandboxResolution = '1080p' | '4k' | '5k'

// Mirrors types.SandboxResolutionSize in api/pkg/types/simple_spec_task.go.
export const SANDBOX_RESOLUTIONS: { value: SandboxResolution, label: string, description: string, width: number, height: number }[] = [
  { value: '1080p', label: '1080p', description: '1920×1080', width: 1920, height: 1080 },
  { value: '4k', label: '4K', description: '3840×2160 · for demo recordings', width: 3840, height: 2160 },
  { value: '5k', label: '5K', description: '5120×2880', width: 5120, height: 2880 },
]

export function sandboxResolution(display?: TypesSandboxDisplay): SandboxResolution {
  return SANDBOX_RESOLUTIONS.find((r) => r.value === display?.resolution)?.value || '1080p'
}

export function sandboxResolutionLabel(display?: TypesSandboxDisplay): string {
  const value = sandboxResolution(display)
  return SANDBOX_RESOLUTIONS.find((r) => r.value === value)!.label
}

// deriveDisplaySettings resolves a spec task's desktop size from its
// sandbox_display, defaulting to 1920x1080x60 like the backend does.
export function deriveDisplaySettings(display?: TypesSandboxDisplay): DisplaySettings {
  const preset = SANDBOX_RESOLUTIONS.find((r) => r.value === sandboxResolution(display))!
  return { width: preset.width, height: preset.height, fps: 60 }
}
