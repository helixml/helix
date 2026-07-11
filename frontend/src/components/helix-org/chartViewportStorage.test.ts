import { describe, it, expect, beforeEach } from 'vitest'

import { loadChartViewport, saveChartViewport } from './chartViewportStorage'

describe('chartViewportStorage', () => {
  const userId = 'usr_test'
  const orgId = 'org_test'
  const key = `helix.orgChart.viewport.${userId}.${orgId}`

  beforeEach(() => {
    window.localStorage.clear()
  })

  it('returns null when nothing is stored', () => {
    expect(loadChartViewport(userId, orgId)).toBeNull()
  })

  it('returns null for empty user or org id', () => {
    saveChartViewport(userId, orgId, { x: 1, y: 2, zoom: 1 })
    expect(loadChartViewport('', orgId)).toBeNull()
    expect(loadChartViewport(userId, '')).toBeNull()
  })

  it('round-trips a viewport', () => {
    saveChartViewport(userId, orgId, { x: 120.5, y: -40, zoom: 0.85 })
    expect(loadChartViewport(userId, orgId)).toEqual({
      x: 120.5,
      y: -40,
      zoom: 0.85,
    })
    expect(window.localStorage.getItem(key)).toContain('"zoom":0.85')
  })

  it('scopes by user and org', () => {
    saveChartViewport(userId, orgId, { x: 1, y: 2, zoom: 1 })
    expect(loadChartViewport('other_user', orgId)).toBeNull()
    expect(loadChartViewport(userId, 'other_org')).toBeNull()
  })

  it('rejects invalid stored payloads', () => {
    window.localStorage.setItem(key, JSON.stringify({ x: 1, y: 2 }))
    expect(loadChartViewport(userId, orgId)).toBeNull()
    window.localStorage.setItem(key, JSON.stringify({ x: 1, y: 2, zoom: 0 }))
    expect(loadChartViewport(userId, orgId)).toBeNull()
    window.localStorage.setItem(key, 'not-json')
    expect(loadChartViewport(userId, orgId)).toBeNull()
  })

  it('does not write when ids are missing or viewport is invalid', () => {
    saveChartViewport('', orgId, { x: 1, y: 2, zoom: 1 })
    saveChartViewport(userId, '', { x: 1, y: 2, zoom: 1 })
    saveChartViewport(userId, orgId, { x: 1, y: 2, zoom: -1 } as any)
    expect(window.localStorage.length).toBe(0)
  })
})
