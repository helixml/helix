export const CHAT_TURN_NAVIGATOR_MIN_ITEMS = 1
export const CHAT_TURN_NAVIGATOR_ITEM_SPACING = 8

export interface ChatTurnNavigatorItem {
  id: string
  userText: string | null
  assistantText: string | null
}

export const compactChatTurnPreview = (text: string | null | undefined) => {
  const compact = text?.replace(/\s+/g, ' ').trim() ?? ''
  return compact || null
}

export const resolveChatTurnAssistantPreview = (
  responseMessage: string | null | undefined,
  responseEntries: Array<{ type?: string; content?: string }> | null | undefined,
) => {
  const legacyResponse = compactChatTurnPreview(responseMessage)
  if (legacyResponse) return legacyResponse

  for (let index = (responseEntries?.length ?? 0) - 1; index >= 0; index -= 1) {
    const entry = responseEntries?.[index]
    if (entry?.type !== 'text' || typeof entry.content !== 'string') continue
    const withoutThinking = entry.content
      .replace(/<thinking>[\s\S]*?<\/thinking>/gi, ' ')
      .replace(/<think>[\s\S]*?<\/think>/gi, ' ')
    const preview = compactChatTurnPreview(withoutThinking)
    if (preview) return preview
  }
  return null
}

export const resolveChatTurnNavigatorTopPercent = (index: number, itemCount: number) => {
  if (itemCount <= 1) return 0
  const boundedIndex = Math.max(0, Math.min(index, itemCount - 1))
  return (boundedIndex / (itemCount - 1)) * 100
}

export const resolveChatTurnNavigatorIndexFromPointer = ({
  itemCount,
  railTop,
  railHeight,
  pointerY,
}: {
  itemCount: number
  railTop: number
  railHeight: number
  pointerY: number
}) => {
  if (itemCount <= 0 || railHeight <= 0) return null
  if (itemCount === 1) return 0
  const progress = Math.max(0, Math.min(1, (pointerY - railTop) / railHeight))
  return Math.max(0, Math.min(itemCount - 1, Math.round(progress * (itemCount - 1))))
}
