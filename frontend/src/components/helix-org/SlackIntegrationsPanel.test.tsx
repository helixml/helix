import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import SlackIntegrationsPanel from './SlackIntegrationsPanel'

const openDialog = vi.fn()
let admin = false

vi.mock('../../hooks/useAccount', () => ({ default: () => ({ admin }) }))
vi.mock('../../contexts/settingsDialog', () => ({
  useSettingsDialog: () => ({ openDialog }),
}))
vi.mock('../../hooks/useSnackbar', () => ({
  default: () => ({ error: vi.fn(), success: vi.fn() }),
}))
vi.mock('../../services/helixOrgService', () => ({
  useListSlackWorkspaces: () => ({ data: [], isLoading: false }),
  useListSlackApps: () => ({ data: [] }),
  useStartSlackInstall: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useConnectSlackWorkspace: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useDisconnectSlackWorkspace: () => ({ mutateAsync: vi.fn() }),
}))

describe('SlackIntegrationsPanel', () => {
  beforeEach(() => {
    admin = false
    openDialog.mockClear()
  })

  it('shows admins a link to configure the Slack app', () => {
    admin = true
    render(<SlackIntegrationsPanel />)

    fireEvent.click(screen.getByRole('button', { name: 'Configure Slack app' }))

    expect(openDialog).toHaveBeenCalledWith('admin', { tab: 'service_connections' })
  })

  it('keeps the empty state text-only for non-admins', () => {
    render(<SlackIntegrationsPanel />)

    expect(screen.getByText('No Slack app has been configured by an administrator yet.')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Configure Slack app' })).not.toBeInTheDocument()
  })
})
