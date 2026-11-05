import { TypesNativeDisplay, TypesSandboxDisplay } from '../api/api'

export interface DisplaySettings {
  width: number
  height: number
  fps: number
}

export type SandboxResolution = '1080p' | '4k' | '5k' | 'native'

export interface SandboxResolutionOption {
  value: SandboxResolution
  label: string
  description: string
  width: number
  height: number
}

// Mirrors types.SandboxResolutionSize in api/pkg/types/simple_spec_task.go.
export const SANDBOX_RESOLUTIONS: SandboxResolutionOption[] = [
  { value: '1080p', label: '1080p', description: '1920×1080', width: 1920, height: 1080 },
  { value: '4k', label: '4K', description: '3840×2160 · for demo recordings', width: 3840, height: 2160 },
  { value: '5k', label: '5K', description: '5120×2880', width: 5120, height: 2880 },
]

function hasNative(native?: TypesNativeDisplay): native is Required<TypesNativeDisplay> {
  return !!native?.width && !!native?.height
}

// sandboxResolutions lists the presets, plus "native" where the host reported
// its display (Helix for Mac: the laptop's width and zoom, at 16:9).
export function sandboxResolutions(native?: TypesNativeDisplay): SandboxResolutionOption[] {
  if (!hasNative(native)) {
    return SANDBOX_RESOLUTIONS
  }
  return [
    {
      value: 'native',
      label: 'Native',
      description: `${native.width}×${native.height} · matches this Mac`,
      width: native.width,
      height: native.height,
    },
    ...SANDBOX_RESOLUTIONS,
  ]
}

// Mirrors types.DefaultSandboxResolution: native where reported, else 1080p.
export function defaultSandboxResolution(native?: TypesNativeDisplay): SandboxResolution {
  return hasNative(native) ? 'native' : '1080p'
}

export function sandboxResolution(display?: TypesSandboxDisplay, native?: TypesNativeDisplay): SandboxResolution {
  return sandboxResolutions(native).find((r) => r.value === display?.resolution)?.value
    || defaultSandboxResolution(native)
}

export function sandboxResolutionLabel(display?: TypesSandboxDisplay, native?: TypesNativeDisplay): string {
  const value = sandboxResolution(display, native)
  return sandboxResolutions(native).find((r) => r.value === value)!.label
}

// deriveDisplaySettings resolves a spec task's desktop size from its
// sandbox_display, defaulting the way the backend does.
export function deriveDisplaySettings(display?: TypesSandboxDisplay, native?: TypesNativeDisplay): DisplaySettings {
  const value = sandboxResolution(display, native)
  const preset = sandboxResolutions(native).find((r) => r.value === value)!
  return { width: preset.width, height: preset.height, fps: 60 }
}
