import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { ThemeProvider, createTheme } from '@mui/material/styles'
import LoginRegisterDialog from './LoginRegisterDialog'

const mockV1AuthLoginCreate = vi.fn()
const mockV1AuthRegisterCreate = vi.fn()

let mockConfigValue: { registration_enabled: boolean } = { registration_enabled: true }

vi.mock('../../hooks/useApi', () => ({
  default: () => ({
    getApiClient: () => ({
      v1AuthLoginCreate: mockV1AuthLoginCreate,
      v1AuthRegisterCreate: mockV1AuthRegisterCreate,
    }),
  }),
}))

vi.mock('../../hooks/useSnackbar', () => ({
  default: () => ({
    success: vi.fn(),
    error: vi.fn(),
    info: vi.fn(),
  }),
}))

vi.mock('../../hooks/useRouter', () => ({
  default: () => ({
    navigate: vi.fn(),
  }),
}))

vi.mock('../../services/userService', () => ({
  useGetConfig: () => ({
    data: mockConfigValue,
    isLoading: false,
  }),
}))

function renderDialog(open = true) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <LoginRegisterDialog open={open} onClose={vi.fn()} />
    </QueryClientProvider>
  )
}

function fillEmail(value: string) {
  const emailInput = document.querySelector('input[name="username"]') as HTMLInputElement
  fireEvent.change(emailInput, { target: { value } })
}

function fillPassword(value: string) {
  const passwordInput = document.querySelector('input[name="password"]') as HTMLInputElement
  fireEvent.change(passwordInput, { target: { value } })
}

function switchToRegister() {
  fireEvent.click(screen.getByText('Register here'))
}

function fillRegisterFields(email: string, name: string, password: string, confirm: string) {
  fillEmail(email)
  const nameInput = document.querySelector('input[name="name"]') as HTMLInputElement
  fireEvent.change(nameInput, { target: { value: name } })
  fillPassword(password)
  const confirmInput = document.querySelector('input[name="password-confirm"]') as HTMLInputElement
  fireEvent.change(confirmInput, { target: { value: confirm } })
}

describe('LoginRegisterDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockConfigValue = { registration_enabled: true }
  })

  describe('login mode', () => {
    it('shows error when fields are empty', async () => {
      renderDialog()
      fireEvent.click(screen.getByRole('button', { name: 'Login' }))

      await waitFor(() => {
        expect(screen.getByText('Please fill in all fields')).toBeInTheDocument()
      })
      expect(mockV1AuthLoginCreate).not.toHaveBeenCalled()
    })

    it('shows error for email without @', async () => {
      renderDialog()
      fillEmail('notanemail')
      fillPassword('password123')
      fireEvent.click(screen.getByRole('button', { name: 'Login' }))

      await waitFor(() => {
        expect(screen.getByText('Please enter a valid email address')).toBeInTheDocument()
      })
      expect(mockV1AuthLoginCreate).not.toHaveBeenCalled()
    })

    it('shows error for email without domain', async () => {
      renderDialog()
      fillEmail('user@')
      fillPassword('password123')
      fireEvent.click(screen.getByRole('button', { name: 'Login' }))

      await waitFor(() => {
        expect(screen.getByText('Please enter a valid email address')).toBeInTheDocument()
      })
      expect(mockV1AuthLoginCreate).not.toHaveBeenCalled()
    })

    it('shows error for email without TLD', async () => {
      renderDialog()
      fillEmail('user@domain')
      fillPassword('password123')
      fireEvent.click(screen.getByRole('button', { name: 'Login' }))

      await waitFor(() => {
        expect(screen.getByText('Please enter a valid email address')).toBeInTheDocument()
      })
      expect(mockV1AuthLoginCreate).not.toHaveBeenCalled()
    })

    it('accepts valid email and calls API', async () => {
      mockV1AuthLoginCreate.mockResolvedValue({})
      renderDialog()
      fillEmail('user@example.com')
      fillPassword('password123')
      fireEvent.click(screen.getByRole('button', { name: 'Login' }))

      await waitFor(() => {
        expect(mockV1AuthLoginCreate).toHaveBeenCalledWith({
          email: 'user@example.com',
          password: 'password123',
        })
      })
    })

    it('shows API error from string response', async () => {
      mockV1AuthLoginCreate.mockRejectedValue({
        response: { data: 'Invalid credentials' },
      })
      renderDialog()
      fillEmail('user@example.com')
      fillPassword('password123')
      fireEvent.click(screen.getByRole('button', { name: 'Login' }))

      await waitFor(() => {
        expect(screen.getByText('Invalid credentials')).toBeInTheDocument()
      })
    })

    it('shows API error from response.data.message', async () => {
      mockV1AuthLoginCreate.mockRejectedValue({
        response: { data: { message: 'Account locked' } },
      })
      renderDialog()
      fillEmail('user@example.com')
      fillPassword('password123')
      fireEvent.click(screen.getByRole('button', { name: 'Login' }))

      await waitFor(() => {
        expect(screen.getByText('Account locked')).toBeInTheDocument()
      })
    })

    it('shows API error from response.data.error', async () => {
      mockV1AuthLoginCreate.mockRejectedValue({
        response: { data: { error: 'Server error' } },
      })
      renderDialog()
      fillEmail('user@example.com')
      fillPassword('password123')
      fireEvent.click(screen.getByRole('button', { name: 'Login' }))

      await waitFor(() => {
        expect(screen.getByText('Server error')).toBeInTheDocument()
      })
    })

    it('shows JSON-stringified error as fallback', async () => {
      mockV1AuthLoginCreate.mockRejectedValue({
        response: { data: { code: 500 } },
      })
      renderDialog()
      fillEmail('user@example.com')
      fillPassword('password123')
      fireEvent.click(screen.getByRole('button', { name: 'Login' }))

      await waitFor(() => {
        expect(screen.getByText('{"code":500}')).toBeInTheDocument()
      })
    })

    it('shows err.message when no response data', async () => {
      mockV1AuthLoginCreate.mockRejectedValue(new Error('Network error'))
      renderDialog()
      fillEmail('user@example.com')
      fillPassword('password123')
      fireEvent.click(screen.getByRole('button', { name: 'Login' }))

      await waitFor(() => {
        expect(screen.getByText('Network error')).toBeInTheDocument()
      })
    })
  })

  describe('register mode', () => {
    it('shows error when fields are empty', async () => {
      renderDialog()
      switchToRegister()
      fireEvent.click(screen.getByRole('button', { name: 'Register' }))

      await waitFor(() => {
        expect(screen.getByText('Please fill in all fields')).toBeInTheDocument()
      })
      expect(mockV1AuthRegisterCreate).not.toHaveBeenCalled()
    })

    it('shows error for invalid email on registration', async () => {
      renderDialog()
      switchToRegister()
      fillRegisterFields('bademail', 'Test User', 'password123', 'password123')
      fireEvent.click(screen.getByRole('button', { name: 'Register' }))

      await waitFor(() => {
        expect(screen.getByText('Please enter a valid email address')).toBeInTheDocument()
      })
      expect(mockV1AuthRegisterCreate).not.toHaveBeenCalled()
    })

    it('shows error for email with spaces', async () => {
      renderDialog()
      switchToRegister()
      fillRegisterFields('user @example.com', 'Test User', 'password123', 'password123')
      fireEvent.click(screen.getByRole('button', { name: 'Register' }))

      await waitFor(() => {
        expect(screen.getByText('Please enter a valid email address')).toBeInTheDocument()
      })
      expect(mockV1AuthRegisterCreate).not.toHaveBeenCalled()
    })

    it('shows error when passwords do not match', async () => {
      renderDialog()
      switchToRegister()
      fillRegisterFields('user@example.com', 'Test User', 'password123', 'password456')
      fireEvent.click(screen.getByRole('button', { name: 'Register' }))

      await waitFor(() => {
        expect(screen.getByText('Passwords do not match')).toBeInTheDocument()
      })
      expect(mockV1AuthRegisterCreate).not.toHaveBeenCalled()
    })

    it('shows error when password is too short', async () => {
      renderDialog()
      switchToRegister()
      fillRegisterFields('user@example.com', 'Test User', 'short', 'short')
      fireEvent.click(screen.getByRole('button', { name: 'Register' }))

      await waitFor(() => {
        expect(screen.getByText('Password must be at least 8 characters long')).toBeInTheDocument()
      })
      expect(mockV1AuthRegisterCreate).not.toHaveBeenCalled()
    })

    it('accepts valid input and calls API', async () => {
      mockV1AuthRegisterCreate.mockResolvedValue({})
      renderDialog()
      switchToRegister()
      fillRegisterFields('user@example.com', 'Test User', 'password123', 'password123')
      fireEvent.click(screen.getByRole('button', { name: 'Register' }))

      await waitFor(() => {
        expect(mockV1AuthRegisterCreate).toHaveBeenCalledWith({
          email: 'user@example.com',
          full_name: 'Test User',
          password: 'password123',
          password_confirm: 'password123',
        })
      })
    })

    it('shows API error from string response on registration', async () => {
      mockV1AuthRegisterCreate.mockRejectedValue({
        response: { data: 'Email already taken' },
      })
      renderDialog()
      switchToRegister()
      fillRegisterFields('user@example.com', 'Test User', 'password123', 'password123')
      fireEvent.click(screen.getByRole('button', { name: 'Register' }))

      await waitFor(() => {
        expect(screen.getByText('Email already taken')).toBeInTheDocument()
      })
    })
  })

  describe('registration disabled', () => {
    it('shows disabled message and prevents submission when registration is disabled', async () => {
      mockConfigValue = { registration_enabled: false }
      renderDialog()
      switchToRegister()

      await waitFor(() => {
        expect(screen.getByText('New account registrations are disabled. Please contact your server administrator.')).toBeInTheDocument()
      })

      const registerButton = screen.getByRole('button', { name: 'Register' })
      expect(registerButton).toBeDisabled()
    })
  })
})

// jsdom's getComputedStyle does not resolve emotion's descendant rules
// (`.css-xxx .MuiOutlinedInput-input`), so read the matching cascade rule
// directly — it is exactly what the browser applies to the input.
function resolvedInputColor(input: HTMLInputElement): string {
  const root = input.closest('.MuiFormControl-root') as HTMLElement
  const cssClass = [...root.classList].find((c) => c.startsWith('css-'))
  const colors: string[] = []
  for (const sheet of document.styleSheets) {
    let rules: CSSRuleList
    try {
      rules = (sheet as CSSStyleSheet).cssRules
    } catch {
      continue
    }
    for (const rule of Array.from(rules)) {
      const styleRule = rule as CSSStyleRule
      if (
        styleRule.selectorText?.includes(cssClass) &&
        styleRule.selectorText.includes('.MuiOutlinedInput-input')
      ) {
        colors.push(styleRule.style.color)
      }
    }
  }
  expect(colors.length).toBeGreaterThan(0)
  return colors[colors.length - 1]
}

// Regression: the shared input styling was hardcoded to dark-theme colors
// while the dialog surface follows the app theme, so in light mode typed
// text (near-white #F1F1F1) was invisible on the near-white dialog — worst
// on the register form's Confirm Password.
describe('LoginRegisterDialog — password field theming', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  function renderStyledDialog(mode: 'light' | 'dark') {
    render(
      <ThemeProvider theme={createTheme({ palette: { mode } })}>
        <LoginRegisterDialog open onClose={() => {}} />
      </ThemeProvider>
    )
    switchToRegister()
    return {
      password: document.querySelector('input[name="password"]') as HTMLInputElement,
      confirm: document.querySelector('input[name="password-confirm"]') as HTMLInputElement,
    }
  }

  it('keeps both password fields masked and readable in light mode', () => {
    const { password, confirm } = renderStyledDialog('light')

    expect(password.type).toBe('password')
    expect(confirm.type).toBe('password')

    // Light token (lightText = #000000), not the legacy hardcoded #F1F1F1.
    expect(resolvedInputColor(password)).toBe('#000000')
    expect(resolvedInputColor(confirm)).toBe('#000000')
  })

  it('keeps both password fields masked and readable in dark mode', () => {
    const { password, confirm } = renderStyledDialog('dark')

    expect(password.type).toBe('password')
    expect(confirm.type).toBe('password')

    // Dark token (darkText = #e0e0e0).
    expect(resolvedInputColor(password)).toBe('#e0e0e0')
    expect(resolvedInputColor(confirm)).toBe('#e0e0e0')
  })
})
