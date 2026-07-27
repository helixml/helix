import { describe, expect, it } from 'vitest'

import { isTranscriptTopic } from './helixOrgTopics'

describe('helix org transcript topics', () => {
  it('identifies only canonical transcript topic ids', () => {
    expect(isTranscriptTopic('s-transcript-b-writer')).toBe(true)
    expect(isTranscriptTopic('s-team-b-writer')).toBe(false)
    expect(isTranscriptTopic()).toBe(false)
  })
})
