import { act, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import DesktopStreamViewer from './DesktopStreamViewer'

const streamState = vi.hoisted(() => ({ instances: [] as any[] }))

vi.mock('../../lib/helix-stream/stream/websocket-stream', () => {
  class WebSocketStream {
    listener?: (event: any) => void
    input = new Proxy({}, { get: () => vi.fn() })

    constructor() {
      streamState.instances.push(this)
    }
    addInfoListener(listener: (event: any) => void) {
      this.listener = listener
    }
    emit(detail: any) {
      this.listener?.({ detail })
    }
    setCanvas() {}
    setVideoEnabled() {}
    setAudioEnabled() {}
    setThrottleRatio() {}
    close() {}
    getInput() {
      return this.input
    }
  }
  return { WebSocketStream }
})

vi.mock('../../lib/helix-stream/stream/video', () => ({
  getWebCodecsSupportedVideoFormats: vi.fn().mockResolvedValue({}),
  getStandardVideoFormats: vi.fn().mockReturnValue({}),
}))
vi.mock('../../lib/helix-stream/component/settings_menu', () => ({
  defaultStreamSettings: () => ({ videoSizeCustom: {}, controllerConfig: {} }),
  getDefaultBitrateForResolution: () => 5000,
}))
vi.mock('../../hooks/useApi', () => ({
  default: () => ({
    getApiClient: () => ({ v1ConfigList: vi.fn().mockResolvedValue({ data: {} }) }),
  }),
}))
vi.mock('../../hooks/useLightTheme', () => ({ default: () => false }))
vi.mock('../../contexts/account', () => ({
  useAccount: () => ({ initialized: true, user: { name: 'Kai' } }),
}))
vi.mock('../../contexts/VideoStreamContext', () => ({
  useVideoStream: () => ({ registerStream: () => () => {} }),
}))
vi.mock('../../utils/isMobileOrTablet', () => ({ isMobileOrTablet: () => false }))
vi.mock('./StatsOverlay', () => ({ default: () => null }))
vi.mock('./ChartsPanel', () => ({ default: () => null }))
vi.mock('./ConnectionOverlay', () => ({ default: () => null }))
vi.mock('./RemoteCursorsOverlay', () => ({ default: () => null }))
vi.mock('./AgentCursorOverlay', () => ({ default: () => null }))
vi.mock('./CursorRenderer', () => ({ default: () => null }))
vi.mock('./InsecureContextWarning', () => ({ default: () => null }))

describe('DesktopStreamViewer reconnect presence', () => {
  beforeEach(() => {
    streamState.instances = []
    vi.spyOn(console, 'log').mockImplementation(() => {})
    vi.spyOn(console, 'warn').mockImplementation(() => {})
    vi.spyOn(console, 'error').mockImplementation(() => {})
    Object.defineProperty(window, 'matchMedia', {
      configurable: true,
      value: () => ({ matches: false, addEventListener() {}, removeEventListener() {} }),
    })
    globalThis.ResizeObserver = class {
      observe() {}
      disconnect() {}
    } as any
    globalThis.IntersectionObserver = class {
      constructor(private callback: IntersectionObserverCallback) {}
      observe(target: Element) {
        this.callback([{ isIntersecting: true, target } as IntersectionObserverEntry], this as any)
      }
      disconnect() {}
    } as any
  })

  it('replaces the old presence roster when its socket reconnects', async () => {
    render(<DesktopStreamViewer sessionId="ses_test" />)
    await waitFor(() => expect(streamState.instances).toHaveLength(1))
    const stream = streamState.instances[0]

    act(() => {
      stream.emit({ type: 'connected' })
      stream.emit({ type: 'selfId', clientId: 1 })
      stream.emit({ type: 'remoteUserJoined', user: { userId: 1, userName: 'Kai', color: '#f00' } })
      stream.emit({ type: 'connectionComplete', capabilities: { touch: false } })
    })
    expect(screen.getAllByText('K')).toHaveLength(1)

    act(() => {
      stream.emit({ type: 'connected' })
      stream.emit({ type: 'selfId', clientId: 2 })
      stream.emit({ type: 'remoteUserJoined', user: { userId: 2, userName: 'Kai', color: '#0f0' } })
    })
    expect(screen.getAllByText('K')).toHaveLength(1)
  })

  it('resyncs distinct same-name viewers by ID and handles their departure', async () => {
    render(<DesktopStreamViewer sessionId="ses_test" />)
    await waitFor(() => expect(streamState.instances).toHaveLength(1))
    const stream = streamState.instances[0]

    act(() => {
      stream.emit({ type: 'connected' })
      stream.emit({ type: 'selfId', clientId: 1 })
      stream.emit({ type: 'remoteUserJoined', user: { userId: 1, userName: 'Kai', color: '#f00' } })
      stream.emit({ type: 'remoteUserJoined', user: { userId: 9, userName: 'Kai', color: '#00f' } })
      stream.emit({ type: 'connectionComplete', capabilities: { touch: false } })
    })
    expect(screen.getAllByText('K')).toHaveLength(2)

    act(() => {
      stream.emit({ type: 'connected' })
      stream.emit({ type: 'selfId', clientId: 2 })
      stream.emit({ type: 'remoteUserJoined', user: { userId: 2, userName: 'Kai', color: '#0f0' } })
      stream.emit({ type: 'remoteUserJoined', user: { userId: 9, userName: 'Kai', color: '#00f' } })
    })
    expect(screen.getAllByText('K')).toHaveLength(2)

    act(() => stream.emit({ type: 'remoteUserLeft', userId: 9 }))
    expect(screen.getAllByText('K')).toHaveLength(1)
  })
})
