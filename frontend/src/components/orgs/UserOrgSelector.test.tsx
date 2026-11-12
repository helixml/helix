import { fireEvent, render, screen, within } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import UserOrgSelector from './UserOrgSelector'

const mockOnLogout = vi.fn()

vi.mock('../../hooks/useAccount', () => ({
  default: () => ({
    initialized: true,
    user: { name: 'Test User', email: 'test@example.com' },
    admin: false,
    onLogin: vi.fn(),
    onLogout: mockOnLogout,
    orgNavigate: vi.fn(),
    setMobileMenuOpen: vi.fn(),
    organizationTools: {
      organization: null,
      organizations: [],
      orgID: null,
    },
  }),
}))

vi.mock('../../hooks/useRouter', () => ({
  default: () => ({
    name: 'orgs',
    params: {},
    meta: {},
    navigate: vi.fn(),
  }),
}))

vi.mock('../../hooks/useLightTheme', () => ({
  default: () => ({
    isLight: false,
    backgroundColor: '#000',
    panelColor: '#111',
    border: '1px solid #222',
    textColor: '#fff',
    textColorFaded: '#aaa',
  }),
}))

vi.mock('../../hooks/useThemeConfig', () => ({
  default: () => ({ drawerWidth: 320, smallDrawerWidth: 240 }),
}))

vi.mock('../../hooks/useIsBigScreen', () => ({ default: () => true }))
vi.mock('../../hooks/useIsPhone', () => ({ default: () => false }))
vi.mock('../../services/userService', () => ({ useGetConfig: () => ({ data: {} }) }))
vi.mock('../../contexts/settingsDialog', () => ({
  useSettingsDialog: () => ({ activeDialog: null, openDialog: vi.fn() }),
}))
vi.mock('../system/TokenUsageDisplay', () => ({ default: () => null }))
vi.mock('../system/LowCreditsDisplay', () => ({ default: () => null }))
vi.mock('../subscription/SubscriptionStatusBanner', () => ({ default: () => null }))
vi.mock('./LoginRegisterDialog', () => ({ default: () => null }))

describe('UserOrgSelector logout', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('asks for confirmation before signing out', () => {
    render(<UserOrgSelector sidebarVisible />)

    fireEvent.click(screen.getByRole('button', { name: 'Log out' }))

    expect(mockOnLogout).not.toHaveBeenCalled()
    const dialog = screen.getByRole('dialog')
    fireEvent.click(within(dialog).getByRole('button', { name: 'Sign out' }))
    expect(mockOnLogout).toHaveBeenCalledOnce()
  })
})
