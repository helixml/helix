import '@testing-library/jest-dom'
import { configure } from '@testing-library/react'
import { createElement } from 'react'
import { afterEach, vi } from 'vitest'

// vitest.config.ts gives CI tests a 30s timeout because the runner shares its
// CPUs; give findBy*/waitFor the same headroom, or a single slow role query
// under load exhausts their 1s default first.
if (process.env.CI) configure({ asyncUtilTimeout: 10_000 })

// Stub global window objects used in components
global.window.scrollTo = vi.fn();

// jsdom never implements window.isSecureContext, so it reads as undefined and
// every component using the shared clipboard helper silently takes the
// insecure-context (execCommand) path, which jsdom also lacks. Helix is served
// over HTTPS/localhost in practice, so default the flag to true like real
// deployments. Tests that exercise the insecure fallback redefine it per case.
Object.defineProperty(window, 'isSecureContext', {
  configurable: true,
  value: true,
});

// jsdom's Blob predates Blob.arrayBuffer(), which every supported browser has.
if (typeof Blob.prototype.arrayBuffer !== 'function') {
  Blob.prototype.arrayBuffer = function arrayBuffer(this: Blob) {
    return new Promise<ArrayBuffer>((resolve, reject) => {
      const reader = new FileReader()
      reader.onload = () => resolve(reader.result as ArrayBuffer)
      reader.onerror = () => reject(reader.error)
      reader.readAsArrayBuffer(this)
    })
  }
}

// jsdom performs real HTTP, so an unmocked request depends on whatever happens
// to listen on localhost:3000 (in CI: nothing, refused after a variable delay).
// Refuse it without touching the network and fail the test that made it.
const unmockedRequests: string[] = []
const refuseRequest = (request: string): never => {
  unmockedRequests.push(request)
  throw new Error(`Unmocked network request in a test: ${request}`)
}
XMLHttpRequest.prototype.open = function open(method: string, url: string | URL) {
  refuseRequest(`${method} ${url}`)
}
globalThis.fetch = (input: RequestInfo | URL) =>
  Promise.reject(refuseRequest(`fetch ${input instanceof Request ? input.url : String(input)}`))
afterEach(() => {
  if (unmockedRequests.length === 0) return
  const requests = unmockedRequests.splice(0)
  throw new Error(`Unmocked network request(s): ${requests.join(', ')}. Mock the service hook or API client.`)
})

// Stub localStorage if not fully provided by jsdom
if (!global.localStorage || typeof global.localStorage.clear !== 'function') {
  const store: Record<string, string> = {};
  global.localStorage = {
    getItem: (key: string) => store[key] ?? null,
    setItem: (key: string, value: string) => { store[key] = value; },
    removeItem: (key: string) => { delete store[key]; },
    clear: () => { Object.keys(store).forEach(k => delete store[k]); },
    get length() { return Object.keys(store).length; },
    key: (index: number) => Object.keys(store)[index] ?? null,
  } as Storage;
}

// Mock for react-markdown that preserves HTML structure
vi.mock('react-markdown', () => {
  return {
    default: ({ children }: { children: string }) => {
      return children // Pass through content for tests
    }
  }
})

// Mock react-syntax-highlighter: skip tokenizing, but keep the real
// <pre><code> wrapper and the caller's styles so tests can assert the code
// surface (inline-code CSS keys off `:not(pre) > code`).
vi.mock('react-syntax-highlighter', () => {
  const SyntaxHighlighterMock = ({
    children,
    customStyle,
    codeTagProps,
    PreTag = 'pre',
    CodeTag = 'code',
  }: any) =>
    createElement(
      PreTag,
      { style: customStyle },
      createElement(CodeTag, codeTagProps, children),
    )

  return {
    Prism: SyntaxHighlighterMock
  }
})

// Mock styles
vi.mock('react-syntax-highlighter/dist/esm/styles/prism', () => {
  return {
    oneDark: {},
    oneLight: {}
  }
})
