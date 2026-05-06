import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactElement } from 'react'

import ExternalAgentDesktopViewer from './ExternalAgentDesktopViewer'

const stopExternalAgent = vi.fn()
const resumeExternalAgent = vi.fn()

vi.mock('../../hooks/useApi', () => ({
  default: () => ({
    getApiClient: () => ({
      v1SessionsStopExternalAgentDelete: stopExternalAgent,
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
  useGetSession: () => ({ data: undefined }),
}))

vi.mock('./DesktopStreamViewer', () => ({
  default: () => <div data-testid="desktop-stream" />,
}))

vi.mock('./ScreenshotViewer', () => ({
  default: () => <div data-testid="screenshot-viewer" />,
}))

vi.mock('./SandboxDropZone', () => ({
  default: ({ children }: any) => <div data-testid="sandbox-drop-zone">{children}</div>,
}))

vi.mock('../session/EmbeddedSessionView', () => ({
  default: () => null,
}))

vi.mock('../common/RobustPromptInput', () => ({
  default: () => null,
}))

describe('ExternalAgentDesktopViewer sandbox mode', () => {
  const renderViewer = (ui: ReactElement) =>
    render(
      <QueryClientProvider client={new QueryClient()}>
        {ui}
      </QueryClientProvider>,
    )

  it('does not show session resume controls for stopped sandbox desktops', () => {
    renderViewer(
      <ExternalAgentDesktopViewer
        sessionId="sbx_1"
        mode="screenshot"
        initialSandboxState="absent"
        sandboxMode
      />,
    )

    expect(screen.getByText('Desktop Unavailable')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /start desktop/i })).not.toBeInTheDocument()
    expect(resumeExternalAgent).not.toHaveBeenCalled()
  })

  it('does not show session stop controls for starting sandbox desktops', () => {
    renderViewer(
      <ExternalAgentDesktopViewer
        sessionId="sbx_1"
        mode="screenshot"
        initialSandboxState="starting"
        sandboxMode
      />,
    )

    expect(screen.queryByRole('button', { name: /^stop$/i })).not.toBeInTheDocument()
    expect(stopExternalAgent).not.toHaveBeenCalled()
  })
})
