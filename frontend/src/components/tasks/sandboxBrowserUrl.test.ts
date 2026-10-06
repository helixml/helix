import { describe, expect, it } from 'vitest'

import {
  isSandboxBrowserNavigationMessage,
  parseSandboxBrowserTarget,
  sandboxDisplayUrlFromPreview,
  sandboxPreviewUrl,
  sandboxPreviewURLWithScheme,
} from './sandboxBrowserUrl'

describe('parseSandboxBrowserTarget', () => {
  it('normalizes a bare localhost address', () => {
    expect(parseSandboxBrowserTarget('localhost:8080/docs?q=one#intro')).toEqual({
      displayUrl: 'http://localhost:8080/docs?q=one#intro',
      port: 8080,
      path: '/docs?q=one#intro',
    })
  })

  it('uses port 80 when no port is present', () => {
    expect(parseSandboxBrowserTarget('http://127.0.0.1/').port).toBe(80)
  })

  it.each([
    'https://localhost:8443',
    'http://example.com:8080',
    'file:///tmp/index.html',
    'http://user:password@localhost:8080',
  ])('rejects unsupported target %s', (target) => {
    expect(() => parseSandboxBrowserTarget(target)).toThrow()
  })
})

describe('sandboxPreviewUrl', () => {
  it('uses the configured public origin and preserves the path', () => {
    expect(sandboxPreviewUrl(
      'http://share-blue-fox.dev.localhost:8080',
      '/docs?q=one#intro',
      false,
    )).toBe('http://share-blue-fox.dev.localhost:8080/docs?q=one#intro')
  })

  it('keeps a configured HTTPS preview origin', () => {
    expect(sandboxPreviewUrl(
      'https://share-blue-fox.preview.example.com',
      '/',
      true,
    )).toBe('https://share-blue-fox.preview.example.com/')
  })

  it('overrides a stale cached route scheme with current server configuration', () => {
    expect(sandboxPreviewUrl(
      'https://share-blue-fox.dev.localhost:8080',
      '/dashboard',
      false,
    )).toBe('http://share-blue-fox.dev.localhost:8080/dashboard')
  })

  it('repairs an existing browser history entry when configuration changes', () => {
    expect(sandboxPreviewURLWithScheme(
      'https://share-blue-fox.dev.localhost:8080/dashboard?q=one#status',
      false,
    )).toBe('http://share-blue-fox.dev.localhost:8080/dashboard?q=one#status')
  })
})

describe('sandboxDisplayUrlFromPreview', () => {
  it('maps a navigated preview path back to its localhost address', () => {
    expect(sandboxDisplayUrlFromPreview(
      'http://localhost:8080/',
      'http://share-blue-fox.dev.localhost:8080/',
      'http://share-blue-fox.dev.localhost:8080/dashboard?q=one#status',
    )).toBe('http://localhost:8080/dashboard?q=one#status')
  })

  it('rejects navigation reports from another origin', () => {
    expect(sandboxDisplayUrlFromPreview(
      'http://localhost:8080/',
      'http://share-blue-fox.dev.localhost:8080/',
      'https://example.com/dashboard',
    )).toBeUndefined()
  })
})

describe('isSandboxBrowserNavigationMessage', () => {
  it('accepts complete bridge messages', () => {
    expect(isSandboxBrowserNavigationMessage({
      type: 'helix:sandbox-browser:navigate',
      href: 'http://share-blue-fox.dev.localhost:8080/dashboard',
      navigationType: 'push',
    })).toBe(true)
  })

  it('rejects malformed bridge messages', () => {
    expect(isSandboxBrowserNavigationMessage({
      type: 'helix:sandbox-browser:navigate',
      href: 42,
      navigationType: 'push',
    })).toBe(false)
  })
})
