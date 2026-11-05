import { describe, expect, it } from 'vitest'

import { LIST_INTERACTIONS_QUERY_KEY } from './sessionService'

describe('LIST_INTERACTIONS_QUERY_KEY', () => {
  it('preserves the existing key for normal interaction lists', () => {
    expect(LIST_INTERACTIONS_QUERY_KEY('session-1', 0, 5, 'desc')).toEqual([
      'interactions',
      'session-1',
      0,
      5,
      'desc',
    ])
  })

  it('separates pending-question lists from normal interaction lists', () => {
    expect(LIST_INTERACTIONS_QUERY_KEY('session-1', 0, 5, 'desc', true)).toEqual([
      'interactions',
      'session-1',
      0,
      5,
      'desc',
      'pending_question',
    ])
  })
})
