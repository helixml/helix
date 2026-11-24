import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import HelixOrgBotDetail from './HelixOrgBotDetail'

const mocks = vi.hoisted(() => ({
  bot: { id: 'b-one', name: 'Agent One', status: 'stopped' } as Record<string, unknown>,
  refetch: vi.fn(),
}))

vi.mock('react-router5', () => ({
  useRouter: () => ({ navigate: vi.fn(), buildPath: vi.fn(() => '/project') }),
}))
vi.mock('../hooks/useRouter', () => ({
  default: () => ({ params: { org_id: 'acme', bot_id: 'b-one' }, navigate: vi.fn() }),
}))
vi.mock('../hooks/useApi', () => ({ default: () => ({ getApiClient: () => ({}) }) }))
vi.mock('../hooks/useSnackbar', () => ({ default: () => ({ success: vi.fn(), error: vi.fn() }) }))
vi.mock('../contexts/streaming', () => ({ useStreaming: () => ({ currentResponses: new Map() }) }))
vi.mock('../components/helix-org/useHelixOrgBreadcrumbs', () => ({ default: () => [] }))
vi.mock('../components/helix-org/HelixOrgShell', () => ({
  default: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
}))
vi.mock('../components/helix-org/AgentRestartRequiredBanner', () => ({ default: () => null }))
vi.mock('../components/helix-org/BotRuntimeForm', () => ({ default: () => null }))
vi.mock('../components/helix-org/ToolPickerDialog', () => ({ default: () => null }))
vi.mock('../components/helix-org/WorkerSecretsPanel', () => ({ default: () => null }))
vi.mock('../components/widgets/LoadingSpinner', () => ({ default: () => null }))
vi.mock('../components/widgets/MonacoEditor', () => ({ default: () => null }))
vi.mock('../components/widgets/DeleteConfirmWindow', () => ({ default: () => null }))
vi.mock('../services/projectService', () => ({ useListProjects: () => ({ data: [] }) }))
vi.mock('../services/workerChatSession', () => ({ fetchExistingWorkerSession: () => Promise.resolve(null) }))
vi.mock('../services/sessionService', () => ({ useSwitchAgent: () => ({ mutateAsync: vi.fn() }) }))
vi.mock('../services/triggerService', () => ({
  useBotAttachments: () => ({ data: [] }),
  useCreateBotAttachment: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useDeleteBotAttachment: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useTriggers: () => ({ data: [] }),
}))
vi.mock('../services/helixOrgService', () => ({
  useDeleteBot: () => ({ mutateAsync: vi.fn(), isPending: false, isSuccess: false }),
  useHelixOrgBot: () => ({
    data: { bot: mocks.bot, project_id: 'project-one', legacy_app_id: 'app-one' },
    isLoading: false,
    refetch: mocks.refetch,
  }),
  useUpdateBot: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useActivateBot: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useStopBotAgent: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useRestartBotAgent: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useResetBot: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useBotResetPlan: () => ({ data: undefined, isLoading: false }),
  useListHelixOrgTools: () => ({ data: [] }),
  useListHelixOrgProcessors: () => ({ data: [] }),
}))

describe('HelixOrgBotDetail activation error', () => {
  beforeEach(() => {
    mocks.bot = { id: 'b-one', name: 'Agent One', status: 'stopped' }
    mocks.refetch.mockReset()
  })

  it('shows an asynchronous activation error returned by bot polling', async () => {
    const view = render(<HelixOrgBotDetail />)
    expect(screen.queryByRole('alert')).toBeNull()

    mocks.bot = {
      id: 'b-one',
      name: 'Agent One',
      status: 'stopped',
      activation_error: '32 bot activations are already running for this organization; stop another bot and try again.',
    }
    view.rerender(<HelixOrgBotDetail />)

    expect(await screen.findByRole('alert')).toHaveTextContent(
      '32 bot activations are already running for this organization; stop another bot and try again.',
    )
  })

  it('warns about repeated activation failures even when chat cleared the latest error', () => {
    mocks.bot = {
      id: 'b-one',
      name: 'Agent One',
      status: 'running',
      activation_failure_streak: 3,
      last_activation_failure: "session error: The AI agent didn't come online in time",
    }
    render(<HelixOrgBotDetail />)

    const alert = screen.getByRole('alert')
    expect(alert).toHaveTextContent('The last 3 activations of this bot failed')
    expect(alert).toHaveTextContent("didn't come online in time")
  })

  it('does not warn about a single failure', () => {
    mocks.bot = { id: 'b-one', name: 'Agent One', status: 'running', activation_failure_streak: 1, last_activation_failure: 'once' }
    render(<HelixOrgBotDetail />)
    expect(screen.queryByRole('alert')).toBeNull()
  })
})
