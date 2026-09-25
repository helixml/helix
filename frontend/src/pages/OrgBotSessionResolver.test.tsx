import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { renderToString } from 'react-dom/server'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import OrgBotSessionResolver from './OrgBotSessionResolver'

const mocks = vi.hoisted(() => ({
  activate: vi.fn(),
  list: vi.fn(),
  refetch: vi.fn(),
  consumeDraft: vi.fn(),
  appendDraft: vi.fn(),
  bots: [] as Array<{ id: string; name?: string; session_id?: string; status?: string }>,
  listLoading: false,
  listFetching: false,
  listError: false,
  errorStatus: 404,
  params: { org_id: 'my-org', bot_id: 'chief-of-staff' } as Record<string, string>,
  navigateReplace: vi.fn(),
}))

vi.mock('../components/helix-org/orgBotChatDraft', () => ({
  consumeOrgBotChatDraft: mocks.consumeDraft,
}))

vi.mock('../hooks/usePromptHistory', () => ({
  appendPromptDraft: mocks.appendDraft,
}))

vi.mock('../hooks/useRouter', () => ({
  default: () => ({
    params: mocks.params,
    navigateReplace: mocks.navigateReplace,
  }),
}))

vi.mock('./Session', () => ({
  default: ({ sessionId }: { sessionId: string }) => (
    <div data-testid="resolved-session">{sessionId}</div>
  ),
}))

vi.mock('../services/helixOrgService', () => ({
  useHelixOrgBot: (botId: string, options: object) => {
    mocks.list({ botId, ...options })
    const bot = mocks.bots.find((candidate) => candidate.id === botId)
    return {
      data: bot ? { bot } : undefined,
      isLoading: mocks.listLoading,
      isFetching: mocks.listFetching,
      isError: mocks.listError,
      error: mocks.listError ? { response: { status: mocks.errorStatus } } : null,
      refetch: mocks.refetch,
    }
  },
  useActivateBot: () => ({ mutateAsync: mocks.activate, isPending: false }),
}))

describe('OrgBotSessionResolver', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.bots = []
    mocks.listLoading = false
    mocks.listFetching = false
    mocks.listError = false
    mocks.errorStatus = 404
    mocks.params = { org_id: 'my-org', bot_id: 'chief-of-staff' }
    mocks.navigateReplace.mockReset()
    mocks.activate.mockResolvedValue({})
    mocks.refetch.mockResolvedValue({})
    mocks.consumeDraft.mockReturnValue('')
  })

  it('moves a queued draft into the durable session before rendering it', async () => {
    mocks.bots = [{ id: 'chief-of-staff', session_id: 'ses-existing' }]
    mocks.consumeDraft.mockReturnValue('I would like to create a new bot')

    render(<OrgBotSessionResolver />)

    expect(await screen.findByTestId('resolved-session')).toHaveTextContent('ses-existing')
    expect(mocks.consumeDraft).toHaveBeenCalledWith('my-org', 'chief-of-staff')
    expect(mocks.appendDraft).toHaveBeenCalledWith(
      'ses-existing',
      'I would like to create a new bot',
    )
  })

  it('renders an existing durable session at the bot route without activating the bot', async () => {
    mocks.bots = [{ id: 'chief-of-staff', session_id: 'ses-existing', status: 'stopped' }]

    expect(renderToString(<OrgBotSessionResolver />)).not.toContain('Meet your')

    render(<OrgBotSessionResolver />)

    expect(await screen.findByTestId('resolved-session')).toHaveTextContent('ses-existing')
    expect(mocks.activate).not.toHaveBeenCalled()
    expect(mocks.list).toHaveBeenNthCalledWith(1, {
      botId: 'chief-of-staff',
      enabled: true,
      refetchInterval: 2000,
    })
    expect(mocks.list).toHaveBeenLastCalledWith({
      botId: 'chief-of-staff',
      enabled: true,
      refetchInterval: 10000,
    })
  })

  it('activates once and renders the durable session when polling finds it', async () => {
    mocks.bots = [{ id: 'chief-of-staff' }]
    const view = render(<OrgBotSessionResolver />)

    await waitFor(() => expect(mocks.activate).toHaveBeenCalledTimes(1))
    mocks.bots = [{ id: 'chief-of-staff', session_id: 'ses-started' }]
    view.rerender(<OrgBotSessionResolver />)

    expect(await screen.findByTestId('resolved-session')).toHaveTextContent('ses-started')
    expect(mocks.activate).toHaveBeenCalledTimes(1)
  })

  it('switches to a recreated session without changing routes', async () => {
    mocks.bots = [{ id: 'chief-of-staff', session_id: 'ses-original' }]
    const view = render(<OrgBotSessionResolver />)

    expect(await screen.findByTestId('resolved-session')).toHaveTextContent('ses-original')
    mocks.bots = [{ id: 'chief-of-staff', session_id: 'ses-recreated' }]
    view.rerender(<OrgBotSessionResolver />)

    expect(await screen.findByTestId('resolved-session')).toHaveTextContent('ses-recreated')
    expect(mocks.activate).not.toHaveBeenCalled()
  })

  it('introduces the selected agent without offering a premature retry', async () => {
    mocks.params = { org_id: 'my-org', bot_id: 'chief-of-staff', intro: '1' }
    mocks.bots = [{ id: 'chief-of-staff', name: 'Chief of Staff' }]
    render(<OrgBotSessionResolver />)

    await waitFor(() => expect(mocks.activate).toHaveBeenCalledTimes(1))
    expect(screen.getByRole('heading', { name: 'Meet your Chief of Staff' })).toBeInTheDocument()
    expect(screen.getByText(/new agent is getting ready/i)).toBeInTheDocument()
    expect(screen.getByText('Preparing Chief of Staff')).toBeInTheDocument()
    expect(screen.getByText('Finding your agent')).toBeInTheDocument()
    expect(screen.getByText('Starting a secure workspace')).toBeInTheDocument()
    expect(screen.getByText('Opening your conversation')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /retry/i })).not.toBeInTheDocument()
  })

  it('closes the introduction when navigating to the normal chat route for the same bot', async () => {
    mocks.params = { org_id: 'my-org', bot_id: 'chief-of-staff', intro: '1' }
    mocks.bots = [{ id: 'chief-of-staff', name: 'Chief of Staff', status: 'stopped' }]
    const view = render(<OrgBotSessionResolver />)
    expect(await screen.findByRole('heading', { name: 'Meet your Chief of Staff' })).toBeInTheDocument()

    mocks.params = { org_id: 'my-org', bot_id: 'chief-of-staff' }
    view.rerender(<OrgBotSessionResolver />)

    expect(screen.queryByRole('heading', { name: 'Meet your Chief of Staff' })).not.toBeInTheDocument()
    expect(screen.getByRole('status')).toHaveTextContent('Opening chat')
  })

  it('opens an unactivated bot from the chat list without the onboarding introduction', async () => {
    mocks.bots = [{ id: 'chief-of-staff', name: 'Chief of Staff', status: 'stopped' }]
    render(<OrgBotSessionResolver />)

    await waitFor(() => expect(mocks.activate).toHaveBeenCalledTimes(1))
    expect(screen.getByRole('status')).toHaveTextContent('Opening chat')
    expect(screen.queryByRole('heading', { name: /Meet your/i })).not.toBeInTheDocument()
  })

  it('offers retry when activation fails', async () => {
    mocks.bots = [{ id: 'chief-of-staff' }]
    mocks.activate.mockRejectedValueOnce(new Error('failed'))
    render(<OrgBotSessionResolver />)

    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Could not start chief-of-staff.',
    )
    const retry = screen.getByRole('button', {
      name: 'Retry starting chief-of-staff',
    })
    mocks.activate.mockResolvedValue({})
    fireEvent.click(retry)

    await waitFor(() => expect(mocks.activate).toHaveBeenCalledTimes(2))
  })

  it('offers query retry when the bot detail lookup fails', async () => {
    mocks.listError = true
    mocks.errorStatus = 500
    render(<OrgBotSessionResolver />)

    expect(screen.getByRole('alert')).toHaveTextContent(
      'Could not load this agent.',
    )
    fireEvent.click(screen.getByRole('button', {
      name: 'Retry finding agent',
    }))

    expect(mocks.refetch).toHaveBeenCalledTimes(1)
    expect(mocks.activate).not.toHaveBeenCalled()
  })

  it('returns to Chat when the landing Chief of Staff no longer exists', async () => {
    mocks.listError = true
    render(<OrgBotSessionResolver />)

    await waitFor(() => expect(mocks.navigateReplace).toHaveBeenCalledWith('org_chat', {
      org_id: 'my-org',
    }))
    expect(mocks.activate).not.toHaveBeenCalled()
  })

  it('keeps the missing-agent error for a non-landing bot', () => {
    mocks.params = { org_id: 'my-org', bot_id: 'deleted-bot' }
    mocks.listError = true
    render(<OrgBotSessionResolver />)

    expect(screen.getByRole('alert')).toHaveTextContent('Could not find this agent.')
    expect(mocks.navigateReplace).not.toHaveBeenCalled()
  })

  it('keeps showing the lookup progress while the bot list is loading', () => {
    mocks.listLoading = true
    render(<OrgBotSessionResolver />)

    expect(screen.getByRole('status')).toHaveTextContent('Opening chat')
    expect(screen.queryByRole('heading', { name: /Meet your/i })).not.toBeInTheDocument()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })

  it('does not introduce or activate a bot from stale detail data during a refetch', () => {
    mocks.params = { org_id: 'my-org', bot_id: 'chief-of-staff', intro: '1' }
    mocks.bots = [{ id: 'chief-of-staff', name: 'Chief of Staff', status: 'stopped' }]
    mocks.listFetching = true
    const view = render(<OrgBotSessionResolver />)

    expect(screen.getByRole('status')).toHaveTextContent('Opening chat')
    expect(screen.queryByRole('heading', { name: /Meet your/i })).not.toBeInTheDocument()
    expect(mocks.activate).not.toHaveBeenCalled()

    mocks.bots = [{ id: 'chief-of-staff', name: 'Chief of Staff', session_id: 'ses-existing' }]
    mocks.listFetching = false
    view.rerender(<OrgBotSessionResolver />)
    expect(screen.getByTestId('resolved-session')).toHaveTextContent('ses-existing')
    expect(mocks.activate).not.toHaveBeenCalled()
  })

  it('waits for a running bot to report its session instead of introducing it', () => {
    mocks.bots = [{ id: 'chief-of-staff', name: 'Chief of Staff', status: 'running' }]
    render(<OrgBotSessionResolver />)

    expect(screen.getByRole('status')).toHaveTextContent('Opening chat')
    expect(screen.queryByRole('heading', { name: /Meet your/i })).not.toBeInTheDocument()
    expect(mocks.activate).not.toHaveBeenCalled()
  })

  it('does not carry a previous bot’s activation error into a switch', async () => {
    mocks.bots = [{ id: 'chief-of-staff' }, { id: 'another-bot', status: 'running' }]
    mocks.activate.mockRejectedValueOnce(new Error('failed'))
    const view = render(<OrgBotSessionResolver />)
    expect(await screen.findByRole('alert')).toHaveTextContent('Could not start chief-of-staff.')

    mocks.params = { org_id: 'my-org', bot_id: 'another-bot' }
    view.rerender(<OrgBotSessionResolver />)

    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    expect(screen.getByRole('status')).toHaveTextContent('Opening chat')
    expect(mocks.activate).toHaveBeenCalledTimes(1)
  })

  it('polls promptly when switching away from a resolved session', async () => {
    mocks.bots = [
      { id: 'chief-of-staff', session_id: 'ses-existing' },
      { id: 'another-bot', status: 'running' },
    ]
    const view = render(<OrgBotSessionResolver />)
    expect(await screen.findByTestId('resolved-session')).toHaveTextContent('ses-existing')

    mocks.params = { org_id: 'my-org', bot_id: 'another-bot' }
    view.rerender(<OrgBotSessionResolver />)

    expect(screen.getByRole('status')).toHaveTextContent('Opening chat')
    expect(mocks.list).toHaveBeenLastCalledWith({
      botId: 'another-bot', enabled: true, refetchInterval: 2000,
    })
  })
})
