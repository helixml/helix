import { describe, it, expect } from 'vitest'
import { seedPromptIndex, shouldShowWelcome } from './minimalChatLogic'

describe('seedPromptIndex', () => {
  it('hides the opening briefing on a fully loaded thread', () => {
    expect(seedPromptIndex(true, false)).toBe(0)
  })

  it('hides nothing when the thread starts mid-conversation', () => {
    // Pagination has not reached the oldest page, so index 0 is a real message
    // the customer sent. Blanking it would be worse than showing the prompt.
    expect(seedPromptIndex(true, true)).toBe(-1)
  })

  it('hides nothing outside minimal mode', () => {
    // A developer watching their own task should see the prompt that started
    // it — that is the task.
    expect(seedPromptIndex(false, false)).toBe(-1)
    expect(seedPromptIndex(false, true)).toBe(-1)
  })
})

describe('shouldShowWelcome', () => {
  it('shows the welcome screen when only the briefing exists', () => {
    // The agent has been told who it is talking to and has greeted them. That
    // is not a conversation.
    expect(shouldShowWelcome(true, false, 1)).toBe(true)
  })

  it('shows it on a session with nothing in it at all', () => {
    expect(shouldShowWelcome(true, false, 0)).toBe(true)
  })

  it('gives way as soon as the customer has asked something', () => {
    expect(shouldShowWelcome(true, false, 2)).toBe(false)
  })

  it('gives way the instant they hit send, before the count catches up', () => {
    // The interaction list is polled. Without this the welcome screen comes
    // back for a beat after they have already spoken.
    expect(shouldShowWelcome(true, true, 1)).toBe(false)
  })

  it('never shows outside minimal mode', () => {
    expect(shouldShowWelcome(false, false, 0)).toBe(false)
    expect(shouldShowWelcome(false, false, 1)).toBe(false)
  })
})
