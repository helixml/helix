// Cross-panel focus for the helix-org left chat rail: chart node click
// (and any other surface) picks which bot the chat panel shows without
// navigating away from the chart.

export const CHAT_BOT_FOCUS_EVENT = 'helix-org-chat-focus-bot'

export type ChatBotFocusDetail = {
  orgId: string
  botId: string
}

export const chatBotStorageKey = (orgId: string) => `helix-org-chat-bot:${orgId}`

/** Persist + broadcast which bot the org chat rail should show. */
export function focusChatBot(orgId: string, botId: string): void {
  if (!orgId || !botId) return
  try {
    localStorage.setItem(chatBotStorageKey(orgId), botId)
  } catch {
    // private mode / quota — still broadcast so in-memory listeners work
  }
  window.dispatchEvent(
    new CustomEvent<ChatBotFocusDetail>(CHAT_BOT_FOCUS_EVENT, {
      detail: { orgId, botId },
    }),
  )
}
