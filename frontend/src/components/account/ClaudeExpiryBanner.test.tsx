import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import ClaudeExpiryBanner from './ClaudeExpiryBanner'

type Sub = {
  owner_type: 'user' | 'org'
  refresh_token_expires_at?: string
  credential_type?: string
}

let subscriptions: Sub[] = []

vi.mock('./ClaudeSubscriptionConnect', () => ({
  useClaudeSubscriptions: () => ({ data: subscriptions }),
  default: () => null,
}))

const inHours = (h: number) => new Date(Date.now() + h * 3600_000).toISOString()

describe('ClaudeExpiryBanner', () => {
  beforeEach(() => {
    subscriptions = []
  })

  it('stays quiet when there is no subscription', () => {
    const { container } = render(<ClaudeExpiryBanner onReconnect={() => {}} />)
    expect(container).toBeEmptyDOMElement()
  })

  it('stays quiet while the login has days left', () => {
    // A sign-in is good for ~9 days; warning on day one would be pure noise.
    subscriptions = [{ owner_type: 'user', refresh_token_expires_at: inHours(72) }]
    const { container } = render(<ClaudeExpiryBanner onReconnect={() => {}} />)
    expect(container).toBeEmptyDOMElement()
  })

  it('stays quiet for a setup token, which has no login deadline', () => {
    subscriptions = [{ owner_type: 'user', credential_type: 'setup_token' }]
    const { container } = render(<ClaudeExpiryBanner onReconnect={() => {}} />)
    expect(container).toBeEmptyDOMElement()
  })

  it('warns on the last day, before anything breaks', () => {
    subscriptions = [{ owner_type: 'user', refresh_token_expires_at: inHours(5.5) }]
    render(<ClaudeExpiryBanner onReconnect={() => {}} />)

    expect(screen.getByText('Claude sign-in expires today')).toBeInTheDocument()
    expect(screen.getByText(/Expires in 5h/)).toBeInTheDocument()
  })

  it('reports an already-dead login and why it matters', () => {
    subscriptions = [{ owner_type: 'user', refresh_token_expires_at: inHours(-2) }]
    render(<ClaudeExpiryBanner onReconnect={() => {}} />)

    expect(screen.getByText('Claude sign-in expired')).toBeInTheDocument()
    expect(screen.getByText(/agents using this subscription will fail/)).toBeInTheDocument()
  })

  it('opens the reconnect flow when the user acts on it', () => {
    const onReconnect = vi.fn()
    subscriptions = [{ owner_type: 'user', refresh_token_expires_at: inHours(3) }]
    render(<ClaudeExpiryBanner onReconnect={onReconnect} />)

    fireEvent.click(screen.getByRole('button', { name: 'Sign in' }))
    expect(onReconnect).toHaveBeenCalledTimes(1)
  })

  it('ignores an org subscription when judging the user own login', () => {
    // The banner speaks for the credential the signed-in user must renew.
    subscriptions = [{ owner_type: 'org', refresh_token_expires_at: inHours(1) }]
    const { container } = render(<ClaudeExpiryBanner onReconnect={() => {}} />)
    expect(container).toBeEmptyDOMElement()
  })
})
