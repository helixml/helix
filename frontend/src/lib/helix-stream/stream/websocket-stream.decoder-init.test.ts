import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { WebSocketStream } from './websocket-stream'
import { getStandardVideoFormats } from './video'

/**
 * On connect the server replays the current GOP, keyframe first, immediately
 * after StreamInit. initVideoDecoder awaits VideoDecoder.isConfigSupported before
 * configure(), so those frames arrive while the decoder is unconfigured. They
 * used to be dropped, losing the only keyframe: the viewer then sat on "Waiting
 * for video" until the encoder's next scheduled keyframe, which on an idle Helix
 * for Mac desktop is ~30s away.
 */
class FakeWebSocket {
  static OPEN = 1
  readyState = FakeWebSocket.OPEN
  binaryType = ''
  bufferedAmount = 0
  constructor(public url: string) {}
  addEventListener() {}
  removeEventListener() {}
  close() {}
  send() {}
}

let releaseConfigSupport: () => void = () => {}
const decoded: string[] = []

class FakeVideoDecoder {
  state = 'unconfigured'
  decodeQueueSize = 0
  static isConfigSupported(config: any) {
    return new Promise((resolve) => {
      releaseConfigSupport = () => resolve({ supported: true, config })
    })
  }
  constructor(_init: any) {}
  configure() {
    this.state = 'configured'
  }
  decode(chunk: { type: string }) {
    decoded.push(chunk.type)
  }
  close() {
    this.state = 'closed'
  }
}

class FakeEncodedVideoChunk {
  type: string
  constructor(init: { type: string }) {
    this.type = init.type
  }
}

// [type=VideoFrame][codec=H264][flags][pts u64 BE][width u16][height u16][payload]
function videoFrame(keyframe: boolean, pts: number): Uint8Array {
  const buf = new Uint8Array(15 + 8)
  const view = new DataView(buf.buffer)
  buf[0] = 0x01
  buf[1] = 0x01
  buf[2] = keyframe ? 0x01 : 0x00
  view.setBigUint64(3, BigInt(pts), false)
  view.setUint16(11, 1920, false)
  view.setUint16(13, 1080, false)
  buf.set([0, 0, 0, 1, keyframe ? 0x65 : 0x41, 0, 0, 0], 15)
  return buf
}

function makeStream(): any {
  const settings: any = {
    videoSize: '1080p',
    mouseScrollMode: 'highres',
    controllerConfig: { invertAB: false, invertXY: false },
    videoSizeCustom: { width: 1920, height: 1080 },
  }
  return new WebSocketStream({} as any, 1, 1, settings, getStandardVideoFormats(), [1920, 1080], 'ses_test')
}

describe('WebSocketStream frames arriving while the decoder configures', () => {
  const saved: Record<string, any> = {}

  beforeEach(() => {
    decoded.length = 0
    for (const k of ['WebSocket', 'VideoDecoder', 'EncodedVideoChunk']) saved[k] = (globalThis as any)[k]
    ;(globalThis as any).WebSocket = FakeWebSocket
    ;(globalThis as any).VideoDecoder = FakeVideoDecoder
    ;(globalThis as any).EncodedVideoChunk = FakeEncodedVideoChunk
    vi.spyOn(console, 'log').mockImplementation(() => {})
    vi.spyOn(console, 'warn').mockImplementation(() => {})
  })

  afterEach(() => {
    for (const k of Object.keys(saved)) (globalThis as any)[k] = saved[k]
    vi.restoreAllMocks()
  })

  it('decodes the replayed keyframe once configure() completes', async () => {
    const stream = makeStream()
    const init: Promise<void> = stream.initVideoDecoder(1, 1920, 1080)

    // GOP replay lands before isConfigSupported resolves
    stream.handleVideoFrame(videoFrame(true, 1000))
    stream.handleVideoFrame(videoFrame(false, 2000))
    stream.handleVideoFrame(videoFrame(false, 3000))
    expect(decoded).toEqual([])

    releaseConfigSupport()
    await init

    expect(decoded).toEqual(['key', 'delta', 'delta'])
  })

  it('holds only from the latest keyframe and drops leading deltas', async () => {
    const stream = makeStream()
    const init: Promise<void> = stream.initVideoDecoder(1, 1920, 1080)

    stream.handleVideoFrame(videoFrame(false, 500))
    stream.handleVideoFrame(videoFrame(true, 1000))
    stream.handleVideoFrame(videoFrame(false, 2000))
    stream.handleVideoFrame(videoFrame(true, 3000))
    stream.handleVideoFrame(videoFrame(false, 4000))

    releaseConfigSupport()
    await init

    expect(decoded).toEqual(['key', 'delta'])
  })
})
