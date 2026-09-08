import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import OrgBotSessionResolver from './OrgBotSessionResolver'

const mocks = vi.hoisted(() => ({
  activate: vi.fn(),
  list: vi.fn(),
  refetch: vi.fn(),
  navigateReplace: vi.fn(),
  bots: [] as Array<{ id: string; name?: string; session_id?: string }>,
  listError: false,
}))

vi.mock('../hooks/useRouter', () => ({
  default: () => ({
    params: { org_id: 'my-org', bot_id: 'chief-of-staff' },
    navigateReplace: mocks.navigateReplace,
  }),
}))

vi.mock('../services/helixOrgService', () => ({
  useListHelixOrgBots: (options: object) => {
    mocks.list(options)
    return {
      data: mocks.bots,
      isLoading: false,
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
    mocks.listError = false
    mocks.activate.mockResolvedValue({})
    mocks.refetch.mockResolvedValue({})
  })

  it('opens an existing durable session without activating the bot', async () => {
    mocks.bots = [{ id: 'chief-of-staff', session_id: 'ses-existing' }]

    render(<OrgBotSessionResolver />)

    await waitFor(() => expect(mocks.navigateReplace).toHaveBeenCalledWith(
      'org_session',
      { org_id: 'my-org', session_id: 'ses-existing' },
    ))
    expect(mocks.activate).not.toHaveBeenCalled()
    expect(mocks.list).toHaveBeenCalledWith({
      enabled: true,
      refetchInterval: 2000,
    })
  })

  it('activates once and opens the durable session when polling finds it', async () => {
    mocks.bots = [{ id: 'chief-of-staff' }]
    const view = render(<OrgBotSessionResolver />)

    await waitFor(() => expect(mocks.activate).toHaveBeenCalledTimes(1))
    mocks.bots = [{ id: 'chief-of-staff', session_id: 'ses-started' }]
    view.rerender(<OrgBotSessionResolver />)

    await waitFor(() => expect(mocks.navigateReplace).toHaveBeenCalledWith(
      'org_session',
      { org_id: 'my-org', session_id: 'ses-started' },
    ))
    expect(mocks.activate).toHaveBeenCalledTimes(1)
  })

  it('introduces the selected agent without offering a premature retry', async () => {
    mocks.bots = [{ id: 'chief-of-staff', name: 'Chief of Staff' }]
    render(<OrgBotSessionResolver />)

    await waitFor(() => expect(mocks.activate).toHaveBeenCalledTimes(1))
    expect(screen.getByRole('heading', { name: 'Meet Chief of Staff' })).toBeInTheDocument()
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
})
