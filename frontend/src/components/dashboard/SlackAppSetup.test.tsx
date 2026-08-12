import { act, fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import SlackAppSetup from './SlackAppSetup'

const writeText = vi.fn((_: string) => Promise.resolve())

vi.mock('../../hooks/useAccount', () => ({
  default: () => ({ serverConfig: { server_url: 'https://helix.example' } }),
}))

describe('SlackAppSetup', () => {
  beforeEach(() => {
    writeText.mockClear()
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } })
  })

  it('opens the base Slack creation flow and copies the complete manifest', async () => {
    render(<SlackAppSetup open onClose={vi.fn()} ingressMode="rest" appName="Acme" />)

    const link = screen.getByRole('link', { name: 'Open Slack app creation' })
    expect(link).toHaveAttribute('href', 'https://api.slack.com/apps?new_app=1')
    expect(link.getAttribute('href')).not.toContain('manifest_json')

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'Copy' }))
      await Promise.resolve()
    })
    const manifest = JSON.parse(writeText.mock.calls[0][0])

    expect(manifest.features.bot_user.display_name).toBe('Acme')
    expect(manifest.features.app_home).toEqual({
      home_tab_enabled: false,
      messages_tab_enabled: true,
      messages_tab_read_only_enabled: false,
    })
    expect(manifest.oauth_config.scopes.bot.length).toBeGreaterThan(0)
    expect(manifest.oauth_config.redirect_urls).toContain('https://helix.example/api/v1/slack/oauth/callback')
    expect(manifest.settings.event_subscriptions.request_url).toBe('https://helix.example/api/v1/slack/events')
    expect(manifest.settings.event_subscriptions.bot_events.length).toBeGreaterThan(0)
  })
})
