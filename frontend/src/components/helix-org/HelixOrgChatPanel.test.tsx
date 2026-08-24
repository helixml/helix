import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { chatBotStorageKey, focusChatBot } from './chatBotFocus'
import HelixOrgChatPanel from './HelixOrgChatPanel'

const DEFAULT_BOTS = [
  { id: 'bot-one', name: 'Agent One', kind: 'bot', agent_status: 'running' },
  { id: 'bot-two', name: 'Agent Two', kind: 'bot', agent_status: 'running' },
]

const mocks = vi.hoisted(() => ({
  router: {
    params: { org_id: 'acme' } as Record<string, string>,
    navigate: vi.fn(),
  },
  setCurrentSessionId: vi.fn(),
  bots: [] as Array<Record<string, unknown>>,
}))

vi.mock('../../hooks/useRouter', () => ({ default: () => mocks.router }))
vi.mock('../../hooks/useApi', () => ({ default: () => ({ getApiClient: vi.fn() }) }))
vi.mock('../../hooks/useSnackbar', () => ({
  default: () => ({ success: vi.fn(), error: vi.fn() }),
}))
vi.mock('../../hooks/useLightTheme', () => ({
  default: () => ({ isLight: true }),
}))
vi.mock('../../contexts/streaming', () => ({
  useStreaming: () => ({ setCurrentSessionId: mocks.setCurrentSessionId, currentResponses: new Map() }),
}))
vi.mock('../../services/helixOrgService', () => ({
  useListHelixOrgBots: () => ({ data: mocks.bots }),
  useHelixOrgBot: (botId?: string) => ({ data: botId ? { project_id: `project-${botId}` } : undefined, refetch: vi.fn() }),
  useActivateBot: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useStopBotAgent: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useRestartBotAgent: () => ({ mutateAsync: vi.fn(), isPending: false }),
}))
vi.mock('../../services/workerChatSession', () => ({
  fetchExistingWorkerSession: (projectId: string) => Promise.resolve(`session-${projectId}`),
}))
vi.mock('../session/AgentChat', () => ({
  default: ({ placeholder }: { placeholder: string }) => <div>{placeholder}</div>,
}))
vi.mock('../external-agent/ExternalAgentDesktopViewer', () => ({
  default: () => <div>Desktop viewer</div>,
}))
vi.mock('./HelixOrgBotPanelTab', () => ({ default: () => <div>Tasks view</div> }))

// Renders the panel. Passing `bot` replaces the org's bot list with a single
// bot and selects it via the bot_id query param — used by tests that need to
// control fields (like restart_required) on the selected bot specifically.
// With no `bot`, it reproduces the default two-bot fixture the query-selection
// tests rely on.
const renderPanel = (overrides?: { bot?: Record<string, unknown> & { id: string } }) => {
  if (overrides?.bot) {
    mocks.bots = [{ kind: 'bot', name: overrides.bot.id, ...overrides.bot }]
    mocks.router.params = { org_id: 'acme', bot_id: overrides.bot.id }
  }
  return render(<HelixOrgChatPanel />)
}

describe('HelixOrgChatPanel query selection', () => {
  beforeEach(() => {
    localStorage.clear()
    mocks.router.params = { org_id: 'acme' }
    mocks.setCurrentSessionId.mockReset()
    mocks.bots = DEFAULT_BOTS.map((b) => ({ ...b }))
  })

  it('prioritizes bot_id and switches back to chat when the query changes', async () => {
    const view = renderPanel()
    await screen.findByText('Message Agent One…')
    fireEvent.click(screen.getByRole('button', { name: 'Desktop' }))
    expect(await screen.findByText('Desktop viewer')).toBeInTheDocument()

    mocks.router.params = { org_id: 'acme', bot_id: 'bot-two' }
    view.rerender(<HelixOrgChatPanel />)

    expect(await screen.findByText('Message Agent Two…')).toBeInTheDocument()
    await waitFor(() => {
      expect(localStorage.getItem(chatBotStorageKey('acme'))).toBe('bot-two')
    })

    act(() => focusChatBot('acme', 'bot-one'))
    expect(await screen.findByText('Message Agent One…')).toBeInTheDocument()
  })

  it('shows the restart banner when the selected bot reports stale config', async () => {
    renderPanel({ bot: { id: 'b-one', agent_status: 'running', restart_required: true } })
    expect(await screen.findByTestId('agent-restart-required-banner')).toBeInTheDocument()
  })

  it('hides the restart banner when config is current', async () => {
    renderPanel({ bot: { id: 'b-one', agent_status: 'running', restart_required: false } })
    await screen.findByText('Message b-one…')
    expect(screen.queryByTestId('agent-restart-required-banner')).toBeNull()
  })

  it('keeps the restart banner visible after switching away from the chat tab', async () => {
    renderPanel({ bot: { id: 'b-one', agent_status: 'running', restart_required: true } })
    await screen.findByTestId('agent-restart-required-banner')

    fireEvent.click(screen.getByRole('button', { name: 'Desktop' }))
    expect(await screen.findByText('Desktop viewer')).toBeInTheDocument()
    expect(screen.getByTestId('agent-restart-required-banner')).toBeInTheDocument()
  })

  it('does not re-arm a dismissed restart banner on a tab switch', async () => {
    renderPanel({ bot: { id: 'b-one', agent_status: 'running', restart_required: true } })
    await screen.findByTestId('agent-restart-required-banner')

    fireEvent.click(screen.getByRole('button', { name: 'Not now' }))
    expect(screen.queryByTestId('agent-restart-required-banner')).toBeNull()

    fireEvent.click(screen.getByRole('button', { name: 'Desktop' }))
    await screen.findByText('Desktop viewer')
    expect(screen.queryByTestId('agent-restart-required-banner')).toBeNull()
  })

  it('re-arms the restart banner when switching to a different stale bot', async () => {
    mocks.bots = [
      { id: 'b-one', name: 'Bot One', kind: 'bot', agent_status: 'running', restart_required: true },
      { id: 'b-two', name: 'Bot Two', kind: 'bot', agent_status: 'running', restart_required: true },
    ]
    mocks.router.params = { org_id: 'acme', bot_id: 'b-one' }
    render(<HelixOrgChatPanel />)

    await screen.findByTestId('agent-restart-required-banner')
    fireEvent.click(screen.getByRole('button', { name: 'Not now' }))
    expect(screen.queryByTestId('agent-restart-required-banner')).toBeNull()

    // Switch selection to the other stale bot via the header dropdown.
    fireEvent.mouseDown(screen.getByRole('combobox'))
    fireEvent.click(within(screen.getByRole('listbox')).getByText('Bot Two'))

    await screen.findByText('Message Bot Two…')
    expect(await screen.findByTestId('agent-restart-required-banner')).toBeInTheDocument()
  })
})
