import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import GlobalNotifications from './GlobalNotifications'

const mocks = vi.hoisted(() => ({
  acknowledge: vi.fn(),
  fireNotification: vi.fn(),
  navigate: vi.fn(),
  browserNotificationsEnabled: false,
}))

const event = {
  id: 'event-1',
  user_id: 'user-1',
  organization_id: 'org-1',
  project_id: '',
  spec_task_id: '',
  event_type: 'org_message' as const,
  title: 'Message from Agent Two',
  description: 'Please review this.',
  created_at: new Date().toISOString(),
  acknowledged_at: undefined as string | undefined,
  metadata: { bot_id: 'bot-two' },
}

vi.mock('react-router5', () => ({
  useRouter: () => ({
    buildPath: (_name: string, params: Record<string, string>) => `/orgs/${params.org_id}/chart?bot_id=${params.bot_id}`,
    navigate: mocks.navigate,
  }),
}))
vi.mock('../../hooks/useAccount', () => ({
  default: () => ({
    orgNavigate: vi.fn(),
    organizationTools: { organizations: [{ id: 'org-1', name: 'acme', display_name: 'Acme' }] },
  }),
}))
vi.mock('../../hooks/useApi', () => ({ default: () => ({ getApiClient: vi.fn() }) }))
vi.mock('../../hooks/useLightTheme', () => ({
  default: () => ({
    isLight: true,
    textColor: '#111',
    textColorFaded: '#666',
    panelColor: '#fff',
    border: '1px solid #ddd',
  }),
}))
vi.mock('../../hooks/useNavigationHistory', () => ({ useNavigationHistory: () => [] }))
vi.mock('../../hooks/useAttentionEvents', () => ({
  useAttentionEvents: () => ({
    events: [event],
    newEvents: [event],
    acknowledge: mocks.acknowledge,
    dismiss: vi.fn(),
    snooze: vi.fn(),
    dismissAll: vi.fn(),
  }),
}))
vi.mock('../../hooks/useBrowserNotifications', () => ({
  useBrowserNotifications: () => ({
    shouldPrompt: false,
    isEnabled: mocks.browserNotificationsEnabled,
    disabledByUser: false,
    requestPermission: vi.fn(),
    setOptOut: vi.fn(),
    fireNotification: mocks.fireNotification,
  }),
}))

describe('org message notifications', () => {
  beforeEach(() => {
    mocks.acknowledge.mockReset()
    mocks.fireNotification.mockReset()
    mocks.navigate.mockReset()
    mocks.browserNotificationsEnabled = false
    event.acknowledged_at = undefined
  })

  it('links to the agent chat and acknowledges before navigating', () => {
    render(<GlobalNotifications />)
    fireEvent.click(screen.getAllByRole('button')[0])

    expect(screen.queryByText('Respond')).not.toBeInTheDocument()
    const link = screen.getByRole('link', { name: 'Continue in agent chat' })
    expect(link).toHaveAttribute('href', '/orgs/acme/chart?bot_id=bot-two')
    fireEvent.click(link)

    expect(mocks.acknowledge).toHaveBeenCalledWith('event-1')
    expect(mocks.navigate).toHaveBeenCalledWith('helix_org_chart', {
      org_id: 'acme',
      bot_id: 'bot-two',
    })
  })

  it('hides the bell badge for read notifications while retaining the panel total', () => {
    event.acknowledged_at = new Date().toISOString()
    render(<GlobalNotifications />)

    const bellButton = screen.getAllByRole('button')[0]
    expect(within(bellButton).queryByText('1')).not.toBeInTheDocument()

    fireEvent.click(bellButton)
    expect(screen.getByText('1')).toBeInTheDocument()
  })

  it('opens the same agent chat from a browser notification', async () => {
    mocks.browserNotificationsEnabled = true
    render(<GlobalNotifications />)

    await waitFor(() => expect(mocks.fireNotification).toHaveBeenCalledOnce())
    mocks.fireNotification.mock.calls[0][3]()

    expect(mocks.acknowledge).toHaveBeenCalledWith('event-1')
    expect(mocks.navigate).toHaveBeenCalledWith('helix_org_chart', {
      org_id: 'acme',
      bot_id: 'bot-two',
    })
  })
})
