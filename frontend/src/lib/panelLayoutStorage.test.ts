import { describe, expect, it, beforeEach } from 'vitest'

import { loadPanelLayout, savePanelLayout } from './panelLayoutStorage'

const storageKey = 'helix.panel-layout.test'
const panelIds = ['chat', 'content'] as const

describe('panelLayoutStorage', () => {
  beforeEach(() => localStorage.clear())

  it('round-trips a valid layout', () => {
    const layout = { chat: 62.5, content: 37.5 }

    savePanelLayout(storageKey, layout, panelIds)

    expect(loadPanelLayout(storageKey, panelIds)).toEqual(layout)
  })

  it('rejects layouts for a different panel group', () => {
    savePanelLayout(storageKey, { chat: 62.5, content: 37.5 }, panelIds)

    expect(loadPanelLayout(storageKey, ['left', 'right'])).toBeNull()
  })

  it('ignores malformed stored layouts', () => {
    localStorage.setItem(storageKey, JSON.stringify({ chat: 90, content: 5 }))

    expect(loadPanelLayout(storageKey, panelIds)).toBeNull()
  })
})
