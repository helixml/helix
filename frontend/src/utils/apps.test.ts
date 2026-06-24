import { describe, it, expect } from 'vitest'
import { isExternalAgent, isHelixOrgChartAgent, isSpecTaskSwitchableAgent } from './apps'
import { IApp, AGENT_TYPE_ZED_EXTERNAL, AGENT_TYPE_HELIX_AGENT } from '../types'

// Minimal IApp builder — only the fields the predicates read.
const makeApp = (opts: {
  agentType?: string
  defaultAgentType?: string
  isHelixOrgAgent?: boolean
}): IApp =>
  ({
    id: 'app_test',
    config: {
      helix: {
        assistants: opts.agentType ? [{ agent_type: opts.agentType }] : [],
        default_agent_type: opts.defaultAgentType,
      },
    },
    is_helix_org_agent: opts.isHelixOrgAgent,
  } as unknown as IApp)

describe('isExternalAgent', () => {
  it('is true when an assistant is zed_external', () => {
    expect(isExternalAgent(makeApp({ agentType: AGENT_TYPE_ZED_EXTERNAL }))).toBe(true)
  })

  it('is true when default_agent_type is zed_external', () => {
    expect(isExternalAgent(makeApp({ defaultAgentType: AGENT_TYPE_ZED_EXTERNAL }))).toBe(true)
  })

  it('is false for a non-external agent', () => {
    expect(isExternalAgent(makeApp({ agentType: AGENT_TYPE_HELIX_AGENT }))).toBe(false)
  })
})

describe('isSpecTaskSwitchableAgent', () => {
  it('keeps an external agent that is not part of the org chart', () => {
    expect(
      isSpecTaskSwitchableAgent(makeApp({ agentType: AGENT_TYPE_ZED_EXTERNAL })),
    ).toBe(true)
  })

  it('drops an external agent that backs an org-chart Worker', () => {
    const app = makeApp({ agentType: AGENT_TYPE_ZED_EXTERNAL, isHelixOrgAgent: true })
    expect(isHelixOrgChartAgent(app)).toBe(true)
    expect(isSpecTaskSwitchableAgent(app)).toBe(false)
  })

  it('drops a non-external agent', () => {
    expect(
      isSpecTaskSwitchableAgent(makeApp({ agentType: AGENT_TYPE_HELIX_AGENT })),
    ).toBe(false)
  })
})
