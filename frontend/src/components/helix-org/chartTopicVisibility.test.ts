import { beforeEach, describe, expect, it } from 'vitest'

import {
  DEFAULT_CHART_TOPIC_FILTERS,
  chartTopicFilterFor,
  loadChartTopicVisibility,
  saveChartTopicVisibility,
} from './chartTopicVisibility'

describe('chartTopicVisibility', () => {
  const userId = 'usr_test'
  const orgId = 'org_test'

  beforeEach(() => {
    window.localStorage.clear()
  })

  it('hides direct messages, local topics, and other topics by default', () => {
    expect(DEFAULT_CHART_TOPIC_FILTERS).toEqual([
      'webhook',
      'github',
      'gitlab',
      'postmark',
      'cron',
    ])
  })

  it('separates direct messages from other local topics', () => {
    expect(chartTopicFilterFor({ name: 'dm: b-mason ↔ chief-of-staff', kind: 'local' })).toBe('direct_messages')
    expect(chartTopicFilterFor({ name: 'Engineering', kind: 'local' })).toBe('local')
  })

  it('classifies every topic transport exposed by the creation dialog', () => {
    expect(chartTopicFilterFor({ name: 'Outbound', kind: 'webhook' })).toBe('webhook')
    expect(chartTopicFilterFor({ name: 'Pull requests', kind: 'github' })).toBe('github')
    expect(chartTopicFilterFor({ name: 'Merge requests', kind: 'gitlab' })).toBe('gitlab')
    expect(chartTopicFilterFor({ name: 'Inbox', kind: 'postmark' })).toBe('postmark')
    expect(chartTopicFilterFor({ name: 'Daily report', kind: 'cron' })).toBe('cron')
  })

  it('maps the backend email kind to Postmark and unknown kinds to Other', () => {
    expect(chartTopicFilterFor({ name: 'Inbox', kind: 'email' })).toBe('postmark')
    expect(chartTopicFilterFor({ name: 'Slack alerts', kind: 'slack' })).toBe('other')
  })

  it('round-trips filters in canonical order and scopes them by user and org', () => {
    saveChartTopicVisibility(userId, orgId, ['cron', 'direct_messages'])

    expect(loadChartTopicVisibility(userId, orgId)).toEqual(['direct_messages', 'cron'])
    expect(loadChartTopicVisibility('other_user', orgId)).toBeNull()
    expect(loadChartTopicVisibility(userId, 'other_org')).toBeNull()
  })

  it('returns null for missing or invalid settings', () => {
    const key = `helix.orgChart.topicVisibility.${userId}.${orgId}`
    expect(loadChartTopicVisibility(userId, orgId)).toBeNull()
    window.localStorage.setItem(key, JSON.stringify(['cron', 'not-a-filter']))
    expect(loadChartTopicVisibility(userId, orgId)).toBeNull()
    window.localStorage.setItem(key, 'not-json')
    expect(loadChartTopicVisibility(userId, orgId)).toBeNull()
  })
})
