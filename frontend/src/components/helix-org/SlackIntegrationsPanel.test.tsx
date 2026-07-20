import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import SlackIntegrationsPanel from './SlackIntegrationsPanel'

const openDialog = vi.fn()
const snackbarError = vi.fn()
const snackbarSuccess = vi.fn()
let admin = false
let apps: any[] = []

vi.mock('../../hooks/useAccount', () => ({ default: () => ({ admin }) }))
vi.mock('../../contexts/settingsDialog', () => ({
  useSettingsDialog: () => ({ openDialog }),
}))
vi.mock('../../hooks/useSnackbar', () => ({
  default: () => ({ error: snackbarError, success: snackbarSuccess }),
}))
vi.mock('../../services/helixOrgService', () => ({
  useListSlackWorkspaces: () => ({ data: [], isLoading: false }),
  useListSlackApps: () => ({ data: apps }),
  useStartSlackInstall: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useConnectSlackWorkspace: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useDisconnectSlackWorkspace: () => ({ mutateAsync: vi.fn() }),
}))

describe('SlackIntegrationsPanel', () => {
  beforeEach(() => {
    admin = false
    apps = []
    openDialog.mockClear()
    snackbarError.mockClear()
    snackbarSuccess.mockClear()
    window.history.replaceState({}, '', '/orgs/acme/helix-org/settings')
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

  it('labels the Slack OAuth action Connect workspace', () => {
    apps = [{ id: 'app-1', slack_client_id: 'client-1' }]

    render(<SlackIntegrationsPanel />)

    expect(screen.getByRole('button', { name: 'Connect workspace' })).toBeInTheDocument()
  })

  it('shows OAuth success feedback and removes the query parameter', async () => {
    window.history.replaceState({}, '', '/orgs/acme/helix-org/settings?slack_installed=1&view=table')

    render(<SlackIntegrationsPanel />)

    await waitFor(() => expect(snackbarSuccess).toHaveBeenCalledWith('Slack workspace connected'))
    expect(window.location.search).toBe('?view=table')
  })

  it('shows OAuth error feedback and removes the query parameter', async () => {
    window.history.replaceState({}, '', '/orgs/acme/helix-org/settings?slack_error=Try+again')

    render(<SlackIntegrationsPanel />)

    await waitFor(() => expect(snackbarError).toHaveBeenCalledWith('Try again'))
    expect(window.location.search).toBe('')
  })
})
