import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'

const source = readFileSync('src/services/helixOrgService.ts', 'utf8')
const triggerSource = readFileSync('src/services/triggerService.ts', 'utf8')

const count = (pattern: RegExp) => source.match(pattern)?.length ?? 0
const countTriggers = (pattern: RegExp) => triggerSource.match(pattern)?.length ?? 0

describe('Helix Org Bot API argument order', () => {
  it('passes Bot and organization IDs in generated-client order', () => {
    expect(count(/v1OrgsBots[A-Za-z0-9]*\(/g)).toBe(16)
    expect(count(/v1OrgsBotsDetail\(orgID\)/g)).toBe(1)
    expect(count(/v1OrgsBotsCreate\(orgID, payload\)/g)).toBe(1)
    expect(count(/v1OrgsBotsPartialUpdate\(orgID, id, body\)/g)).toBe(1)
    expect(count(/v1OrgsBotsChatCreate\(botId, orgID\)/g)).toBe(1)
    expect(count(/v1OrgsBotsActivateCreate\(botId, orgID\)/g)).toBe(1)
    expect(count(/v1OrgsBotsStopCreate\(botId, orgID\)/g)).toBe(1)
    expect(count(/v1OrgsBotsRestartCreate\(botId, orgID\)/g)).toBe(1)
    expect(count(/v1OrgsBotsDetail2\(botId, orgID\)/g)).toBe(2)
    expect(count(/v1OrgsBotsParentsCreate\(botID, orgID, \{ parent_id: parentID \}\)/g)).toBe(1)
    expect(count(/v1OrgsBotsParentsDelete\(botID, parentID, orgID\)/g)).toBe(1)
    expect(count(/v1OrgsBotsDelete\(botId, orgID\)/g)).toBe(1)
    expect(count(/v1OrgsBotsSecretsDetail\(orgID, botID!\)/g)).toBe(1)
    expect(count(/v1OrgsBotsAvailableSecretsDetail\(orgID, botID!\)/g)).toBe(1)
    expect(count(/v1OrgsBotsSecretsUpdate\(orgID, botID!, input.name, input.payload\)/g)).toBe(1)
    expect(count(/v1OrgsBotsSecretsDelete\(orgID, botID!, name\)/g)).toBe(1)
  })

  it('passes organization ID before Bot and Trigger IDs on attachments', () => {
    expect(countTriggers(/v1OrgsBotsAttachments[A-Za-z0-9]*\(/g)).toBe(6)
    expect(countTriggers(/v1OrgsBotsAttachmentsDetail\(orgID, botID!?\)/g)).toBe(2)
    expect(countTriggers(/v1OrgsBotsAttachmentsCreate\(orgID, botID!?, [^)]+\)/g)).toBe(2)
    expect(countTriggers(/v1OrgsBotsAttachmentsDelete\(orgID, botID!?, attachmentID\)/g)).toBe(2)
    expect(countTriggers(/v1OrgsTriggers[A-Za-z0-9]*\(orgID/g)).toBeGreaterThan(0)
  })
})
