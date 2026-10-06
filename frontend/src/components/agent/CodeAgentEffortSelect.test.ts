import { describe, expect, it } from 'vitest'
import { getCodeAgentEffortOptions } from './CodeAgentEffortSelect'

const values = (runtime: string, supported?: readonly string[]) =>
  getCodeAgentEffortOptions(runtime, supported).map((option) => option.value)

describe('getCodeAgentEffortOptions', () => {
  it('keeps the full runtime list when the model has no known profile', () => {
    // Undefined means Helix has no profile — an empty or guessed-narrow
    // selector would be worse than an occasionally-wrong option.
    expect(values('codex_cli')).toContain('high')
    expect(values('claude_code')).toContain('xhigh')
    expect(values('opencode', [])).toContain('high')
  })

  it('drops efforts the model rejects', () => {
    // qwen3.8-27b: the provider 400s on "high" and accepts "xhigh". Offering
    // "high" is what aborted a real spec-task turn.
    const narrowed = values('opencode', ['none', 'low', 'medium', 'xhigh'])
    expect(narrowed).not.toContain('high')
    expect(narrowed).toContain('xhigh')
    expect(narrowed).toContain('medium')
    expect(narrowed).toContain('low')
  })

  it('always keeps "default", which means send nothing rather than a provider value', () => {
    expect(values('opencode', ['xhigh'])).toContain('default')
    expect(values('claude_code', ['low'])).toContain('default')
  })

  it('narrows the claude list to a 4.6-generation model that rejects xhigh', () => {
    const narrowed = values('claude_code', ['low', 'medium', 'high', 'max'])
    expect(narrowed).not.toContain('xhigh')
    expect(narrowed).toContain('max')
    expect(narrowed).toContain('high')
  })

  it('falls back to the runtime list when narrowing would leave only "default"', () => {
    // A profile listing nothing the harness can express should not produce a
    // selector with a single meaningless entry.
    const narrowed = values('codex_cli', ['some-effort-the-harness-cannot-send'])
    expect(narrowed.length).toBeGreaterThan(1)
    expect(narrowed).toContain('high')
  })

  it('matches effort values case-insensitively', () => {
    expect(values('opencode', ['XHigh', 'MEDIUM'])).toEqual(
      expect.arrayContaining(['default', 'medium', 'xhigh']),
    )
  })
})
