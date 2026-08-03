import { describe, expect, it } from 'vitest'

import {
  compactChatTurnPreview,
  resolveChatTurnAssistantPreview,
  resolveChatTurnNavigatorIndexFromPointer,
  resolveChatTurnNavigatorTopPercent,
} from './ChatTurnNavigator.logic'

describe('ChatTurnNavigator', () => {
  it('compacts message previews without inventing fallback text', () => {
    expect(compactChatTurnPreview('  A message\n\nwith   spacing  ')).toBe('A message with spacing')
    expect(compactChatTurnPreview('   ')).toBeNull()
    expect(compactChatTurnPreview(undefined)).toBeNull()
  })

  it('uses the latest assistant prose and skips tool calls or thinking-only entries', () => {
    expect(resolveChatTurnAssistantPreview('', [
      { type: 'text', content: '<thinking>private reasoning</thinking>Earlier update.' },
      { type: 'tool_call', content: 'Tool output' },
      { type: 'text', content: '<thinking>more private reasoning</thinking>' },
    ])).toBe('Earlier update.')
    expect(resolveChatTurnAssistantPreview('Legacy response', [])).toBe('Legacy response')
  })

  it('distributes markers evenly from the start to the end of the rail', () => {
    expect(resolveChatTurnNavigatorTopPercent(0, 5)).toBe(0)
    expect(resolveChatTurnNavigatorTopPercent(2, 5)).toBe(50)
    expect(resolveChatTurnNavigatorTopPercent(4, 5)).toBe(100)
  })

  it('selects the nearest marker and clamps pointers outside the rail', () => {
    const input = { itemCount: 5, railTop: 100, railHeight: 80 }
    expect(resolveChatTurnNavigatorIndexFromPointer({ ...input, pointerY: 100 })).toBe(0)
    expect(resolveChatTurnNavigatorIndexFromPointer({ ...input, pointerY: 141 })).toBe(2)
    expect(resolveChatTurnNavigatorIndexFromPointer({ ...input, pointerY: 500 })).toBe(4)
    expect(resolveChatTurnNavigatorIndexFromPointer({ ...input, pointerY: 0 })).toBe(0)
  })
})
