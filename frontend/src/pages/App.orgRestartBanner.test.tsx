// Covers the restart-required banner mount on the Agent settings page
// (route `org_agent`, `<App />`). This is a heavy page component with many
// data hooks; every one is mocked below so the test exercises only the
// wiring this change touches: resolving the Bot backing this App via
// `useListHelixOrgBots().agent_id`, and driving
// `AgentRestartRequiredBanner.visible` off `bot.restart_required`.

import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import App from './App'
import { AGENT_KIND_ORG } from '../types'

const mocks = vi.hoisted(() => ({
  router: {
    params: { org_id: 'acme', app_id: 'app-target', tab: 'general' } as Record<string, string>,
    navigate: vi.fn(),
    mergeParams: vi.fn(),
  },
  bots: [] as Array<{ id: string; agent_id: string; restart_required?: boolean }>,
  restartMutateAsync: vi.fn(),
  restartIsPending: false,
}))

const flatApp = { name: 'Target Agent', default_agent_type: 'text' } as any

vi.mock('../hooks/useRouter', () => ({ default: () => mocks.router }))
vi.mock('../hooks/useApi', () => ({ default: () => ({ getApiClient: () => ({}), delete: vi.fn() }) }))
vi.mock('../hooks/useSnackbar', () => ({
  default: () => ({ success: vi.fn(), error: vi.fn(), info: vi.fn() }),
}))
vi.mock('../hooks/useAccount', () => ({
  default: () => ({
    user: { id: 'user-1' },
    admin: false,
    orgNavigate: vi.fn(),
    appApiKeys: [],
    addAppAPIKey: vi.fn(),
    serverConfig: {},
  }),
}))
vi.mock('../hooks/useThemeConfig', () => ({ default: () => ({}) }))
vi.mock('../hooks/useLightTheme', () => ({
  default: () => ({ isLight: true, border: '1px solid #eee', scrollbar: {} }),
}))
vi.mock('../hooks/useApp', () => ({
  default: () => ({
    app: { id: 'app-target', agent_kind: AGENT_KIND_ORG },
    flatApp,
    id: 'app-target',
    isReadOnly: false,
    isSafeToSave: true,
    isAppSaving: false,
    showErrors: false,
    accessGrants: [],
    createAccessGrant: vi.fn(),
    deleteAccessGrant: vi.fn(),
    userAccess: { isAdmin: false },
    saveFlatApp: vi.fn(),
    loadApp: vi.fn(),
  }),
}))
vi.mock('../services/helixOrgService', () => ({
  useActivateBot: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useListHelixOrgBotDetails: () => [],
  useListHelixOrgBots: () => ({ data: mocks.bots, isLoading: false }),
  useRestartBotAgent: () => ({
    mutateAsync: mocks.restartMutateAsync,
    isPending: mocks.restartIsPending,
  }),
}))
vi.mock('../components/system/Page', () => ({
  default: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
}))
vi.mock('../components/app/FocusedAgentDetails', () => ({
  default: () => <div>Focused agent details</div>,
}))
vi.mock('../components/helix-org/HelixOrgTopNav', () => ({ default: () => null }))

describe('App agent settings page restart banner', () => {
  beforeEach(() => {
    mocks.router.params = { org_id: 'acme', app_id: 'app-target', tab: 'general' }
    mocks.bots = []
    mocks.restartMutateAsync.mockReset()
    mocks.restartIsPending = false
  })

  it('shows the restart banner when the matched bot reports stale config', async () => {
    mocks.bots = [{ id: 'bot-one', agent_id: 'app-target', restart_required: true }]
    render(<App />)
    expect(await screen.findByTestId('agent-restart-required-banner')).toBeInTheDocument()
  })

  it('does not show the restart banner when the matched bot is current', async () => {
    mocks.bots = [{ id: 'bot-one', agent_id: 'app-target', restart_required: false }]
    render(<App />)
    await screen.findByText('Focused agent details')
    expect(screen.queryByTestId('agent-restart-required-banner')).toBeNull()
  })

  it('does not show the restart banner when no bot matches this app', async () => {
    mocks.bots = [{ id: 'bot-other', agent_id: 'app-other', restart_required: true }]
    render(<App />)
    await screen.findByText('Focused agent details')
    expect(screen.queryByTestId('agent-restart-required-banner')).toBeNull()
  })
})
