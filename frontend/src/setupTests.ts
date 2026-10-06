import '@testing-library/jest-dom'
import { createElement } from 'react'
import { vi } from 'vitest'

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
