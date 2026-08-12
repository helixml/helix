export const CHAT_SIDEBAR_MIN_WIDTH = 280
export const CHAT_SIDEBAR_MAX_WIDTH = 640
export const CHAT_SIDEBAR_DEFAULT_WIDTH = 360

export const chatSidebarWidthStorageKey = (orgId: string): string => (
  `helix:project-chat-sidebar:width:${orgId}`
)

export const clampChatSidebarWidth = (
  value: number,
  fallback = CHAT_SIDEBAR_DEFAULT_WIDTH,
): number => {
  const normalized = Number.isFinite(value) ? value : fallback
  return Math.min(CHAT_SIDEBAR_MAX_WIDTH, Math.max(CHAT_SIDEBAR_MIN_WIDTH, Math.round(normalized)))
}

export const parseChatSidebarWidth = (
  storedValue: string | null,
  fallback = CHAT_SIDEBAR_DEFAULT_WIDTH,
): number => {
  if (storedValue === null) return clampChatSidebarWidth(fallback, fallback)
  const parsed = Number(storedValue)
  return clampChatSidebarWidth(parsed, fallback)
}
