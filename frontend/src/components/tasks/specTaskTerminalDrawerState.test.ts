import { describe, expect, it } from 'vitest'

import {
  clampSpecTaskTerminalHeight,
  DEFAULT_SPEC_TASK_TERMINAL_HEIGHT,
  isSpecTaskTerminalToggleShortcut,
  loadSpecTaskTerminalDrawerState,
  saveSpecTaskTerminalDrawerState,
  specTaskTerminalDrawerStorageKey,
} from './specTaskTerminalDrawerState'

function memoryStorage() {
  const values = new Map<string, string>()
  return {
    getItem: (key: string) => values.get(key) ?? null,
    setItem: (key: string, value: string) => values.set(key, value),
    values,
  }
}

describe('spec task terminal drawer state', () => {
  it('persists open state and a clamped height per task', () => {
    const storage = memoryStorage()

    saveSpecTaskTerminalDrawerState(
      'spt_one',
      { open: true, height: 10_000 },
      storage,
    )

    expect(loadSpecTaskTerminalDrawerState('spt_one', storage)).toEqual({
      open: true,
      height: clampSpecTaskTerminalHeight(10_000),
    })
    expect(storage.values.has(specTaskTerminalDrawerStorageKey('spt_two'))).toBe(false)
    expect(loadSpecTaskTerminalDrawerState('spt_two', storage)).toEqual({
      open: false,
      height: DEFAULT_SPEC_TASK_TERMINAL_HEIGHT,
    })
  })

  it('clamps invalid and undersized heights', () => {
    expect(clampSpecTaskTerminalHeight(Number.NaN, 1000)).toBe(
      DEFAULT_SPEC_TASK_TERMINAL_HEIGHT,
    )
    expect(clampSpecTaskTerminalHeight(20, 1000)).toBe(180)
    expect(clampSpecTaskTerminalHeight(900, 1000)).toBe(700)
  })

  it('matches the T3-style Ctrl/Cmd+J shortcut without extra modifiers', () => {
    const event = {
      key: 'j',
      ctrlKey: true,
      metaKey: false,
      altKey: false,
      shiftKey: false,
    }
    expect(isSpecTaskTerminalToggleShortcut(event)).toBe(true)
    expect(isSpecTaskTerminalToggleShortcut({ ...event, ctrlKey: false, metaKey: true })).toBe(true)
    expect(isSpecTaskTerminalToggleShortcut({ ...event, shiftKey: true })).toBe(false)
    expect(isSpecTaskTerminalToggleShortcut({ ...event, key: 'k' })).toBe(false)
  })
})
