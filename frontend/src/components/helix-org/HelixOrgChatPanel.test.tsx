import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { chatBotStorageKey, focusChatBot } from './chatBotFocus'
import HelixOrgChatPanel from './HelixOrgChatPanel'

const mocks = vi.hoisted(() => ({
  router: {
    params: { org_id: 'acme' } as Record<string, string>,
    navigate: vi.fn(),
  },
  setCurrentSessionId: vi.fn(),
}))

const bots = [
  { id: 'bot-one', name: 'Agent One', kind: 'bot', agent_status: 'running' },
  { id: 'bot-two', name: 'Agent Two', kind: 'bot', agent_status: 'running' },
]

vi.mock('../../hooks/useRouter', () => ({ default: () => mocks.router }))
vi.mock('../../hooks/useApi', () => ({ default: () => ({ getApiClient: vi.fn() }) }))
vi.mock('../../hooks/useSnackbar', () => ({
  default: () => ({ success: vi.fn(), error: vi.fn() }),
}))
vi.mock('../../hooks/useLightTheme', () => ({
  default: () => ({ isLight: true }),
}))
vi.mock('../../contexts/streaming', () => ({
  useStreaming: () => ({ setCurrentSessionId: mocks.setCurrentSessionId }),
}))
vi.mock('../../services/helixOrgService', () => ({
  useListHelixOrgBots: () => ({ data: bots }),
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

describe('HelixOrgChatPanel query selection', () => {
  beforeEach(() => {
    localStorage.clear()
    mocks.router.params = { org_id: 'acme' }
    mocks.setCurrentSessionId.mockReset()
  })

  it('prioritizes bot_id and switches back to chat when the query changes', async () => {
    const view = render(<HelixOrgChatPanel />)
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
})
