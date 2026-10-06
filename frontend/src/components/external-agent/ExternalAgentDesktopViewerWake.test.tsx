import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'

import ExternalAgentDesktopViewer from './ExternalAgentDesktopViewer'
import type { SandboxState } from './sandboxState'

vi.mock('../../hooks/useApi', () => ({
  default: () => ({
    getApiClient: () => ({
      v1SessionsStopExternalAgentDelete: vi.fn(),
      v1SessionsResumeCreate: vi.fn(),
      v1ExternalAgentsUploadCreate: vi.fn(),
    }),
  }),
}))

vi.mock('../../hooks/useSnackbar', () => ({
  default: () => ({ success: vi.fn(), error: vi.fn(), info: vi.fn() }),
}))

vi.mock('../../contexts/streaming', () => ({
  useStreaming: () => ({ NewInference: vi.fn(), setCurrentSessionId: vi.fn() }),
}))

vi.mock('../../services/sessionService', () => ({
  GET_SESSION_QUERY_KEY: (id: string) => ['session', id],
  useGetSession: () => ({ data: undefined }),
}))

// Record every wakeSignal the viewer hands the stream. DesktopStreamViewer
// resets its reconnect retry budget and reconnects on each change, which is
// what repoints the transport at the container that came up.
const wakeSignals: number[] = []
vi.mock('./DesktopStreamViewer', () => ({
  default: ({ wakeSignal }: { wakeSignal?: number }) => {
    wakeSignals.push(wakeSignal ?? -1)
    return <div data-testid="desktop-stream" />
  },
}))

vi.mock('./ScreenshotViewer', () => ({ default: () => <div /> }))
vi.mock('./SandboxDropZone', () => ({
  default: ({ children }: any) => <div>{children}</div>,
}))
vi.mock('../session/EmbeddedSessionView', () => ({ default: () => null }))
vi.mock('../common/RobustPromptInput', () => ({ default: () => null }))

const latestWake = () => wakeSignals[wakeSignals.length - 1]

// The wake signal used to fire on the paused→reachable edge. That worked for
// restart only by accident: the backend cleared the status during teardown, so
// the live probe reported "stopped" and the session passed through "absent".
// Now that a restart reports "restarting" throughout (the actual bug fix), the
// signal has to fire on "re-entered running after leaving it" instead — or the
// viewer keeps a possibly-exhausted retry budget pointed at the dead container.
describe('ExternalAgentDesktopViewer wake signal', () => {
  beforeEach(() => {
    wakeSignals.length = 0
  })

  const renderAt = (state: SandboxState) => {
    const qc = new QueryClient()
    const view = render(
      <QueryClientProvider client={qc}>
        <ExternalAgentDesktopViewer sessionId="ses_1" mode="stream" initialSandboxState={state} />
      </QueryClientProvider>,
    )
    return {
      rerenderAt: (next: SandboxState) =>
        view.rerender(
          <QueryClientProvider client={qc}>
            <ExternalAgentDesktopViewer sessionId="ses_1" mode="stream" initialSandboxState={next} />
          </QueryClientProvider>,
        ),
    }
  }

  it('does not fire on first mount into running', () => {
    renderAt('running')
    expect(latestWake()).toBe(0)
  })

  it('fires once on running → starting → running (the fixed restart path)', () => {
    const { rerenderAt } = renderAt('running')
    expect(latestWake()).toBe(0)

    // A restart now reports "restarting", which derives to "starting" — it
    // never passes through "absent", so the old sawPaused edge would never fire.
    rerenderAt('starting')
    expect(latestWake()).toBe(0)

    rerenderAt('running')
    expect(latestWake()).toBe(1)
  })

  it('still fires on running → absent → running (the original wake path)', () => {
    const { rerenderAt } = renderAt('running')
    rerenderAt('absent')
    rerenderAt('running')
    expect(latestWake()).toBe(1)
  })

  it('fires again on a second restart', () => {
    const { rerenderAt } = renderAt('running')
    rerenderAt('starting')
    rerenderAt('running')
    rerenderAt('starting')
    rerenderAt('running')
    expect(latestWake()).toBe(2)
  })

  it('does not fire while still leaving running (no premature reconnect)', () => {
    const { rerenderAt } = renderAt('running')
    rerenderAt('starting')
    rerenderAt('starting')
    rerenderAt('absent')
    expect(latestWake()).toBe(0)
  })
})
