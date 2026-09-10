const STORAGE_KEY_PREFIX = 'helix_org_bot_chat_draft'

const storageKey = (organizationId: string, botId: string): string =>
  `${STORAGE_KEY_PREFIX}:${organizationId}:${botId}`

export function queueOrgBotChatDraft(
  organizationId: string,
  botId: string,
  content: string,
): void {
  try {
    sessionStorage.setItem(storageKey(organizationId, botId), content)
  } catch (error) {
    console.warn('Failed to queue org bot chat draft:', error)
  }
}

export function consumeOrgBotChatDraft(
  organizationId: string,
  botId: string,
): string {
  try {
    const key = storageKey(organizationId, botId)
    const content = sessionStorage.getItem(key) ?? ''
    sessionStorage.removeItem(key)
    return content
  } catch (error) {
    console.warn('Failed to consume org bot chat draft:', error)
    return ''
  }
}
