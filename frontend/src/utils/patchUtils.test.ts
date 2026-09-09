import { describe, it, expect } from 'vitest'
import { applyPatch, hasPatchGap } from './patchUtils'

describe('applyPatch', () => {
  it('takes the patch directly as the first chunk', () => {
    expect(applyPatch('', 0, 'Hello', 5)).toBe('Hello')
  })

  it('appends during normal streaming', () => {
    expect(applyPatch('Hello', 5, ' world', 11)).toBe('Hello world')
  })

  it('rewrites from the offset on a backwards edit', () => {
    expect(applyPatch('Hello world', 5, ' there', 11)).toBe('Hello there')
  })

  it('truncates when the content got shorter', () => {
    expect(applyPatch('Hello world', 5, ' you', 9)).toBe('Hello you')
  })
})

describe('hasPatchGap', () => {
  // The bug this exists to catch. A viewer watching someone else's session saw
  // only the LAST SENTENCE of each reply. The socket had dropped, the streaming
  // baseline was cleared on reconnect, and no catch-up snapshot arrived — so the
  // client held "" while the server was hundreds of characters in.
  //
  // applyPatch cannot tell that apart from an append: its
  // `patchOffset >= currentContent.length` branch is true for BOTH. It happily
  // concatenates onto nothing and the reply silently restarts mid-sentence.
  it('detects a baseline that is missing content', () => {
    expect(hasPatchGap('', 500)).toBe(true)
  })

  it('demonstrates what applyPatch does with that gap, which is why we check first', () => {
    // 500 >= 0, so this is treated as a plain append onto an empty string.
    // The first 500 characters are gone and nothing reports it.
    expect(applyPatch('', 500, 'and finally, in summary.', 524))
      .toBe('and finally, in summary.')
  })

  it('does not flag an ordinary append', () => {
    expect(hasPatchGap('Hello', 5)).toBe(false)
  })

  it('does not flag a backwards edit', () => {
    expect(hasPatchGap('Hello world', 5)).toBe(false)
  })

  it('does not flag the very first chunk', () => {
    expect(hasPatchGap('', 0)).toBe(false)
  })
})
