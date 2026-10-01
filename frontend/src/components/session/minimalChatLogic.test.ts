import { describe, it, expect } from 'vitest'
import { composerSendMode, seedPromptIndex, shouldShowWelcome } from './minimalChatLogic'

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

  it('does not mistake an interaction query that is still loading for an empty session', () => {
    expect(shouldShowWelcome(true, false, 0, true)).toBe(false)
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

// An org bot instance (the Find AI chat) has no briefing turn: interaction 0 is
// the customer's own first question. Both rules above assumed a spec task.
describe('sessions without a briefing turn', () => {
  it("never hides the customer's first message", () => {
    expect(seedPromptIndex(true, false, false)).toBe(-1)
  })

  it('shows the welcome screen only when nothing has been said', () => {
    expect(shouldShowWelcome(true, false, 0, false, false)).toBe(true)
  })

  it('does not put the welcome screen over a real first exchange after a reload', () => {
    expect(shouldShowWelcome(true, false, 1, false, false)).toBe(false)
  })
})

describe('composerSendMode', () => {
  it('sends directly from a minimal embed with no backend queue', () => {
    // The welcome screen remounts the composer on first send; in queued mode
    // the remounted instance replayed the message, so one click became two
    // concurrent turns.
    expect(composerSendMode(true, false)).toBe('direct')
  })

  it('keeps the backend queue for spec-task embeds', () => {
    expect(composerSendMode(true, true)).toBe('queued')
  })

  it('leaves the full app composer queued', () => {
    expect(composerSendMode(false, false)).toBe('queued')
    expect(composerSendMode(false, true)).toBe('queued')
  })
})
