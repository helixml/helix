import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'

const source = readFileSync('src/services/helixOrgService.ts', 'utf8')

const count = (pattern: RegExp) => source.match(pattern)?.length ?? 0

describe('Helix Org Agent API argument order', () => {
  it('always passes organization ID before Agent and relationship IDs', () => {
    expect(count(/v1OrgsAgents[A-Za-z0-9]*\(/g)).toBe(21)
    expect(count(/v1OrgsAgentsDetail\(orgID\)/g)).toBe(1)
    expect(count(/v1OrgsAgentsCreate\(orgID, payload\)/g)).toBe(1)
    expect(count(/v1OrgsAgentsPartialUpdate\(orgID, id, body\)/g)).toBe(1)
    expect(count(/v1OrgsAgentsChatCreate\(orgID, botId\)/g)).toBe(1)
    expect(count(/v1OrgsAgentsActivateCreate\(orgID, botId\)/g)).toBe(1)
    expect(count(/v1OrgsAgentsStopAgentCreate\(orgID, botId\)/g)).toBe(1)
    expect(count(/v1OrgsAgentsRestartAgentCreate\(orgID, botId\)/g)).toBe(1)
    expect(count(/v1OrgsAgentsDetail2\(orgID, botId\)/g)).toBe(2)
    expect(count(/v1OrgsAgentsParentsCreate\(orgID, botID, \{ parent_id: parentID \}\)/g)).toBe(1)
    expect(count(/v1OrgsAgentsParentsDelete\(orgID, botID, parentID\)/g)).toBe(1)
    expect(count(/v1OrgsAgentsSubscriptionsDetail\(orgID, botID\)/g)).toBe(1)
    expect(count(/v1OrgsAgentsSubscriptionsCreate\(orgID, botID, \{ topic_id: topicID \}\)/g)).toBe(2)
    expect(count(/v1OrgsAgentsSubscriptionsDelete\(orgID, botID, topicID\)/g)).toBe(2)
    expect(count(/v1OrgsAgentsDelete\(orgID, botId\)/g)).toBe(1)
    expect(count(/v1OrgsAgentsSecretsDetail\(orgID, agentID!\)/g)).toBe(1)
    expect(count(/v1OrgsAgentsAvailableSecretsDetail\(orgID, agentID!\)/g)).toBe(1)
    expect(count(/v1OrgsAgentsSecretsUpdate\(orgID, agentID!, input.name, input.payload\)/g)).toBe(1)
    expect(count(/v1OrgsAgentsSecretsDelete\(orgID, agentID!, name\)/g)).toBe(1)
  })
})
