import { createRoot } from 'react-dom/client';
import App from './App'

const win = (window as any)
win.setUserFunctions = []
win.viewPageFunctions = []
win.emitErrorFunctions = []
win.emitEventFunctions = []

win.setUser = (user: any) => {
  win.setUserFunctions.forEach((fn: any) => fn(user))
}

win.viewPage = (page: any) => {
  win.viewPageFunctions.forEach((fn: any) => fn(page))
}

win.emitError = (err: any) => {
  win.emitErrorFunctions.forEach((fn: any) => fn(err))
}

win.emitEvent = (ev: any) => {
  win.emitEventFunctions.forEach((fn: any) => fn(ev))
}

// When running inside an iframe (e.g. macOS desktop app), intercept external
// link clicks and ask the parent frame to open them in the system browser.
if (window.parent !== window) {
  document.addEventListener('click', (e) => {
    const anchor = (e.target as HTMLElement).closest('a')
    if (!anchor) return
    const href = anchor.getAttribute('href')
    if (!href || href.startsWith('#') || href.startsWith('javascript:')) return
    // Only intercept links that go to a different origin
    try {
      const url = new URL(href, window.location.href)
      if (url.origin !== window.location.origin) {
        e.preventDefault()
        window.parent.postMessage({ type: 'open-external-url', url: href }, '*')
      }
    } catch {
      // invalid URL, ignore
    }
  }, true)

  // WKWebView cursor bridge: WKWebView doesn't propagate CSS resize cursors
  // (col-resize, row-resize, etc.) from cross-origin iframe content to native
  // NSCursor. Detect cursor changes and forward them to the parent frame,
  // which calls native NSCursor via a Go binding.
  let lastCursor = ''
  document.addEventListener('mousemove', (e) => {
    const cursor = getComputedStyle(e.target as Element).cursor
    if (cursor !== lastCursor) {
      lastCursor = cursor
      window.parent.postMessage({ type: 'helix:cursor', cursor }, '*')
    }
  }, { passive: true })
}

const container = document.getElementById('root');
if (container) {
  const root = createRoot(container); // createRoot(container!) if you use TypeScript
  root.render(<App />);
}