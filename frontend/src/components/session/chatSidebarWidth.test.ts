import { describe, expect, it } from 'vitest'

import {
  CHAT_SIDEBAR_MAX_WIDTH,
  CHAT_SIDEBAR_MIN_WIDTH,
  chatSidebarWidthStorageKey,
  clampChatSidebarWidth,
  parseChatSidebarWidth,
} from './chatSidebarWidth'

describe('chat sidebar width storage', () => {
  it('uses an organization-scoped storage key', () => {
    expect(chatSidebarWidthStorageKey('unmanned-org')).toBe(
      'helix:project-chat-sidebar:width:unmanned-org',
    )
  })

  it('clamps invalid and out-of-range widths', () => {
    expect(clampChatSidebarWidth(319.6)).toBe(320)
    expect(parseChatSidebarWidth(String(CHAT_SIDEBAR_MIN_WIDTH - 1))).toBe(CHAT_SIDEBAR_MIN_WIDTH)
    expect(parseChatSidebarWidth(String(CHAT_SIDEBAR_MAX_WIDTH + 1))).toBe(CHAT_SIDEBAR_MAX_WIDTH)
    expect(parseChatSidebarWidth('not-a-width', 400)).toBe(400)
  })
})
