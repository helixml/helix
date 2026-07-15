import { describe, it, expect } from 'vitest'
import { buildManifest, BOT_EVENTS } from './slackManifest'

const REDIRECT = 'https://helix.example/api/v1/slack/oauth/callback'
const EVENTS = 'https://helix.example/api/v1/slack/events'

describe('buildManifest', () => {
  // The bug that broke REST end-to-end: Slack rejects bot_events with
  // neither a Request URL nor Socket Mode ("Subscription requires a Request
  // URL"). REST must declare the Events Request URL.
  it('REST declares the Events Request URL and disables Socket Mode', () => {
    const m = JSON.parse(buildManifest('rest', REDIRECT, EVENTS, 'Acme'))
    expect(m.settings.event_subscriptions.request_url).toBe(EVENTS)
    expect(m.settings.socket_mode_enabled).toBe(false)
    expect(m.settings.event_subscriptions.bot_events.length).toBeGreaterThan(0)
  })

  // Socket Mode is the other valid shape: no Request URL, socket enabled.
  it('Socket Mode enables the socket and omits the Request URL', () => {
    const m = JSON.parse(buildManifest('socket', REDIRECT, EVENTS, 'Acme'))
    expect(m.settings.socket_mode_enabled).toBe(true)
    expect(m.settings.event_subscriptions.request_url).toBeUndefined()
    expect(m.settings.event_subscriptions.bot_events.length).toBeGreaterThan(0)
  })

  it('always carries the OAuth redirect URL and bot scopes', () => {
    const m = JSON.parse(buildManifest('rest', REDIRECT, EVENTS))
    expect(m.oauth_config.redirect_urls).toContain(REDIRECT)
    expect(m.oauth_config.scopes.bot).toContain('chat:write')
    expect(m.oauth_config.scopes.bot).toContain('im:write')
    expect(m.oauth_config.scopes.bot).toContain('users:read')
    expect(m.oauth_config.scopes.bot).toContain('users:read.email')
  })

  // Every subscribed message.* event must have its matching *:history scope
  // (Slack rejects an event whose scope is missing).
  it('derives bot_events only from requested *:history scopes', () => {
    const m = JSON.parse(buildManifest('rest', REDIRECT, EVENTS))
    const scopes: string[] = m.oauth_config.scopes.bot
    for (const ev of BOT_EVENTS as string[]) {
      if (ev === 'app_mention') {
        expect(scopes).toContain('app_mentions:read')
      } else {
        // message.channels -> channels:history, etc.
        expect(scopes).toContain(ev.replace('message.', '') + ':history')
      }
    }
  })

  it('falls back to the Helix name and clamps to Slack’s 35-char cap', () => {
    expect(JSON.parse(buildManifest('rest', REDIRECT, EVENTS)).display_information.name).toBe('Helix')
    const long = 'x'.repeat(50)
    expect(JSON.parse(buildManifest('rest', REDIRECT, EVENTS, long)).display_information.name.length).toBe(35)
  })
})
