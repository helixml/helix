import { describe, expect, it } from 'vitest'

import { AGENT_KIND_HELIX } from '../types'
import { DEFAULT_AGENT_KIND } from './Apps'

describe('organization agents page', () => {
  it('seeds the new-agent dialog with the helix kind', () => {
    // The page shows every agent in one list, so there is no selected tab to
    // carry a kind — the dialog starts on the native Helix agent.
    expect(DEFAULT_AGENT_KIND).toBe(AGENT_KIND_HELIX)
  })
})
