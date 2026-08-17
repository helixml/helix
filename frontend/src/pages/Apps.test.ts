import { describe, expect, it } from 'vitest'

import { AGENT_KIND_CODING, AGENT_KIND_HELIX, AGENT_KIND_ORG } from '../types'
import { agentTabs, DEFAULT_AGENT_KIND } from './Apps'

describe('organization agent tabs', () => {
  it('puts legacy coding agents last', () => {
    expect(agentTabs.map((tab) => tab.value)).toEqual([
      AGENT_KIND_HELIX,
      AGENT_KIND_ORG,
      AGENT_KIND_CODING,
    ])
    expect(agentTabs[2].label).toBe('Coding Agents (legacy)')
    expect(DEFAULT_AGENT_KIND).toBe(AGENT_KIND_HELIX)
  })
})
