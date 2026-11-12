import { describe, expect, it } from 'vitest'
import { callbackUrlOriginDiffers, isOAuthPopupMessage } from './oauthPopupMessage'

const ORIGIN = 'https://app.example.test'

function messageEvent(data: unknown, origin: string, source: unknown): MessageEvent {
  return new MessageEvent('message', { data, origin, source: source as MessageEventSource })
}

describe('isOAuthPopupMessage', () => {
  const popup = { name: 'oauth-popup' } as unknown as Window
  const otherWindow = { name: 'other' } as unknown as Window

  it('accepts a same-origin success message from the popup', () => {
    expect(isOAuthPopupMessage(messageEvent({ type: 'oauth-success' }, ORIGIN, popup), popup, 'oauth-success', ORIGIN)).toBe(true)
  })

  it('ignores messages from a foreign origin', () => {
    expect(isOAuthPopupMessage(messageEvent({ type: 'oauth-success' }, 'https://evil.example', popup), popup, 'oauth-success', ORIGIN)).toBe(false)
    expect(isOAuthPopupMessage(messageEvent({ type: 'oauth-success' }, 'null', popup), popup, 'oauth-success', ORIGIN)).toBe(false)
  })

  it('ignores same-origin messages that do not come from the popup', () => {
    expect(isOAuthPopupMessage(messageEvent({ type: 'oauth-success' }, ORIGIN, otherWindow), popup, 'oauth-success', ORIGIN)).toBe(false)
    expect(isOAuthPopupMessage(messageEvent({ type: 'oauth-success' }, ORIGIN, null), popup, 'oauth-success', ORIGIN)).toBe(false)
    expect(isOAuthPopupMessage(messageEvent({ type: 'oauth-success' }, ORIGIN, popup), null, 'oauth-success', ORIGIN)).toBe(false)
  })

  it('ignores other message types and malformed data', () => {
    expect(isOAuthPopupMessage(messageEvent({ type: 'oauth-failure' }, ORIGIN, popup), popup, 'oauth-success', ORIGIN)).toBe(false)
    expect(isOAuthPopupMessage(messageEvent('oauth-success', ORIGIN, popup), popup, 'oauth-success', ORIGIN)).toBe(false)
    expect(isOAuthPopupMessage(messageEvent(null, ORIGIN, popup), popup, 'oauth-success', ORIGIN)).toBe(false)
  })

  it('defaults the expected origin to the current page origin', () => {
    expect(isOAuthPopupMessage(messageEvent({ type: 'oauth-success' }, window.location.origin, popup), popup)).toBe(true)
  })
})

describe('callbackUrlOriginDiffers', () => {
  it('is false for a callback on the page origin', () => {
    expect(callbackUrlOriginDiffers(`${ORIGIN}/api/v1/oauth/flow/callback`, ORIGIN)).toBe(false)
  })

  it('is true for a callback on another origin', () => {
    expect(callbackUrlOriginDiffers('https://api.example.test/api/v1/oauth/flow/callback', ORIGIN)).toBe(true)
    expect(callbackUrlOriginDiffers('http://app.example.test/api/v1/oauth/flow/callback', ORIGIN)).toBe(true)
  })

  it('is false for empty or unparsable input', () => {
    expect(callbackUrlOriginDiffers('', ORIGIN)).toBe(false)
    expect(callbackUrlOriginDiffers(undefined, ORIGIN)).toBe(false)
    expect(callbackUrlOriginDiffers('not a url', ORIGIN)).toBe(false)
  })
})
