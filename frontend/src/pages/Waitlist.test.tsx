import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import Waitlist from './Waitlist'

const mockNavigateReplace = vi.fn()
const mockOnLogout = vi.fn()

type AccountValue = {
  initialized: boolean
  user: { email?: string; name?: string; waitlisted?: boolean } | null
  onLogout: () => void
}

let mockAccountValue: AccountValue = {
  initialized: true,
  user: null,
  onLogout: mockOnLogout,
}

vi.mock('../hooks/useAccount', () => ({
  default: () => mockAccountValue,
}))

vi.mock('../hooks/useRouter', () => ({
  default: () => ({ navigateReplace: mockNavigateReplace }),
}))

describe('Waitlist — auto-redirect off the page', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('redirects to orgs when waitlisted flips false (admin approval / invitation consumed)', () => {
    // This is the regression case: previously, a user whose state had
    // already loaded with waitlisted=false but who refreshed /waitlist
    // would stay stuck on the page. The page-level effect must push them
    // off.
    mockAccountValue = {
      initialized: true,
      user: { email: 'u@example.com', waitlisted: false },
      onLogout: mockOnLogout,
    }
    render(<Waitlist />)
    expect(mockNavigateReplace).toHaveBeenCalledWith('orgs')
  })

  it('does NOT redirect while account is still initializing', () => {
    // We must not bounce a user before we know their waitlist state,
    // otherwise the global "push onto /waitlist" redirect chain would
    // race with this "pull off" effect and cause a redirect loop.
    mockAccountValue = {
      initialized: false,
      user: null,
      onLogout: mockOnLogout,
    }
    render(<Waitlist />)
    expect(mockNavigateReplace).not.toHaveBeenCalled()
  })

  it('does NOT redirect when there is no user', () => {
    // No user → either logged out or session hydrating. The login flow
    // owns that redirect, not us.
    mockAccountValue = {
      initialized: true,
      user: null,
      onLogout: mockOnLogout,
    }
    render(<Waitlist />)
    expect(mockNavigateReplace).not.toHaveBeenCalled()
  })

  it('does NOT redirect when the user is still waitlisted', () => {
    // Happy path of the original purpose: the page stays put and renders.
    mockAccountValue = {
      initialized: true,
      user: { email: 'still@waitlisted.com', waitlisted: true },
      onLogout: mockOnLogout,
    }
    render(<Waitlist />)
    expect(mockNavigateReplace).not.toHaveBeenCalled()
    expect(screen.getByText("You're on the waitlist!")).toBeInTheDocument()
  })
})
