import { describe, expect, it } from 'vitest'
import { convertToKeysym, hasShiftLevelMismatch, shouldUseKeysym } from './keysym'

/**
 * A phone's on-screen keyboard emits a shifted symbol with no Shift key event.
 * Replaying that as its physical key position types the unshifted character —
 * '@' arrives as '2', which made logging in to sites from a phone impossible.
 */
function virtualKeyboardEvent(key: string, code: string): KeyboardEvent {
  return new KeyboardEvent('keydown', { key, code, shiftKey: false })
}

function physicalKeyboardEvent(
  key: string,
  code: string,
  shiftKey: boolean,
): KeyboardEvent {
  return new KeyboardEvent('keydown', { key, code, shiftKey })
}

describe('shouldUseKeysym', () => {
  it('routes a virtual-keyboard shifted symbol to keysym mode', () => {
    // The bug from the report: Shift+2 tapped as '@' on an iPhone keyboard.
    expect(shouldUseKeysym(virtualKeyboardEvent('@', 'Digit2'))).toBe(true)
    expect(convertToKeysym(virtualKeyboardEvent('@', 'Digit2'))).toBe(0x40)
  })

  it.each([
    ['!', 'Digit1', 0x21],
    ['#', 'Digit3', 0x23],
    ['_', 'Minus', 0x5f],
    [':', 'Semicolon', 0x3a],
    ['?', 'Slash', 0x3f],
    ['A', 'KeyA', 0x41],
  ])('routes %s to keysym %s', (key, code, expected) => {
    const event = virtualKeyboardEvent(key, code)
    expect(shouldUseKeysym(event)).toBe(true)
    expect(convertToKeysym(event)).toBe(expected)
  })

  it('leaves physical Shift+2 on the evdev keycode path', () => {
    // A real keyboard reports the modifier, so the key position replays correctly.
    // This path must not change - it is what desktop users rely on.
    expect(shouldUseKeysym(physicalKeyboardEvent('@', 'Digit2', true))).toBe(false)
  })

  it.each([
    ['a', 'KeyA'],
    ['2', 'Digit2'],
    ['-', 'Minus'],
    ['.', 'Period'],
    [' ', 'Space'],
  ])('leaves unshifted character %s on the evdev path', (key, code) => {
    expect(shouldUseKeysym(virtualKeyboardEvent(key, code))).toBe(false)
  })

  it.each([
    ['Enter', 'Enter'],
    ['Backspace', 'Backspace'],
    ['ArrowLeft', 'ArrowLeft'],
    ['Escape', 'Escape'],
  ])('leaves named key %s on the evdev path', (key, code) => {
    expect(shouldUseKeysym(virtualKeyboardEvent(key, code))).toBe(false)
  })

  it('still uses keysym mode when event.code is unavailable', () => {
    // Pre-existing iPad/iOS behaviour must be preserved.
    expect(shouldUseKeysym(virtualKeyboardEvent('a', ''))).toBe(true)
    expect(shouldUseKeysym(virtualKeyboardEvent('a', 'Unidentified'))).toBe(true)
  })

  it('ignores events with no usable key', () => {
    expect(shouldUseKeysym(virtualKeyboardEvent('Unidentified', ''))).toBe(false)
    expect(shouldUseKeysym(virtualKeyboardEvent('', ''))).toBe(false)
  })
})

describe('hasShiftLevelMismatch', () => {
  it('does not fire for uppercase produced by CapsLock', () => {
    // CapsLock legitimately yields 'A' with shiftKey=false, and the remote
    // desktop already tracks the CapsLock we forwarded to it. Rewriting these
    // as Shift+A would type lowercase there.
    const event = new KeyboardEvent('keydown', {
      key: 'A',
      code: 'KeyA',
      shiftKey: false,
    })
    Object.defineProperty(event, 'getModifierState', {
      value: (mod: string) => mod === 'CapsLock',
    })
    expect(hasShiftLevelMismatch(event)).toBe(false)
  })

  it('still fires for a symbol while CapsLock is on', () => {
    // CapsLock does not produce '@' - that is still a virtual keyboard.
    const event = new KeyboardEvent('keydown', {
      key: '@',
      code: 'Digit2',
      shiftKey: false,
    })
    Object.defineProperty(event, 'getModifierState', {
      value: (mod: string) => mod === 'CapsLock',
    })
    expect(hasShiftLevelMismatch(event)).toBe(true)
  })
})
