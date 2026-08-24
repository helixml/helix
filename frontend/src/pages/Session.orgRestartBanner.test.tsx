// Covers the restart-required banner mount on the full-page org session
// chat surface (route `org_session`, `<Session orgChatView />`). This is a
// heavy page component with many data hooks; every one is mocked below so
// the test exercises only the wiring this change touches: deriving the bot
// id from `session.data.config.org_worker_id`, looking the bot up via
// `useHelixOrgBot`, and driving `AgentRestartRequiredBanner.visible` off
// `bot.restart_required`.

import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import Session from './Session'

class StubIntersectionObserver implements IntersectionObserver {
  root: Element | Document | null = null
  rootMargin = ''
  thresholds: ReadonlyArray<number> = []
  observe() {}
  unobserve() {}
  disconnect() {}
  takeRecords(): IntersectionObserverEntry[] { return [] }
}
// jsdom does not implement IntersectionObserver; Session.tsx's virtualized
// scroll-loading effect constructs one on mount regardless of interaction
// count, so it needs a stub even though this test never triggers it.
;(global as any).IntersectionObserver = StubIntersectionObserver
// jsdom does not implement Element.scrollTo either; Session.tsx calls it
// from a couple of setTimeout-deferred scroll effects that fire regardless
// of what this test asserts on.
Element.prototype.scrollTo = vi.fn()

const mocks = vi.hoisted(() => ({
  router: {
    params: { org_id: 'acme', session_id: 'ses-1' } as Record<string, string>,
    navigate: vi.fn(),
    setParams: vi.fn(),
  },
  restartRequired: false,
  restartMutateAsync: vi.fn(),
  restartIsPending: false,
}))

const sessionData = {
  id: 'ses-1',
  name: 'Org bot session',
  owner: 'user-1',
  organization_id: 'acme',
  type: 'text',
  mode: 'inference',
  interactions: [] as unknown[],
  config: { org_worker_id: 'bot-one' },
}

vi.mock('../hooks/useRouter', () => ({ default: () => mocks.router }))
vi.mock('../hooks/useApi', () => ({ default: () => ({ getApiClient: () => ({}) }) }))
vi.mock('../hooks/useSnackbar', () => ({
  default: () => ({ success: vi.fn(), error: vi.fn(), info: vi.fn() }),
}))
vi.mock('../hooks/useAccount', () => ({
  default: () => ({
    user: { id: 'user-1' },
    admin: false,
    serverConfig: {},
    setMobileMenuOpen: vi.fn(),
    onLogin: vi.fn(),
  }),
}))
vi.mock('../hooks/useLightTheme', () => ({
  default: () => ({ isLight: true, border: '1px solid #eee', scrollbar: {} }),
}))
vi.mock('../hooks/useSubscriptionGate', () => ({
  default: () => ({ paywallActive: false, navigateToBilling: vi.fn() }),
}))
vi.mock('../hooks/useCodeAgentConfigChange', () => ({ default: () => vi.fn() }))
vi.mock('@mui/material/useMediaQuery', () => ({ default: () => true }))
vi.mock('../contexts/streaming', () => ({
  useStreaming: () => ({ NewInference: vi.fn(), setCurrentSessionId: vi.fn() }),
}))
vi.mock('../services/sessionService', () => ({
  useGetSession: () => ({ data: { data: sessionData }, refetch: vi.fn() }),
  useUpdateSession: () => ({ mutate: vi.fn() }),
  useGetSessionIdleStatus: () => ({ data: undefined }),
  useGetSessionExecutionConfig: () => ({ data: undefined }),
  useUpdateSessionExecutionConfig: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useListSessionSteps: () => ({ data: [] }),
}))
vi.mock('../services/userService', () => ({
  useGetConfig: () => ({ data: { edition: 'self-hosted' } }),
}))
vi.mock('../services/projectService', () => ({
  useGetProject: () => ({ data: undefined }),
}))
vi.mock('../services/helixOrgService', () => ({
  useHelixOrgBot: (botId?: string) => ({
    data: botId ? { bot: { id: botId, restart_required: mocks.restartRequired } } : undefined,
  }),
  useRestartBotAgent: () => ({
    mutateAsync: mocks.restartMutateAsync,
    isPending: mocks.restartIsPending,
  }),
}))
vi.mock('../components/system/Page', () => ({
  default: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
}))
vi.mock('../components/helix-org/OrgAgentSessionWorkspace', () => ({
  default: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
}))
vi.mock('../components/session/InteractionLiveStream', () => ({ default: () => null }))
vi.mock('../components/session/Interaction', () => ({ default: () => null }))
vi.mock('../components/session/SessionToolbar', () => ({ default: () => null }))
vi.mock('../components/session/ChatTurnNavigator', () => ({ default: () => null }))
vi.mock('../components/common/RobustPromptInput', () => ({ default: () => <div>Prompt input</div> }))

describe('Session org chat restart banner', () => {
  beforeEach(() => {
    mocks.router.params = { org_id: 'acme', session_id: 'ses-1' }
    mocks.restartRequired = false
    mocks.restartMutateAsync.mockReset()
    mocks.restartIsPending = false
  })

  it('shows the restart banner when the session bot reports stale config', async () => {
    mocks.restartRequired = true
    render(<Session orgChatView />)
    expect(await screen.findByTestId('agent-restart-required-banner')).toBeInTheDocument()
  })

  it('does not show the restart banner when the bot config is current', async () => {
    mocks.restartRequired = false
    render(<Session orgChatView />)
    await screen.findByText('Prompt input')
    expect(screen.queryByTestId('agent-restart-required-banner')).toBeNull()
  })
})
