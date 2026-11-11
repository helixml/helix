import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { WebSocketStream } from './websocket-stream'
import { WsMessageType } from './websocket-stream.types'
import { getStandardVideoFormats } from './video'

class FakeWebSocket {
  static OPEN = 1
  static instances: FakeWebSocket[] = []

  readyState = FakeWebSocket.OPEN
  binaryType = ''
  bufferedAmount = 0
  closeCalls = 0
  sent: unknown[] = []
  listeners = new Map<string, Array<(event: any) => void>>()

  constructor(public url: string) {
    FakeWebSocket.instances.push(this)
  }

  addEventListener(type: string, listener: (event: any) => void) {
    const listeners = this.listeners.get(type) ?? []
    listeners.push(listener)
    this.listeners.set(type, listeners)
  }

  removeEventListener() {}
  close() {
    this.closeCalls++
  }
  send(data: unknown) {
    this.sent.push(data)
  }

  emit(type: string, event: any = {}) {
    for (const listener of this.listeners.get(type) ?? []) listener(event)
  }
}

function makeStream() {
  const settings: any = {
    videoSize: '1080p',
    fps: 60,
    bitrate: 5000,
    packetSize: 1024,
    playAudioLocal: false,
    mouseScrollMode: 'highres',
    controllerConfig: { invertAB: false, invertXY: false },
    videoSizeCustom: { width: 1920, height: 1080 },
  }
  return new WebSocketStream(
    {} as any,
    1,
    1,
    settings,
    getStandardVideoFormats(),
    [1920, 1080],
    'ses_test',
    'tab_test',
  )
}

describe('WebSocketStream reconnect socket ownership', () => {
  let originalWebSocket: any

  beforeEach(() => {
    FakeWebSocket.instances = []
    vi.useFakeTimers()
    originalWebSocket = globalThis.WebSocket
    globalThis.WebSocket = FakeWebSocket as any
    vi.spyOn(console, 'log').mockImplementation(() => {})
    vi.spyOn(console, 'warn').mockImplementation(() => {})
    vi.spyOn(console, 'error').mockImplementation(() => {})
  })

  afterEach(() => {
    vi.useRealTimers()
    globalThis.WebSocket = originalWebSocket
    vi.restoreAllMocks()
  })

  it('ignores events delivered late by a replaced socket', () => {
    const stream = makeStream()
    const first = FakeWebSocket.instances[0]
    const events: string[] = []
    stream.addInfoListener((event) => events.push(event.detail.type))

    stream.reconnect()
    const replacement = FakeWebSocket.instances[1]
    replacement.emit('open')
    events.length = 0

    const selfID = new Uint8Array(5)
    selfID[0] = WsMessageType.SelfId
    new DataView(selfID.buffer).setUint32(1, 42, true)
    first.emit('open')
    first.emit('message', { data: selfID.buffer })
    first.emit('error', new Event('error'))
    first.emit('close', { code: 1006, reason: '', wasClean: false })

    expect(events).toEqual([])
    expect((stream as any).connected).toBe(true)
    vi.advanceTimersByTime(1500)
    expect(FakeWebSocket.instances).toHaveLength(2)
    expect(replacement.sent).toContainEqual(expect.stringContaining('"type":"init"'))

    stream.close()
  })

  it('still reconnects when the active socket closes', () => {
    vi.spyOn(Math, 'random').mockReturnValue(0.5)
    const stream = makeStream()
    const socket = FakeWebSocket.instances[0]
    const events: string[] = []
    stream.addInfoListener((event) => events.push(event.detail.type))
    socket.emit('open')

    socket.emit('close', { code: 1006, reason: '', wasClean: false })

    expect(events).toContain('disconnected')
    expect(events).toContain('reconnecting')
    vi.advanceTimersByTime(1000)
    expect(FakeWebSocket.instances).toHaveLength(2)

    stream.close()
  })

  it('stops the old socket timers before the replacement opens', () => {
    const stream = makeStream()
    const first = FakeWebSocket.instances[0]
    first.emit('open')

    stream.reconnect()
    const replacement = FakeWebSocket.instances[1]
    vi.advanceTimersByTime(11_000)

    expect(replacement.sent).toEqual([])
    expect(replacement.closeCalls).toBe(0)
    expect(FakeWebSocket.instances).toHaveLength(2)

    stream.close()
  })
})
