import { render, screen, fireEvent } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import AccountDialog from './AccountDialog'

// A phone-width viewport. `useMediaQuery` reads matchMedia, which jsdom does
// not implement, so the queries the dialog asks are answered here.
let narrow = true
let short = false

vi.mock('@mui/material/useMediaQuery', () => ({
  default: (query: any) => (typeof query === 'string' && query.includes('max-height') ? short : narrow),
}))

// The settings pages themselves pull the whole account/services stack; the
// navigation is what this covers.
vi.mock('../../pages/Account', () => ({
  default: ({ tab }: { tab: string }) => <div data-testid="account-pane">pane:{tab}</div>,
}))

const listItems = () => screen.queryAllByRole('button').map((b) => b.textContent)

describe('AccountDialog on a phone', () => {
  beforeEach(() => {
    narrow = true
    short = false
  })

  it('opens on the section list rather than a squeezed sidebar', () => {
    render(<AccountDialog open onClose={() => {}} />)

    expect(screen.getByText('Account')).toBeInTheDocument()
    expect(screen.queryByTestId('account-pane')).not.toBeInTheDocument()
    expect(listItems()).toEqual(
      expect.arrayContaining([
        expect.stringContaining('General Settings'),
        expect.stringContaining('Git Config'),
        expect.stringContaining('Chat'),
        expect.stringContaining('API Keys'),
      ]),
    )
  })

  it('pushes to a section and comes back with the back button', () => {
    render(<AccountDialog open onClose={() => {}} />)

    fireEvent.click(screen.getByText('API Keys'))

    expect(screen.getByTestId('account-pane')).toHaveTextContent('pane:api_keys')
    // The header becomes the section, with a way back to the list.
    expect(screen.getByText('API Keys')).toBeInTheDocument()

    fireEvent.click(screen.getByLabelText('Back to account settings'))

    expect(screen.queryByTestId('account-pane')).not.toBeInTheDocument()
    expect(screen.getByText('Account')).toBeInTheDocument()
  })

  it('reports the selected section so the URL can follow it', () => {
    const onTabChange = vi.fn()
    render(<AccountDialog open onClose={() => {}} onTabChange={onTabChange} />)

    fireEvent.click(screen.getByText('Chat'))

    expect(onTabChange).toHaveBeenCalledWith('chat')
  })

  it('opens straight into a deep-linked section', () => {
    render(<AccountDialog open onClose={() => {}} initialTab="git_config" />)

    expect(screen.getByTestId('account-pane')).toHaveTextContent('pane:git_config')
    expect(screen.getByLabelText('Back to account settings')).toBeInTheDocument()
  })

  it('ignores a tab that is not a section', () => {
    render(<AccountDialog open onClose={() => {}} initialTab="not_a_section" />)

    expect(screen.queryByTestId('account-pane')).not.toBeInTheDocument()
  })
})

describe('AccountDialog on a wide screen', () => {
  beforeEach(() => {
    narrow = false
    short = false
  })

  it('keeps the sidebar and the pane side by side, with no back button', () => {
    render(<AccountDialog open onClose={() => {}} />)

    expect(screen.getByTestId('account-pane')).toHaveTextContent('pane:general')
    expect(screen.queryByLabelText('Back to account settings')).not.toBeInTheDocument()

    fireEvent.click(screen.getByText('Chat'))

    expect(screen.getByTestId('account-pane')).toHaveTextContent('pane:chat')
  })
})
