import { describe, expect, it } from 'vitest'
import {
  DEFAULT_SANDBOX_PRESET,
  SANDBOX_PRESETS,
  defaultSandboxResourceOverrides,
  sandboxPresetsFor,
} from './sandboxPresets'

describe('sandbox presets', () => {
  it('mirrors the Go ladder in api/pkg/types/simple_spec_task.go', () => {
    expect(SANDBOX_PRESETS.map((preset) => [preset.vcpus, preset.memory_mb])).toEqual([
      [1, 2048],
      [4, 8192],
      [8, 16384],
      [12, 24576],
      [16, 32768],
    ])
  })

  it('keys every rung on a distinct vCPU count', () => {
    const vcpus = SANDBOX_PRESETS.map((preset) => preset.vcpus)
    expect(new Set(vcpus).size).toBe(vcpus.length)
  })

  it('defaults to 12 vCPU / 24 GB', () => {
    expect(DEFAULT_SANDBOX_PRESET.vcpus).toBe(12)
    expect(DEFAULT_SANDBOX_PRESET.memory_mb).toBe(24576)
  })

  it('sends only the two numeric fields to the API', () => {
    expect(defaultSandboxResourceOverrides()).toEqual({ vcpus: 12, memory_mb: 24576 })
  })

  // The 31 rows backfilled on meta hold 12/24576. Before the 12-vCPU rung
  // existed the menu had nothing selected for them, which reads as "no sandbox
  // configured" rather than "24 GB".
  it('offers a rung matching a stored 12 vCPU value', () => {
    const presets = sandboxPresetsFor(12, 24576)
    expect(presets).toEqual(SANDBOX_PRESETS)
    expect(presets.find((preset) => preset.vcpus === 12)).toBeDefined()
  })

  // Nothing validates a preset read from the database — every ValidPreset() call
  // site guards an incoming request — so an off-ladder row must stay visible.
  it('surfaces an off-ladder stored value instead of dropping it', () => {
    const presets = sandboxPresetsFor(6, 12288)
    expect(presets).toHaveLength(SANDBOX_PRESETS.length + 1)
    expect(presets.find((preset) => preset.vcpus === 6)).toMatchObject({
      vcpus: 6,
      memory_mb: 12288,
      description: '12 GB RAM',
    })
    expect(presets.map((preset) => preset.vcpus)).toEqual([1, 4, 6, 8, 12, 16])
  })

  it('leaves the ladder alone when no size is stored', () => {
    expect(sandboxPresetsFor(undefined, undefined)).toEqual(SANDBOX_PRESETS)
  })
})
