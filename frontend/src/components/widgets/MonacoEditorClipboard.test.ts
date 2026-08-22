import { afterEach, describe, expect, it, vi } from 'vitest'
import { getMonacoClipboardOverrideServices } from '../../utils/clipboard'

describe('MonacoEditor clipboard integration', () => {
  const originalSecureContext = Object.getOwnPropertyDescriptor(window, 'isSecureContext')
  const originalClipboard = Object.getOwnPropertyDescriptor(navigator, 'clipboard')
  const originalUserAgent = Object.getOwnPropertyDescriptor(navigator, 'userAgent')

  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
    if (originalSecureContext) {
      Object.defineProperty(window, 'isSecureContext', originalSecureContext)
    } else {
      Reflect.deleteProperty(window, 'isSecureContext')
    }
    if (originalClipboard) {
      Object.defineProperty(navigator, 'clipboard', originalClipboard)
    } else {
      Reflect.deleteProperty(navigator, 'clipboard')
    }
    if (originalUserAgent) {
      Object.defineProperty(navigator, 'userAgent', originalUserAgent)
    }
    document.body.innerHTML = ''
  })

  it('does not access navigator.clipboard after Monaco initializes on HTTP', async () => {
    const clipboardGetter = vi.fn(() => {
      throw new TypeError('navigator.clipboard is unavailable')
    })
    Object.defineProperty(window, 'isSecureContext', {
      configurable: true,
      value: false,
    })
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      get: clipboardGetter,
    })
    Object.defineProperty(navigator, 'userAgent', {
      configurable: true,
      value: 'Mozilla/5.0 (iPad; CPU OS 18_6 like Mac OS X) AppleWebKit/605.1.15 Version/18.6 Mobile/15E148 Safari/604.1',
    })
    Object.defineProperty(window, 'matchMedia', {
      configurable: true,
      value: () => ({
        matches: false,
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
      }),
    })
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue({
      webkitBackingStorePixelRatio: 1,
      mozBackingStorePixelRatio: 1,
      msBackingStorePixelRatio: 1,
      oBackingStorePixelRatio: 1,
      backingStorePixelRatio: 1,
      measureText: () => ({ width: 8 }),
      clearRect: vi.fn(),
      fillRect: vi.fn(),
      fillText: vi.fn(),
    } as unknown as CanvasRenderingContext2D)
    vi.stubGlobal('ResizeObserver', class {
      observe() {}
      unobserve() {}
      disconnect() {}
    })

    const monaco = await import('monaco-editor/esm/vs/editor/editor.api.js')
    const container = document.body.appendChild(document.createElement('div'))
    const editor = monaco.editor.create(
      container,
      { value: 'hello', language: 'plaintext' },
      getMonacoClipboardOverrideServices(),
    )

    expect(() => container.click()).not.toThrow()
    expect(clipboardGetter).not.toHaveBeenCalled()

    editor.dispose()
  })
})
