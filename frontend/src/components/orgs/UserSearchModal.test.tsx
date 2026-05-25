import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import UserSearchModal from './UserSearchModal'

const mockSearchUsers = vi.fn()
const mockV1OrgsUsersLookup = vi.fn()
const mockOnAddMember = vi.fn()
const mockOnClose = vi.fn()

// The production useApi wraps getApiClient in useCallback so it's stable
// across renders. Match that — otherwise the lookup useEffect re-runs every
// render and we loop forever.
const stableApiClient = { v1OrganizationsUsersLookupDetail: mockV1OrgsUsersLookup }
const stableGetApiClient = () => stableApiClient
vi.mock('../../hooks/useApi', () => ({
  default: () => ({
    getApiClient: stableGetApiClient,
  }),
}))

vi.mock('../../hooks/useOrganizations', () => ({
  default: () => ({
    searchUsers: mockSearchUsers,
  }),
}))

vi.mock('../../hooks/useAccount', () => ({
  default: () => ({
    organizationTools: { organization: { id: 'org_test' } },
  }),
}))

// Bypass the 300ms debounce so tests don't have to await fake timers — the
// underlying state machine is what we're verifying, not the debounce.
vi.mock('../../hooks/useDebounce', () => ({
  default: <T,>(value: T) => value,
}))

function renderModal() {
  return render(
    <UserSearchModal
      open
      onClose={mockOnClose}
      onAddMember={mockOnAddMember}
    />
  )
}

function typeEmail(value: string) {
  const input = document.querySelector('input[id="search"]') as HTMLInputElement
  fireEvent.change(input, { target: { value } })
}

describe('UserSearchModal — invite-by-email CTA states', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    // searchUsers returns no matches by default so the email-lookup
    // branch always runs.
    mockSearchUsers.mockResolvedValue({ users: [] })
  })

  it('shows "Send invitation" (enabled) when the email has no Helix account', async () => {
    mockV1OrgsUsersLookup.mockResolvedValue({
      data: { email: 'new@example.com', exists: false, is_member: false, is_invited: false },
    })
    renderModal()
    typeEmail('new@example.com')

    await waitFor(() => {
      expect(screen.getByRole('button', { name: 'Send invitation' })).toBeInTheDocument()
    })
    expect(screen.getByRole('button', { name: 'Send invitation' })).not.toBeDisabled()
    // Helper text explains the consequence — they'll get an email.
    expect(
      screen.getByText(/We'll email them an invitation/i),
    ).toBeInTheDocument()
  })

  it('shows "Add to organization" (enabled) when the email belongs to a Helix user outside the org', async () => {
    mockV1OrgsUsersLookup.mockResolvedValue({
      data: { email: 'someone@example.com', exists: true, is_member: false, is_invited: false },
    })
    renderModal()
    typeEmail('someone@example.com')

    await waitFor(() => {
      expect(screen.getByRole('button', { name: 'Add to organization' })).toBeInTheDocument()
    })
    expect(screen.getByRole('button', { name: 'Add to organization' })).not.toBeDisabled()
  })

  it('shows "Already a member" (disabled) when the email is already in the org', async () => {
    mockV1OrgsUsersLookup.mockResolvedValue({
      data: { email: 'inside@example.com', exists: true, is_member: true, is_invited: false },
    })
    renderModal()
    typeEmail('inside@example.com')

    await waitFor(() => {
      expect(screen.getByRole('button', { name: 'Already a member' })).toBeInTheDocument()
    })
    // CTA must be disabled — admins shouldn't be able to re-add an existing
    // member (the action is a no-op and would surface a confusing 4xx).
    expect(screen.getByRole('button', { name: 'Already a member' })).toBeDisabled()
  })

  it('shows "Invitation sent" (disabled) when a pending invitation already exists', async () => {
    // is_invited takes precedence over not-exists — we don't want to spam
    // a second invitation to the same address.
    mockV1OrgsUsersLookup.mockResolvedValue({
      data: { email: 'pending@example.com', exists: false, is_member: false, is_invited: true },
    })
    renderModal()
    typeEmail('pending@example.com')

    await waitFor(() => {
      expect(screen.getByRole('button', { name: 'Invitation sent' })).toBeInTheDocument()
    })
    expect(screen.getByRole('button', { name: 'Invitation sent' })).toBeDisabled()
  })

  it('does not surface a CTA for partial / non-email input', async () => {
    renderModal()
    typeEmail('not-an-email')

    // Wait long enough for any background work to settle, then assert
    // that we never offered to invite a non-email.
    await waitFor(() => {
      expect(mockSearchUsers).toHaveBeenCalled()
    })
    expect(screen.queryByRole('button', { name: 'Send invitation' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Add to organization' })).not.toBeInTheDocument()
    // Email lookup should NOT have been called for a non-email.
    expect(mockV1OrgsUsersLookup).not.toHaveBeenCalled()
  })

  it('falls back to "Send invitation" semantics when the lookup endpoint errors', async () => {
    // Network failure on lookup — we soft-fail to {exists: false} so the
    // admin can still invite; the backend re-validates on submit.
    mockV1OrgsUsersLookup.mockRejectedValue(new Error('network'))
    renderModal()
    typeEmail('flaky@example.com')

    await waitFor(() => {
      expect(screen.getByRole('button', { name: 'Send invitation' })).toBeInTheDocument()
    })
  })

  it('passes the typed email through to onAddMember when "Send invitation" is clicked', async () => {
    mockV1OrgsUsersLookup.mockResolvedValue({
      data: { email: 'invite-me@example.com', exists: false, is_member: false, is_invited: false },
    })
    renderModal()
    typeEmail('invite-me@example.com')

    await waitFor(() => {
      expect(screen.getByRole('button', { name: 'Send invitation' })).not.toBeDisabled()
    })
    fireEvent.click(screen.getByRole('button', { name: 'Send invitation' }))

    // The handler treats the email itself as the user reference — the
    // backend resolves it to either a user-id (existing account) or
    // creates an invitation row.
    expect(mockOnAddMember).toHaveBeenCalledWith('invite-me@example.com')
    expect(mockOnClose).toHaveBeenCalled()
  })
})
