import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import HelixOrgBotDetail from './HelixOrgBotDetail'

const mocks = vi.hoisted(() => ({
  bot: { id: 'b-one', name: 'Agent One', status: 'stopped' } as Record<string, unknown>,
  refetch: vi.fn(),
  restart: vi.fn(),
  reset: vi.fn(),
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
  useRestartBotAgent: () => ({ mutateAsync: mocks.restart, isPending: false }),
  useResetBot: () => ({ mutateAsync: mocks.reset, isPending: false }),
  useBotResetPlan: () => ({
    data: {
      scopes: [
        { scope: 'session', available: true, erases: ['current session'], keeps: ['bot home'] },
        { scope: 'session_and_home', available: true, erases: ['current session', 'bot home'], keeps: ['other bots'] },
      ],
    },
    isLoading: false,
  }),
  useListHelixOrgTools: () => ({ data: [] }),
  useListHelixOrgProcessors: () => ({ data: [] }),
}))

describe('HelixOrgBotDetail lifecycle controls', () => {
  beforeEach(() => {
    mocks.bot = { id: 'b-one', name: 'Agent One', status: 'running', can_edit: true }
    mocks.refetch.mockReset()
    mocks.restart.mockReset().mockResolvedValue({ mode: 'runtime', session_id: 'ses_1' })
    mocks.reset.mockReset().mockResolvedValue({ previous_session_id: 'ses_1' })
  })

  it('offers only the non-destructive Restart in the three-dot menu', async () => {
    render(<HelixOrgBotDetail />)
    fireEvent.click(screen.getByLabelText('Org bot session actions'))
    const menu = await screen.findByRole('menu')
    expect(within(menu).queryByText(/reset/i)).toBeNull()
    fireEvent.click(within(menu).getByText('Restart org bot'))
    await waitFor(() => expect(mocks.restart).toHaveBeenCalledWith('b-one'))
    expect(mocks.reset).not.toHaveBeenCalled()
  })

  it('places Reset bot only in Advanced > Danger zone and routes it to reset', async () => {
    render(<HelixOrgBotDetail />)
    expect(screen.queryByText('Reset bot')).toBeNull()
    fireEvent.click(screen.getByTestId('bot-advanced-toggle'))
    const zone = await screen.findByTestId('bot-danger-zone')
    expect(within(zone).getByText('Danger zone')).toBeInTheDocument()
    fireEvent.click(within(zone).getByRole('button', { name: /reset bot/i }))

    const dialog = await screen.findByRole('dialog')
    fireEvent.click(within(dialog).getByLabelText('Reset session and bot home'))
    fireEvent.change(within(dialog).getByTestId('reset-confirm-input'), { target: { value: 'Agent One' } })
    fireEvent.click(within(dialog).getByTestId('reset-confirm'))
    await waitFor(() => expect(mocks.reset).toHaveBeenCalledWith({ botId: 'b-one', scope: 'session_and_home' }))
    expect(mocks.restart).not.toHaveBeenCalled()
  })

  it('disables Reset for a member who cannot edit the bot', async () => {
    mocks.bot = { id: 'b-one', name: 'Agent One', status: 'running', can_edit: false }
    render(<HelixOrgBotDetail />)
    fireEvent.click(screen.getByTestId('bot-advanced-toggle'))
    const zone = await screen.findByTestId('bot-danger-zone')
    expect(within(zone).getByRole('button', { name: /reset bot/i })).toBeDisabled()
  })
})
