import { describe, it, expect, vi, beforeEach } from 'vitest'
import { forwardRef } from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'

import HelixOrgWorkerDetail from './HelixOrgWorkerDetail'

// Verifies the inline-transcript wiring added to the worker detail page:
// when the worker's project already has an exploratory ("Human Desktop")
// session, the page mounts EmbeddedSessionView against it and subscribes
// the WebSocket via streaming.setCurrentSessionId — so the operator sees
// the worker's conversation without clicking out to the desktop tab.

const mockSetCurrentSessionId = vi.fn()
const mockGetExploratorySession = vi.fn()

vi.mock('../contexts/streaming', () => ({
  useStreaming: () => ({
    setCurrentSessionId: mockSetCurrentSessionId,
    NewInference: vi.fn(),
  }),
}))

vi.mock('../hooks/useApi', () => ({
  default: () => ({
    getApiClient: () => ({
      v1ProjectsExploratorySessionDetail: mockGetExploratorySession,
      v1ProjectsExploratorySessionCreate: vi.fn(),
      v1SessionsResumeCreate: vi.fn(),
    }),
  }),
}))

const mockUseHelixOrgWorker = vi.fn()
vi.mock('../services/helixOrgService', () => ({
  useHelixOrgWorker: (id: string | undefined) => mockUseHelixOrgWorker(id),
  useFireHelixOrgWorker: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useUpdateWorkerIdentity: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useRestartWorkerAgent: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useListHelixOrgStreams: () => ({ data: { streams: [] }, isLoading: false }),
  useListWorkerSubscriptions: () => ({ data: { subscriptions: [] }, isLoading: false }),
  useSubscribeWorker: () => ({ mutateAsync: vi.fn() }),
  useUnsubscribeWorker: () => ({ mutateAsync: vi.fn() }),
}))

vi.mock('../hooks/useAccount', () => ({
  default: () => ({ organizationTools: { organization: { id: 'org_1' } } }),
}))

vi.mock('../hooks/useRouter', () => ({
  default: () => ({
    params: { org_id: 'acme', worker_id: 'w-ai-1' },
    navigate: vi.fn(),
  }),
}))

vi.mock('../hooks/useSnackbar', () => ({
  default: () => ({ error: vi.fn(), success: vi.fn(), info: vi.fn() }),
}))

vi.mock('../components/system/Page', () => ({
  default: ({ children, topbarContent }: any) => (
    <div>{topbarContent}{children}</div>
  ),
}))
vi.mock('../components/widgets/LoadingSpinner', () => ({ default: () => null }))
vi.mock('../components/widgets/MonacoEditor', () => ({
  default: ({ value }: any) => <div data-testid="monaco">{value}</div>,
}))
vi.mock('../components/widgets/DeleteConfirmWindow', () => ({ default: () => null }))
vi.mock('../components/common/RobustPromptInput', () => ({
  default: ({ sessionId }: any) => <div data-testid="prompt-input">{sessionId}</div>,
}))
vi.mock('../components/session/EmbeddedSessionView', () => ({
  __esModule: true,
  default: forwardRef(({ sessionId }: any, _ref) => (
    <div data-testid="embedded-session">{sessionId}</div>
  )),
}))

const renderPage = () =>
  render(
    <QueryClientProvider client={new QueryClient()}>
      <HelixOrgWorkerDetail />
    </QueryClientProvider>,
  )

const worker = {
  id: 'w-ai-1',
  kind: 'ai',
  role_id: 'r-test',
  organization_id: 'org_1',
  identity_content: '',
}

describe('HelixOrgWorkerDetail inline transcript', () => {
  beforeEach(() => {
    mockSetCurrentSessionId.mockReset()
    mockGetExploratorySession.mockReset()
    mockUseHelixOrgWorker.mockReset()
  })

  it('mounts the transcript + subscribes the socket when a session exists', async () => {
    mockUseHelixOrgWorker.mockReturnValue({
      data: { worker, project_id: 'prj_1' },
      isLoading: false,
    })
    mockGetExploratorySession.mockResolvedValue({ data: { id: 'ses_abc' } })

    renderPage()

    // chatSessionId flips null -> 'ses_abc' after the exploratory-session
    // promise resolves. The useEffect that calls setCurrentSessionId runs
    // after the re-render; the embedded-session DOM update is visible
    // before the effect's next tick fires, so the spy assertion must
    // poll alongside the DOM ones rather than running once after.
    await waitFor(() => {
      expect(screen.getByTestId('embedded-session')).toHaveTextContent('ses_abc')
      expect(screen.getByTestId('prompt-input')).toHaveTextContent('ses_abc')
      expect(mockSetCurrentSessionId).toHaveBeenCalledWith('ses_abc')
    })
  })

  it('shows the empty state and never creates a session on load', async () => {
    mockUseHelixOrgWorker.mockReturnValue({
      data: { worker, project_id: 'prj_1' },
      isLoading: false,
    })
    // 204 No Content → adapter maps to null → no transcript.
    mockGetExploratorySession.mockRejectedValue({ response: { status: 204 } })

    renderPage()

    await waitFor(() => {
      expect(screen.getByText(/No conversation yet/i)).toBeInTheDocument()
    })
    expect(screen.queryByTestId('embedded-session')).toBeNull()
  })

  it('does not resolve a session when the worker has no project', async () => {
    mockUseHelixOrgWorker.mockReturnValue({
      data: { worker, project_id: undefined },
      isLoading: false,
    })

    renderPage()

    await waitFor(() => {
      expect(screen.getByText(/No conversation yet/i)).toBeInTheDocument()
    })
    expect(mockGetExploratorySession).not.toHaveBeenCalled()
  })
})
