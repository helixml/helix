import { describe, it, expect } from 'vitest'
import { isChatSelectableAgent, isCodingAgent, isHelixAgent, isOrgAgent, selectCodingAgents, usesFocusedAgentDetails } from './apps'
import { AGENT_KIND_CODING, AGENT_KIND_HELIX, AGENT_KIND_ORG, IApp } from '../types'

// Minimal IApp builder — only the fields the predicates read.
const makeApp = (opts: {
  agentKind: string
}): IApp =>
  ({
    id: 'app_test',
    config: {
      helix: {
        assistants: [],
      },
    },
    agent_kind: opts.agentKind,
  } as unknown as IApp)

describe('agent kind predicates', () => {
  it('uses the persisted kind instead of execution config', () => {
    expect(isHelixAgent(makeApp({ agentKind: AGENT_KIND_HELIX }))).toBe(true)
    expect(isCodingAgent(makeApp({ agentKind: AGENT_KIND_CODING }))).toBe(true)
    expect(isOrgAgent(makeApp({ agentKind: AGENT_KIND_ORG }))).toBe(true)
  })

  it('uses focused details only for coding and org agents', () => {
    expect(usesFocusedAgentDetails(makeApp({ agentKind: AGENT_KIND_CODING }))).toBe(true)
    expect(usesFocusedAgentDetails(makeApp({ agentKind: AGENT_KIND_ORG }))).toBe(true)
    expect(usesFocusedAgentDetails(makeApp({ agentKind: AGENT_KIND_HELIX }))).toBe(false)
    expect(usesFocusedAgentDetails(makeApp({ agentKind: 'future_agent_kind' }))).toBe(false)
  })
})

describe('selectCodingAgents', () => {
  it('keeps external agents that are not part of the org chart', () => {
    expect(
      selectCodingAgents([makeApp({ agentKind: AGENT_KIND_CODING })]).length,
    ).toBe(1)
  })

  it('drops external agents that back an org-chart Worker', () => {
    const app = makeApp({ agentKind: AGENT_KIND_ORG })
    expect(isOrgAgent(null)).toBe(false)
    expect(isOrgAgent(app)).toBe(true)
    expect(selectCodingAgents([app])).toEqual([])
  })

  it('drops non-external agents', () => {
    expect(selectCodingAgents([makeApp({ agentKind: AGENT_KIND_HELIX })])).toEqual([])
  })

  it('tolerates a missing list', () => {
    expect(selectCodingAgents(undefined)).toEqual([])
    expect(selectCodingAgents(null)).toEqual([])
  })
})

describe('isChatSelectableAgent', () => {
  it('only permits native Helix agents', () => {
    expect(isChatSelectableAgent(makeApp({ agentKind: AGENT_KIND_HELIX }))).toBe(true)
    expect(isChatSelectableAgent(makeApp({ agentKind: AGENT_KIND_CODING }))).toBe(false)
    expect(isChatSelectableAgent(makeApp({ agentKind: AGENT_KIND_ORG }))).toBe(false)
  })
})
