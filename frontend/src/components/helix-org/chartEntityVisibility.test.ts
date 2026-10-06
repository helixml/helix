import { beforeEach, describe, expect, it } from 'vitest'

import {
  loadHiddenChartEntityIDs,
  saveHiddenChartEntityIDs,
  selectedChartEntityIDs,
} from './chartEntityVisibility'

describe('chartEntityVisibility', () => {
  const userID = 'usr_test'
  const orgID = 'org_test'

  beforeEach(() => {
    window.localStorage.clear()
  })

  it('round-trips hidden entities and scopes them by user and org', () => {
    saveHiddenChartEntityIDs(userID, orgID, {
      agents: ['bot_one'],
      processors: ['processor_one'],
      assets: ['asset_one'],
    })

    expect(loadHiddenChartEntityIDs(userID, orgID)).toEqual({
      agents: ['bot_one'],
      processors: ['processor_one'],
      assets: ['asset_one'],
    })
    expect(loadHiddenChartEntityIDs('other_user', orgID)).toBeNull()
    expect(loadHiddenChartEntityIDs(userID, 'other_org')).toBeNull()
  })

  it('rejects incomplete and invalid settings', () => {
    const key = `helix.orgChart.entityVisibility.${userID}.${orgID}`
    window.localStorage.setItem(key, JSON.stringify({ agents: [], processors: [] }))
    expect(loadHiddenChartEntityIDs(userID, orgID)).toBeNull()
    window.localStorage.setItem(key, 'not-json')
    expect(loadHiddenChartEntityIDs(userID, orgID)).toBeNull()
  })

  it('defaults fresh charts to agents only and applies saved settings exactly', () => {
    expect(selectedChartEntityIDs('agents', ['bot_one'], null)).toEqual(['bot_one'])
    expect(selectedChartEntityIDs('processors', ['processor_one'], null)).toEqual([])
    expect(selectedChartEntityIDs('assets', ['asset_one'], null)).toEqual([])

    const saved = { agents: ['bot_one'], processors: [], assets: ['asset_one'] }
    expect(selectedChartEntityIDs('agents', ['bot_one', 'bot_two'], saved)).toEqual(['bot_two'])
    expect(selectedChartEntityIDs('processors', ['processor_one'], saved)).toEqual(['processor_one'])
    expect(selectedChartEntityIDs('assets', ['asset_one'], saved)).toEqual([])
  })
})
