import { beforeEach, describe, expect, it } from 'vitest'

import { consumeOrgBotChatDraft, queueOrgBotChatDraft } from './orgBotChatDraft'

describe('orgBotChatDraft', () => {
  beforeEach(() => sessionStorage.clear())

  it('hands a queued draft to the matching bot once', () => {
    queueOrgBotChatDraft('my-org', 'chief-of-staff', 'I would like to create a new bot')

    expect(consumeOrgBotChatDraft('my-org', 'chief-of-staff'))
      .toBe('I would like to create a new bot')
    expect(consumeOrgBotChatDraft('my-org', 'chief-of-staff')).toBe('')
  })

  it('keeps drafts isolated by organization and bot', () => {
    queueOrgBotChatDraft('my-org', 'chief-of-staff', 'draft')

    expect(consumeOrgBotChatDraft('other-org', 'chief-of-staff')).toBe('')
    expect(consumeOrgBotChatDraft('my-org', 'other-bot')).toBe('')
    expect(consumeOrgBotChatDraft('my-org', 'chief-of-staff')).toBe('draft')
  })
})
