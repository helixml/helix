import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import OrgBotSessionResolver from './OrgBotSessionResolver'

const mocks = vi.hoisted(() => ({
  activate: vi.fn(),
  list: vi.fn(),
  refetch: vi.fn(),
  consumeDraft: vi.fn(),
  appendDraft: vi.fn(),
  bots: [] as Array<{ id: string; name?: string; session_id?: string }>,
  listLoading: false,
  listError: false,
}))

vi.mock('../components/helix-org/orgBotChatDraft', () => ({
  consumeOrgBotChatDraft: mocks.consumeDraft,
}))

vi.mock('../hooks/usePromptHistory', () => ({
  appendPromptDraft: mocks.appendDraft,
}))

vi.mock('../hooks/useRouter', () => ({
  default: () => ({
    params: { org_id: 'my-org', bot_id: 'chief-of-staff' },
  }),
}))

vi.mock('./Session', () => ({
  default: ({ sessionId }: { sessionId: string }) => (
    <div data-testid="resolved-session">{sessionId}</div>
  ),
}))

vi.mock('../services/helixOrgService', () => ({
  useListHelixOrgBots: (options: object) => {
    mocks.list(options)
    return {
      data: mocks.bots,
      isLoading: mocks.listLoading,
      isError: mocks.listError,
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
    mocks.listError = false
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
    mocks.bots = [{ id: 'chief-of-staff', session_id: 'ses-existing' }]

    render(<OrgBotSessionResolver />)

    expect(await screen.findByTestId('resolved-session')).toHaveTextContent('ses-existing')
    expect(mocks.activate).not.toHaveBeenCalled()
    expect(mocks.list).toHaveBeenNthCalledWith(1, {
      enabled: true,
      refetchInterval: 2000,
    })
    expect(mocks.list).toHaveBeenLastCalledWith({
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

  it('offers query retry when the bot list fails', async () => {
    mocks.listError = true
    render(<OrgBotSessionResolver />)

    expect(screen.getByRole('alert')).toHaveTextContent(
      'Could not find this agent.',
    )
    fireEvent.click(screen.getByRole('button', {
      name: 'Retry finding agent',
    }))

    expect(mocks.refetch).toHaveBeenCalledTimes(1)
    expect(mocks.activate).not.toHaveBeenCalled()
  })

  it('offers query retry when the bot no longer exists', () => {
    render(<OrgBotSessionResolver />)

    expect(screen.getByRole('alert')).toHaveTextContent(
      'Could not find this agent.',
    )
    expect(mocks.activate).not.toHaveBeenCalled()
  })

  it('keeps showing the lookup progress while the bot list is loading', () => {
    mocks.listLoading = true
    render(<OrgBotSessionResolver />)

    expect(screen.getByText('Finding your agent')).toBeInTheDocument()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })
})
