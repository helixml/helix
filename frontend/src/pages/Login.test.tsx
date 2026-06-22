import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import Login from './Login'

const mockV1AuthLoginCreate = vi.fn()
const mockV1AuthRegisterCreate = vi.fn()
const mockV1InvitationsInfoDetail = vi.fn()

let mockConfigValue: { registration_enabled?: boolean; auth_provider?: string; edition?: string } = {
  registration_enabled: true,
  auth_provider: 'regular',
}

let mockAccountValue: { initialized: boolean; user: unknown; onLogin: () => void } = {
  initialized: true,
  user: null,
  onLogin: vi.fn(),
}

vi.mock('../hooks/useApi', () => ({
  default: () => ({
    getApiClient: () => ({
      v1AuthLoginCreate: mockV1AuthLoginCreate,
      v1AuthRegisterCreate: mockV1AuthRegisterCreate,
      v1InvitationsInfoDetail: mockV1InvitationsInfoDetail,
    }),
  }),
}))

vi.mock('../hooks/useAccount', () => ({
  default: () => mockAccountValue,
}))

vi.mock('../hooks/useSnackbar', () => ({
  default: () => ({ success: vi.fn(), error: vi.fn(), info: vi.fn() }),
}))

vi.mock('../hooks/useRouter', () => ({
  default: () => ({ navigate: vi.fn() }),
}))

vi.mock('../services/userService', () => ({
  useGetConfig: () => ({ data: mockConfigValue, isLoading: false }),
}))

function renderLogin() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <Login />
    </QueryClientProvider>
  )
}

// The Login component reads window.location.search synchronously on mount,
// so each test needs to set the URL before render. jsdom doesn't let us
// reassign window.location, but it accepts replaceState which updates the
// URL object's search.
function setURL(search: string) {
  window.history.replaceState({}, '', `/login${search}`)
}

describe('Login — invitation branch', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockConfigValue = { registration_enabled: true, auth_provider: 'regular' }
    mockAccountValue = { initialized: true, user: null, onLogin: vi.fn() }
    setURL('')
  })

  it('does not call the invitation endpoint when no ?invitation= param is present', () => {
    renderLogin()
    expect(mockV1InvitationsInfoDetail).not.toHaveBeenCalled()
    // Default mode is login — register-only field "Confirm Password" must
    // NOT be on screen.
    expect(screen.queryByLabelText('Confirm Password')).not.toBeInTheDocument()
  })

  it('fetches invitation, switches to register mode, prefills + locks email', async () => {
    mockV1InvitationsInfoDetail.mockResolvedValue({
      data: {
        id: 'oin_abc',
        email: 'invited@example.com',
        organization_name: 'acme',
        organization_display_name: 'Acme Inc.',
      },
    })
    setURL('?invitation=oin_abc')
    renderLogin()

    await waitFor(() => {
      expect(mockV1InvitationsInfoDetail).toHaveBeenCalledWith('oin_abc')
    })

    // Register fields must appear (Confirm Password is register-only).
    await waitFor(() => {
      expect(document.querySelector('input[name="password-confirm"]')).toBeInTheDocument()
    })

    // Email prefilled with the invitation email AND the input is disabled
    // so the user can't register a different address than was invited.
    const emailInput = document.querySelector('input[name="username"]') as HTMLInputElement
    expect(emailInput.value).toBe('invited@example.com')
    expect(emailInput).toBeDisabled()

    // Org display name surfaced both in the page subtitle and the success
    // banner — at least one match is enough.
    await waitFor(() => {
      expect(screen.getAllByText(/Acme Inc\./).length).toBeGreaterThan(0)
    })

    // Locked-field helper text explains why the user can't change the email.
    expect(screen.getByText('Email is locked to the address the invitation was sent to.')).toBeInTheDocument()

    // Primary CTA is the register button — "Create account", not "Sign in".
    expect(screen.getByRole('button', { name: 'Create account' })).toBeInTheDocument()
  })

  it('falls back to organization_name when display_name is empty', async () => {
    mockV1InvitationsInfoDetail.mockResolvedValue({
      data: {
        id: 'oin_xyz',
        email: 'invited@example.com',
        organization_name: 'acme',
      },
    })
    setURL('?invitation=oin_xyz')
    renderLogin()

    await waitFor(() => {
      expect(screen.getAllByText(/acme/).length).toBeGreaterThan(0)
    })
  })

  it('shows a soft warning (not a hard error) when the invitation is invalid', async () => {
    mockV1InvitationsInfoDetail.mockRejectedValue({
      response: { data: { error: 'Invitation has been revoked' } },
    })
    setURL('?invitation=oin_bad')
    renderLogin()

    await waitFor(() => {
      expect(screen.getByText('Invitation has been revoked')).toBeInTheDocument()
    })

    // Crucially: mode is still register (so the user can self-serve), the
    // email field is NOT locked, and the register form is fully usable.
    const emailInput = document.querySelector('input[name="username"]') as HTMLInputElement
    expect(emailInput).not.toBeDisabled()
    expect(document.querySelector('input[name="password-confirm"]')).toBeInTheDocument()
  })

  it('uses the fallback message when the API gives no error detail', async () => {
    mockV1InvitationsInfoDetail.mockRejectedValue(new Error(''))
    setURL('?invitation=oin_404')
    renderLogin()

    await waitFor(() => {
      expect(screen.getByText('This invitation link is invalid or has expired.')).toBeInTheDocument()
    })
  })

  it('submits register with the prefilled email when the invitation flow completes', async () => {
    mockV1InvitationsInfoDetail.mockResolvedValue({
      data: { id: 'oin_ok', email: 'invited@example.com', organization_name: 'acme' },
    })
    mockV1AuthRegisterCreate.mockResolvedValue({})
    setURL('?invitation=oin_ok')
    renderLogin()

    await waitFor(() => {
      const emailInput = document.querySelector('input[name="username"]') as HTMLInputElement
      expect(emailInput.value).toBe('invited@example.com')
    })

    const nameInput = document.querySelector('input[name="name"]') as HTMLInputElement
    fireEvent.change(nameInput, { target: { value: 'Test User' } })
    const passwordInput = document.querySelector('input[name="password"]') as HTMLInputElement
    fireEvent.change(passwordInput, { target: { value: 'password123' } })
    const confirmInput = document.querySelector('input[name="password-confirm"]') as HTMLInputElement
    fireEvent.change(confirmInput, { target: { value: 'password123' } })

    fireEvent.click(screen.getByRole('button', { name: 'Create account' }))

    await waitFor(() => {
      expect(mockV1AuthRegisterCreate).toHaveBeenCalledWith({
        email: 'invited@example.com',
        full_name: 'Test User',
        password: 'password123',
        password_confirm: 'password123',
      })
    })
  })
})
