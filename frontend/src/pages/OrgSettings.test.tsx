import { act, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import OrgSettings from './OrgSettings'

const mocks = vi.hoisted(() => ({
  navigate: vi.fn(),
  updateOrganization: vi.fn(),
  organization: {
    id: 'org-1',
    name: 'acme',
    display_name: 'Acme',
    auto_join_domain: 'acme.com',
    memberships: [{ user_id: 'user-1', role: 'owner' }],
  },
}))

vi.mock('../hooks/useRouter', () => ({
  default: () => ({ navigate: mocks.navigate }),
}))
vi.mock('../hooks/useSnackbar', () => ({
  default: () => ({ success: vi.fn(), error: vi.fn() }),
}))
vi.mock('../hooks/useAccount', () => ({
  default: () => ({
    user: { id: 'user-1' },
    isOrgAdmin: true,
    organizationTools: {
      organization: mocks.organization,
      organizations: [],
      updateOrganization: mocks.updateOrganization,
      deleteOrganization: vi.fn(),
    },
  }),
}))
vi.mock('../components/system/Page', () => ({
  default: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
}))
vi.mock('../components/common/CopyButton', () => ({ default: () => null }))
vi.mock('../components/helix-org/WorkerRuntimePanel', () => ({ default: () => null }))
vi.mock('./HelixOrgSettings', () => ({ default: () => null }))

describe('OrgSettings autosave', () => {
  beforeEach(() => {
    mocks.navigate.mockReset()
    mocks.updateOrganization.mockReset()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('saves valid changed fields on blur and shows confirmation', async () => {
    vi.useFakeTimers()
    let resolveUpdate!: (updated: boolean) => void
    mocks.updateOrganization.mockImplementation(() => new Promise((resolve) => {
      resolveUpdate = resolve
    }))
    render(<OrgSettings />)

    fireEvent.change(screen.getByLabelText(/Name/), { target: { value: 'Acme Corp' } })
    fireEvent.blur(screen.getByLabelText(/Name/))

    expect(screen.getByText('Saving organization settings...')).toBeInTheDocument()
    expect(mocks.updateOrganization).toHaveBeenCalledWith(
      'org-1',
      expect.objectContaining({ display_name: 'Acme Corp' }),
    )
    await act(async () => resolveUpdate(true))
    expect(screen.getByText('Organization settings saved')).toBeInTheDocument()
    act(() => vi.advanceTimersByTime(2999))
    expect(screen.getByText('Organization settings saved')).toBeInTheDocument()
    act(() => vi.advanceTimersByTime(1))
    expect(screen.queryByText('Organization settings saved')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Update Organization' })).not.toBeInTheDocument()
  })

  it('shows failure and does not navigate when the update returns false', async () => {
    vi.useFakeTimers()
    let resolveUpdate!: (updated: boolean) => void
    mocks.updateOrganization.mockImplementation(() => new Promise((resolve) => {
      resolveUpdate = resolve
    }))
    render(<OrgSettings />)

    fireEvent.change(screen.getByLabelText(/Slug/), { target: { value: 'new-acme' } })
    fireEvent.blur(screen.getByLabelText(/Slug/))

    await act(async () => resolveUpdate(false))
    expect(screen.getByText('Failed to save organization settings')).toBeInTheDocument()
    act(() => vi.advanceTimersByTime(3000))
    expect(screen.getByText('Failed to save organization settings')).toBeInTheDocument()
    expect(mocks.navigate).not.toHaveBeenCalled()
  })

  it('does not save invalid values', async () => {
    render(<OrgSettings />)

    fireEvent.change(screen.getByLabelText(/Auto-Join Domain/), { target: { value: '@acme.com' } })
    fireEvent.blur(screen.getByLabelText(/Auto-Join Domain/))

    expect(await screen.findByText("Domain should not start with @, use 'example.com' not '@example.com'")).toBeInTheDocument()
    expect(mocks.updateOrganization).not.toHaveBeenCalled()
  })
})
