import { describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
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

    // Copy is sentence case since the shared TaskSessionPlaceholder took over
    // this surface; the assertion that matters is that a sandbox desktop
    // offers no resume action.
    expect(screen.getByText('Desktop unavailable')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /start desktop/i })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /restart desktop/i })).not.toBeInTheDocument()
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

  it('explains a refused launch instead of only offering to start again', () => {
    // The backend records why StartDesktop refused (e.g. missing Claude
    // subscription) on the task, but the viewer used to drop every startup
    // error that was not "desktop limit reached" — leaving a paused card whose
    // start button fails identically with no explanation.
    renderViewer(
      <ExternalAgentDesktopViewer
        sessionId="ses_1"
        mode="screenshot"
        initialSandboxState="absent"
        startupErrorMessage="agent is configured to use a Claude subscription, but no active Claude subscription is available"
      />,
    )

    expect(screen.getByText(/no active Claude subscription is available/i)).toBeInTheDocument()
  })

  it('sends the user to connect the provider when a subscription is required', () => {
    // Retrying cannot succeed until the subscription exists, so the connect
    // action is the primary one and start is demoted rather than removed.
    const onConnectSubscription = vi.fn()
    renderViewer(
      <ExternalAgentDesktopViewer
        sessionId="ses_1"
        mode="screenshot"
        initialSandboxState="absent"
        startupErrorMessage="agent is configured to use a Claude subscription, but no active Claude subscription is available"
        connectSubscriptionLabel="Connect Claude"
        onConnectSubscription={onConnectSubscription}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Connect Claude' }))
    expect(onConnectSubscription).toHaveBeenCalledTimes(1)
    expect(screen.getByRole('button', { name: /start desktop/i })).toBeInTheDocument()
  })

  it('offers no connect action for an ordinary pause', () => {
    renderViewer(
      <ExternalAgentDesktopViewer sessionId="ses_1" mode="screenshot" initialSandboxState="absent" />,
    )

    expect(screen.queryByRole('button', { name: /^Connect / })).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: /start desktop/i })).toBeInTheDocument()
  })
})
