import { describe, it, expect } from 'vitest'
import { isCodingAgent, isHelixAgent, isOrgAgent, isSpecTaskSwitchableAgent, usesFocusedAgentDetails } from './apps'
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

describe('isSpecTaskSwitchableAgent', () => {
  it('keeps an external agent that is not part of the org chart', () => {
    expect(
      isSpecTaskSwitchableAgent(makeApp({ agentKind: AGENT_KIND_CODING })),
    ).toBe(true)
  })

  it('drops an external agent that backs an org-chart Worker', () => {
    const app = makeApp({ agentKind: AGENT_KIND_ORG })
    expect(isOrgAgent(null)).toBe(false)
    expect(isOrgAgent(app)).toBe(true)
    expect(isSpecTaskSwitchableAgent(app)).toBe(false)
  })

  it('drops a non-external agent', () => {
    expect(
      isSpecTaskSwitchableAgent(makeApp({ agentKind: AGENT_KIND_HELIX })),
    ).toBe(false)
  })
})
