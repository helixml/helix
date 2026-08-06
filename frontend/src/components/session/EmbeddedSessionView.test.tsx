import { act, fireEvent, render, screen } from '@testing-library/react'
import { createTheme, ThemeProvider } from '@mui/material/styles'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import EmbeddedSessionView from './EmbeddedSessionView'

const resizeCallbacks = new Map<Element, ResizeObserverCallback>()

vi.mock('@tanstack/react-query', () => ({
  useQueryClient: () => ({ removeQueries: vi.fn() }),
}))

vi.mock('../../hooks/useAccount', () => ({
  default: () => ({ user: { id: 'user-1' }, serverConfig: {}, admin: false }),
}))

vi.mock('../../hooks/useApi', () => ({
  default: () => ({ getApiClient: () => ({}) }),
}))

vi.mock('../../hooks/useLightTheme', () => ({
  default: () => ({ isDark: true, scrollbar: {} }),
}))

vi.mock('../../contexts/streaming', () => ({
  useStreaming: () => ({ NewInference: vi.fn() }),
}))

vi.mock('../../services/sessionService', () => ({
  GET_SESSION_QUERY_KEY: (id: string) => ['session', id],
  LIST_INTERACTIONS_QUERY_KEY: (id: string) => ['interactions', id],
  useGetSession: () => ({
    data: { data: { id: 'session-1', owner: 'user-1', config: {} } },
    refetch: vi.fn(),
    error: null,
  }),
  useListInteractions: () => ({
    data: {
      data: {
        interactions: [{
          id: 'interaction-1',
          state: 'complete',
          prompt_message: 'Hello',
          response_message: 'Hi',
        }],
        totalPages: 1,
        totalCount: 1,
      },
    },
  }),
  useListSessionSteps: () => ({ data: { data: [] } }),
}))

vi.mock('./Interaction', () => ({
  default: () => <div data-testid="interaction">Interaction</div>,
}))
vi.mock('./InteractionLiveStream', () => ({ default: () => null }))
vi.mock('./PausedBanner', () => ({ default: () => null }))
vi.mock('./ForkBadge', () => ({ default: () => null }))
vi.mock('./ChatTurnNavigator', () => ({ default: () => null }))

const triggerResize = (element: Element, height: number) => {
  const callback = resizeCallbacks.get(element)
  if (!callback) throw new Error('Element is not being observed')
  callback([
    { target: element, contentRect: { height } } as unknown as ResizeObserverEntry,
  ], {} as ResizeObserver)
}

const setScrollGeometry = (
  element: HTMLElement,
  geometry: { scrollTop: number; scrollHeight: number; clientHeight: number },
) => {
  Object.defineProperties(element, {
    scrollTop: { configurable: true, writable: true, value: geometry.scrollTop },
    scrollHeight: { configurable: true, writable: true, value: geometry.scrollHeight },
    clientHeight: { configurable: true, writable: true, value: geometry.clientHeight },
  })
}

describe('EmbeddedSessionView follow-latest behavior', () => {
  beforeEach(() => {
    resizeCallbacks.clear()
    vi.useFakeTimers()
    vi.stubGlobal('ResizeObserver', class {
      constructor(private readonly callback: ResizeObserverCallback) {}
      observe(element: Element) {
        resizeCallbacks.set(element, this.callback)
      }
      disconnect() {}
    })
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.unstubAllGlobals()
  })

  const renderView = () => {
    render(
      <ThemeProvider theme={createTheme({ palette: { mode: 'dark' } })}>
        <EmbeddedSessionView sessionId="session-1" />
      </ThemeProvider>,
    )

    const scrollContainer = document.querySelector<HTMLElement>('[data-session-scroll-container]')!
    const content = scrollContainer.firstElementChild as HTMLElement
    setScrollGeometry(scrollContainer, {
      scrollTop: 800,
      scrollHeight: 1000,
      clientHeight: 200,
    })
    act(() => {
      vi.runOnlyPendingTimers()
      triggerResize(content, 1000)
      triggerResize(scrollContainer, 200)
    })
    return { scrollContainer, content }
  }

  it('stays pinned when the composer queue shrinks the chat viewport', () => {
    const { scrollContainer, content } = renderView()

    setScrollGeometry(scrollContainer, {
      scrollTop: 800,
      scrollHeight: 1000,
      clientHeight: 100,
    })
    fireEvent.scroll(scrollContainer)
    act(() => triggerResize(scrollContainer, 100))

    expect(scrollContainer.scrollTop).toBe(1000)

    Object.defineProperty(scrollContainer, 'scrollHeight', {
      configurable: true,
      writable: true,
      value: 1050,
    })
    act(() => triggerResize(content, 1050))

    expect(scrollContainer.scrollTop).toBe(1050)
    expect(screen.queryByRole('button', { name: 'Jump to latest' })).not.toBeInTheDocument()
  })

  it('still pauses following after an explicit upward scroll', () => {
    const { scrollContainer, content } = renderView()

    fireEvent.wheel(scrollContainer, { deltaY: -40 })
    Object.defineProperty(scrollContainer, 'scrollTop', {
      configurable: true,
      writable: true,
      value: 650,
    })
    fireEvent.scroll(scrollContainer)
    Object.defineProperty(scrollContainer, 'scrollHeight', {
      configurable: true,
      writable: true,
      value: 1050,
    })
    act(() => triggerResize(content, 1050))

    expect(scrollContainer.scrollTop).toBe(650)
    expect(screen.getByRole('button', { name: 'Jump to latest' })).toBeInTheDocument()
  })
})
