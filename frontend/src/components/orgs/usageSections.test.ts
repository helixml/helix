import { describe, expect, it } from 'vitest'

import { usageSection, usageSectionRoute } from './usageSections'

describe('usage sections', () => {
  it('falls back to the overview for an unknown section', () => {
    expect(usageSection('compute').key).toBe('compute')
    expect(usageSection('nope').key).toBe('overview')
    expect(usageSection(undefined).key).toBe('overview')
  })

  it('keeps the date range and filters when moving between sections', () => {
    const search = '?from=2026-11-08&to=2026-11-18&project_id=prj_1&table=users'
    expect(usageSectionRoute('compute', 'acme', search)).toEqual({
      name: 'org_usage_section',
      params: { from: '2026-11-08', to: '2026-11-18', project_id: 'prj_1', org_id: 'acme', section: 'compute' },
    })
    // The overview has no section segment; the breakdown tab never leaks out of Breakdowns.
    expect(usageSectionRoute('overview', 'acme', search)).toEqual({
      name: 'org_usage',
      params: { from: '2026-11-08', to: '2026-11-18', project_id: 'prj_1', org_id: 'acme' },
    })
  })

  it('cannot be pointed at another org or section by the query string', () => {
    const route = usageSectionRoute('tokens', 'acme', '?org_id=evil&section=compute')
    expect(route.params).toMatchObject({ org_id: 'acme', section: 'tokens' })
  })
})
