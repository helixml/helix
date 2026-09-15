import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { WebSocketStream } from './websocket-stream'
import { WsMessageType } from './websocket-stream.types'

/**
 * Exercises the REAL transport, not stubs.
 *
 * The keysym send methods existed and were correct, but WebSocketStream never
 * patched them onto StreamInput, so they wrote to a null RTCDataChannel and
 * every character a virtual keyboard produced was dropped with no error. A test
 * that stubs the send methods cannot catch that - it has to go through the
 * transport the product actually uses.
 */
class FakeWebSocket {
  static OPEN = 1
  static instances: FakeWebSocket[] = []

  readyState = FakeWebSocket.OPEN
  binaryType = ''
  bufferedAmount = 0
  sent: Uint8Array[] = []

  constructor(public url: string) {
    FakeWebSocket.instances.push(this)
  }

  addEventListener() {}
  removeEventListener() {}
  close() {}

  send(data: ArrayBuffer) {
    this.sent.push(new Uint8Array(data.slice(0)))
  }
}

function makeStream(): { stream: WebSocketStream; socket: FakeWebSocket } {
  const settings: any = {
    videoSize: '1080p',
    mouseScrollMode: 'highres',
    controllerConfig: { invertAB: false, invertXY: false },
    videoSizeCustom: { width: 1920, height: 1080 },
  }

  const stream = new WebSocketStream(
    {} as any, // api
    'host',
    'app',
    settings,
    [],
    [1920, 1080],
    'ses_test',
  )

  return { stream, socket: FakeWebSocket.instances.at(-1)! }
}

describe('WebSocketStream keyboard transport', () => {
  let originalWebSocket: any

  beforeEach(() => {
    FakeWebSocket.instances = []
    originalWebSocket = globalThis.WebSocket
    // @ts-ignore - substituting the transport
    globalThis.WebSocket = FakeWebSocket
    vi.spyOn(console, 'log').mockImplementation(() => {})
    vi.spyOn(console, 'warn').mockImplementation(() => {})
  })

  afterEach(() => {
    globalThis.WebSocket = originalWebSocket
    vi.restoreAllMocks()
  })

  it('puts a keysym tap on the wire when a phone keyboard types @', () => {
    const { stream, socket } = makeStream()
    socket.sent.length = 0

    stream
      .getInput()
      .onKeyDown(
        new KeyboardEvent('keydown', { key: '@', code: 'Digit2', shiftKey: false }),
      )

    // [msgType][subType=3 keysym tap][modifiers=0][keysym 0x40 big-endian]
    expect(socket.sent).toHaveLength(1)
    expect([...socket.sent[0]]).toEqual([
      WsMessageType.KeyboardInput,
      3,
      0,
      0x00,
      0x00,
      0x00,
      0x40,
    ])

    stream.close()
  })

  it('puts the Digit2 keycode on the wire for physical Shift+2', () => {
    const { stream, socket } = makeStream()
    socket.sent.length = 0

    stream
      .getInput()
      .onKeyDown(
        new KeyboardEvent('keydown', { key: '@', code: 'Digit2', shiftKey: true }),
      )

    // [msgType][subType=0 keycode][isDown=1][modifiers=SHIFT][KEY_2=3 big-endian]
    expect([...socket.sent[0]]).toEqual([
      WsMessageType.KeyboardInput,
      0,
      1,
      1,
      0x00,
      0x03,
    ])

    stream.close()
  })

  it('types an email address with the @ intact', () => {
    const { stream, socket } = makeStream()
    socket.sent.length = 0

    stream.getInput().sendTextAsKeysyms('a@b')

    const keysyms = socket.sent.map((m) => m[m.length - 1])
    expect(keysyms).toEqual([0x61, 0x40, 0x62])
    // Every message is a keysym tap, never a bare key position.
    expect(socket.sent.every((m) => m[1] === 3)).toBe(true)

    stream.close()
  })
})
