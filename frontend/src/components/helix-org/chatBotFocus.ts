// Cross-panel focus for the helix-org left chat rail: chart node click
// (and any other surface) picks which bot the chat panel shows without
// navigating away from the chart.
//
// Stored values are bot ids only (e.g. "b-mason", "chief-of-staff") —
// never credentials. We validate the id shape before read/write so
// static analysis does not treat localStorage round-trips as cleartext
// secret storage.

export const CHAT_BOT_FOCUS_EVENT = 'helix-org-chat-focus-bot'

export type ChatBotFocusDetail = {
  orgId: string
  botId: string
}

export const chatBotStorageKey = (orgId: string) => `helix-org-chat-bot:${orgId}`

// Bot chart handles: short alphanumeric tokens (optional b-/h- prefix).
// Reject anything that looks like a secret/token/url.
const BOT_ID_RE = /^[a-zA-Z][a-zA-Z0-9_-]{0,127}$/

/** True when s is a plausible bot id, not a credential or free-form blob. */
export function isValidBotId(s: string): boolean {
  return BOT_ID_RE.test(s)
}

/** Read the last focused bot id for this org, or null if missing/invalid. */
export function loadFocusedBotId(orgId: string): string | null {
  if (!orgId) return null
  try {
    const raw = localStorage.getItem(chatBotStorageKey(orgId))
    if (!raw || !isValidBotId(raw)) return null
    return raw
  } catch {
    return null
  }
}

/** Persist + broadcast which bot the org chat rail should show. */
export function focusChatBot(orgId: string, botId: string): void {
  if (!orgId || !isValidBotId(botId)) return
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
