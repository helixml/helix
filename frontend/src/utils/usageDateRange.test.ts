import { describe, expect, it } from 'vitest'

import { buildTaskUsageQuery, initialUsageParam, usageRangeFrom } from './usageDateRange'

describe('usage date ranges', () => {
  const now = new Date('2026-08-13T12:00:00.000Z')

  it('builds the inclusive seven-day task usage query', () => {
    expect(buildTaskUsageQuery('spt_123', now)).toEqual({
      from: '2026-08-07',
      to: '2026-08-13',
      task_id: 'spt_123',
    })
  })

  it('uses an inclusive range for usage presets', () => {
    expect(usageRangeFrom(30, now)).toBe('2026-07-15')
  })

  it('prefers router parameters during internal navigation', () => {
    const search = new URLSearchParams('task_id=stale-task')

    expect(initialUsageParam({ task_id: 'current-task' }, search, 'task_id')).toBe('current-task')
    expect(initialUsageParam({}, search, 'task_id')).toBe('stale-task')
  })
})
