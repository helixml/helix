/**
 * The sandbox size ladder, mirroring types.SpecTaskSandboxPresets in
 * api/pkg/types/simple_spec_task.go. vCPU is the key — memory is never
 * independently selectable — so every rung has a distinct vCPU count.
 *
 * Both `label` and `description` are kept: SpecTaskExecutionControls renders
 * both, CodeAgentExecutionControls renders only `description`. Dropping either
 * silently blanks one menu.
 */
export interface SandboxPreset {
  vcpus: number
  memory_mb: number
  label: string
  description: string
}

export const SANDBOX_PRESETS: SandboxPreset[] = [
  { vcpus: 1, memory_mb: 2048, label: '1 CPU', description: '2 GB RAM' },
  { vcpus: 4, memory_mb: 8192, label: '4 CPU', description: '8 GB RAM' },
  { vcpus: 8, memory_mb: 16384, label: '8 CPU', description: '16 GB RAM' },
  { vcpus: 12, memory_mb: 24576, label: '12 CPU', description: '24 GB RAM' },
  { vcpus: 16, memory_mb: 32768, label: '16 CPU', description: '32 GB RAM' },
]

/** Mirrors DefaultSpecTaskSandboxVCPUs / DefaultSpecTaskSandboxMemoryMB. */
export const DEFAULT_SANDBOX_PRESET: SandboxPreset = SANDBOX_PRESETS[3]

/** The two numeric fields the API accepts — never send label/description. */
export const defaultSandboxResourceOverrides = () => ({
  vcpus: DEFAULT_SANDBOX_PRESET.vcpus,
  memory_mb: DEFAULT_SANDBOX_PRESET.memory_mb,
})

/**
 * The rungs to offer for a given stored value.
 *
 * A row can hold a size that is not on the ladder: nothing validates a preset
 * read from the database (every ValidPreset() call site guards an *incoming*
 * request), and rows have been hand-edited in the past. Such a value used to
 * leave the menu with nothing selected, which reads as "no sandbox configured"
 * rather than "a size we don't have a rung for". Append it instead so it is
 * visible and selected.
 */
export const sandboxPresetsFor = (vcpus?: number, memoryMB?: number): SandboxPreset[] => {
  if (!vcpus || SANDBOX_PRESETS.some((preset) => preset.vcpus === vcpus)) {
    return SANDBOX_PRESETS
  }
  const memory = memoryMB ? `${Math.round(memoryMB / 1024)} GB RAM` : 'custom size'
  return [
    ...SANDBOX_PRESETS,
    { vcpus, memory_mb: memoryMB ?? 0, label: `${vcpus} CPU`, description: memory },
  ].sort((a, b) => a.vcpus - b.vcpus)
}
