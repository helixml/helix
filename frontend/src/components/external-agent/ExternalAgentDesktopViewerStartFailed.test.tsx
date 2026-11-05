import { describe, expect, it, vi, beforeEach } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'

import type { TypesSessionMetadata } from '../../api/api'
import ExternalAgentDesktopViewer from './ExternalAgentDesktopViewer'
import SpecTaskLaunchWindow, { getSpecTaskLaunchPhase } from '../tasks/SpecTaskLaunchWindow'
import { deriveSandboxState } from './sandboxState'

const resumeExternalAgent = vi.fn()
let sessionConfig: TypesSessionMetadata = {}

vi.mock('../../hooks/useApi', () => ({
  default: () => ({
    getApiClient: () => ({
      v1SessionsStopExternalAgentDelete: vi.fn(),
      v1SessionsResumeCreate: resumeExternalAgent,
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
  useGetSession: () => ({ data: { data: { config: sessionConfig, interactions: [] } } }),
}))

vi.mock('./DesktopStreamViewer', () => ({
  default: () => <div data-testid="desktop-stream" />,
}))

vi.mock('./ScreenshotViewer', () => ({
  default: () => <div data-testid="screenshot-viewer" />,
}))

vi.mock('./SandboxDropZone', () => ({
  default: ({ children }: any) => <div>{children}</div>,
}))

vi.mock('../session/EmbeddedSessionView', () => ({
  default: () => null,
}))

vi.mock('../common/RobustPromptInput', () => ({
  default: () => null,
}))

// Mirrors SpecTaskDetailContent: the launch window replaces the desktop
// viewer while getSpecTaskLaunchPhase reports a launch in progress.
const SpecTaskDesktop = ({ status }: { status: string }) => {
  const { hasDesktopLifecycleState } = deriveSandboxState(sessionConfig)
  const phase = getSpecTaskLaunchPhase({
    status,
    activeSessionId: 'ses_1',
    hasDesktopLifecycleState,
  })
  return (
    <QueryClientProvider client={new QueryClient()}>
      {phase ? (
        <SpecTaskLaunchWindow phase={phase} mode="implementation" />
      ) : (
        <ExternalAgentDesktopViewer sessionId="ses_1" mode="stream" />
      )}
    </QueryClientProvider>
  )
}

describe('spec task desktop after a failed start', () => {
  beforeEach(() => {
    resumeExternalAgent.mockReset()
    resumeExternalAgent.mockResolvedValue({})
  })

  it.each(['implementation', 'spec_generation'])(
    'shows the stopped desktop, the failure and a Start button for a task in %s',
    (status) => {
      sessionConfig = {
        external_agent_status: 'stopped',
        status_message:
          'Desktop failed to start: failed to create dev container via Hydra: failed to dial Hydra via RevDial: no connection',
        dev_container_id: 'old-container',
      }

      render(<SpecTaskDesktop status={status} />)

      expect(screen.queryByText('booting virtual desktop environment...')).not.toBeInTheDocument()
      expect(screen.getByText('Desktop paused')).toBeInTheDocument()
      expect(screen.getByText(/Desktop failed to start: .*no connection/)).toBeInTheDocument()
      fireEvent.click(screen.getByRole('button', { name: /start desktop/i }))
      expect(resumeExternalAgent).toHaveBeenCalledWith('ses_1')
    },
  )

  it('offers Start for a session that has run before even if its lifecycle fields were wiped', () => {
    // The state meta was left in: every lifecycle field empty, but the
    // session had a desktop before. It must not read as a first launch.
    sessionConfig = { dev_container_id: 'old-container' }

    render(<SpecTaskDesktop status="implementation" />)

    expect(screen.queryByText('booting virtual desktop environment...')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: /start desktop/i })).toBeInTheDocument()
  })

  it('still shows the boot screen for a session whose first launch has not begun', () => {
    sessionConfig = {}

    render(<SpecTaskDesktop status="implementation" />)

    expect(screen.getByRole('status')).toHaveTextContent('booting virtual desktop environment...')
    expect(screen.queryByRole('button', { name: /start desktop/i })).not.toBeInTheDocument()
  })
})
