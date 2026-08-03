import { beforeEach, describe, expect, it } from 'vitest'

import {
  loadHiddenChartEntityIDs,
  saveHiddenChartEntityIDs,
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
})
