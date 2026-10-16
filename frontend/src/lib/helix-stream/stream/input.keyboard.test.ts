import { beforeEach, describe, expect, it } from 'vitest'
import { StreamInput, defaultStreamInputConfig } from './input'

/**
 * These assert what actually reaches the wire, which is where the bug lived:
 * the routing decision was only half the problem - the keysym messages were
 * also never transmitted in WebSocket mode.
 */
type SentMessage =
  | { kind: 'key'; isDown: boolean; key: number; modifiers: number }
  | { kind: 'keysym'; isDown: boolean; keysym: number; modifiers: number }
  | { kind: 'keysymTap'; keysym: number; modifiers: number }

function makeInput(): { input: StreamInput; sent: SentMessage[] } {
  const input = new StreamInput({
    ...defaultStreamInputConfig(),
    useEvdevCodes: true, // WebSocket streaming mode, as production uses
  })
  const sent: SentMessage[] = []

  // Stand in for the WebSocket transport patches applied by WebSocketStream.
  // @ts-ignore - patching the same private methods the transport patches
  input.sendKey = (isDown: boolean, key: number, modifiers: number) => {
    sent.push({ kind: 'key', isDown, key, modifiers })
  }
  // @ts-ignore
  input.sendKeysym = (isDown: boolean, keysym: number, modifiers: number) => {
    sent.push({ kind: 'keysym', isDown, keysym, modifiers })
  }
  // @ts-ignore
  input.sendKeysymTap = (keysym: number, modifiers: number = 0) => {
    sent.push({ kind: 'keysymTap', keysym, modifiers })
  }

  return { input, sent }
}

describe('StreamInput keyboard routing', () => {
  let input: StreamInput
  let sent: SentMessage[]

  beforeEach(() => {
    ;({ input, sent } = makeInput())
  })

  it('sends @ from a phone keyboard as a keysym tap, not the Digit2 position', () => {
    const init = { key: '@', code: 'Digit2', shiftKey: false }
    input.onKeyDown(new KeyboardEvent('keydown', init))
    input.onKeyUp(new KeyboardEvent('keyup', init))

    // One self-contained tap; the keyup is swallowed so a virtual keyboard that
    // never delivers keyup cannot latch the key down on the remote desktop.
    expect(sent).toEqual([{ kind: 'keysymTap', keysym: 0x40, modifiers: 0 }])
  })

  it('does not send the unshifted key position for a shifted character', () => {
    input.onKeyDown(new KeyboardEvent('keydown', { key: '@', code: 'Digit2', shiftKey: false }))

    // EvdevKey.KEY_2 is 3 - typing that is exactly the reported bug.
    expect(sent.some((m) => m.kind === 'key')).toBe(false)
  })

  it('leaves physical Shift+2 on the keycode path unchanged', () => {
    const init = { key: '@', code: 'Digit2', shiftKey: true }
    input.onKeyDown(new KeyboardEvent('keydown', init))
    input.onKeyUp(new KeyboardEvent('keyup', init))

    expect(sent).toEqual([
      { kind: 'key', isDown: true, key: 3, modifiers: 1 }, // KEY_2 with SHIFT
      { kind: 'key', isDown: false, key: 3, modifiers: 1 },
    ])
  })

  it('leaves ordinary letters on the keycode path', () => {
    const init = { key: 'a', code: 'KeyA', shiftKey: false }
    input.onKeyDown(new KeyboardEvent('keydown', init))
    input.onKeyUp(new KeyboardEvent('keyup', init))

    expect(sent).toEqual([
      { kind: 'key', isDown: true, key: 30, modifiers: 0 }, // KEY_A
      { kind: 'key', isDown: false, key: 30, modifiers: 0 },
    ])
  })

  it('auto-repeat produces one tap per repeated keydown', () => {
    const init = { key: '@', code: 'Digit2', shiftKey: false }
    input.onKeyDown(new KeyboardEvent('keydown', init))
    input.onKeyDown(new KeyboardEvent('keydown', { ...init, repeat: true }))

    expect(sent).toEqual([
      { kind: 'keysymTap', keysym: 0x40, modifiers: 0 },
      { kind: 'keysymTap', keysym: 0x40, modifiers: 0 },
    ])
  })

  it('types a swipe/IME string as one tap per character', () => {
    input.sendTextAsKeysyms('a@B')

    expect(sent).toEqual([
      { kind: 'keysymTap', keysym: 0x61, modifiers: 0 }, // a
      { kind: 'keysymTap', keysym: 0x40, modifiers: 0 }, // @
      { kind: 'keysymTap', keysym: 0x42, modifiers: 0 }, // B
    ])
  })

  it('handles the full local part of an email address', () => {
    input.sendTextAsKeysyms('user@example.com')

    expect(sent).toHaveLength('user@example.com'.length)
    expect(sent.map((m) => (m.kind === 'keysymTap' ? m.keysym : -1))).toEqual(
      [...'user@example.com'].map((c) => c.codePointAt(0)),
    )
  })
})

describe('StreamInput caps lock and held keys', () => {
  let input: StreamInput
  let sent: SentMessage[]

  beforeEach(() => {
    ;({ input, sent } = makeInput())
  })

  const CAPS_LOCK_BIT = 1 << 4

  it('never forwards CapsLock key events, including the macOS keydown-only/keyup-only sequence', () => {
    // macOS: keydown when caps turns on...
    input.onKeyDown(new KeyboardEvent('keydown', { key: 'CapsLock', code: 'CapsLock', modifierCapsLock: true }))
    // ...and only a keyup, later, when it turns off.
    input.onKeyUp(new KeyboardEvent('keyup', { key: 'CapsLock', code: 'CapsLock' }))

    expect(sent).toEqual([])
  })

  it('sets the caps-lock bit from getModifierState and keeps caps letters on the keycode path', () => {
    const init = { key: 'A', code: 'KeyA', modifierCapsLock: true }
    input.onKeyDown(new KeyboardEvent('keydown', init))
    input.onKeyUp(new KeyboardEvent('keyup', init))

    expect(sent).toEqual([
      { kind: 'key', isDown: true, key: 30, modifiers: CAPS_LOCK_BIT },
      { kind: 'key', isDown: false, key: 30, modifiers: CAPS_LOCK_BIT },
    ])
  })

  it('clears the caps-lock bit when caps is off locally', () => {
    input.onKeyDown(new KeyboardEvent('keydown', { key: 'a', code: 'KeyA' }))

    expect(sent).toEqual([{ kind: 'key', isDown: true, key: 30, modifiers: 0 }])
  })

  it('releases exactly the keys sent as down', () => {
    input.onKeyDown(new KeyboardEvent('keydown', { key: 'Control', code: 'ControlLeft', ctrlKey: true }))
    input.onKeyDown(new KeyboardEvent('keydown', { key: 'a', code: 'KeyA', ctrlKey: true }))
    input.onKeyUp(new KeyboardEvent('keyup', { key: 'a', code: 'KeyA', ctrlKey: true }))
    sent.length = 0

    input.releaseAllKeys()

    expect(sent).toEqual([{ kind: 'key', isDown: false, key: 29, modifiers: 0 }]) // KEY_LEFTCTRL
    sent.length = 0
    input.releaseAllKeys()
    expect(sent).toEqual([])
  })

  it('drops a keyup for a key that was never sent as down', () => {
    input.onKeyUp(new KeyboardEvent('keyup', { key: 'Control', code: 'ControlLeft' }))

    expect(sent).toEqual([])
  })
})

