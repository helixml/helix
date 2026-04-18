import { createRoot } from 'react-dom/client';
import App from './App'
import ErrorBoundary from './components/system/ErrorBoundary'
import { isMobileOrTablet } from './utils/isMobileOrTablet'
import { logErrorToSession, getRecentErrors, clearErrorLog } from './utils/errorSessionLog'

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

// === Mobile error overlay (plain HTML, works even when React is dead) ===
// On mobile/tablet, catch unhandled JS errors and show debug info instead of
// a white screen. This prevents Safari's "A problem repeatedly occurred" crash
// cascade where: error → white page → Safari auto-reloads → same error → repeat.
// On desktop, errors propagate normally (dev tools available).
if (isMobileOrTablet()) {
  function renderErrorOverlay(message: string, stack?: string, skipLog?: boolean) {
    const overlay = document.getElementById('error-overlay')
    if (!overlay) return

    // Log to sessionStorage so errors survive WebKit process crashes.
    // Recovery replays pass skipLog=true to avoid chaining "(recovered)" entries.
    if (!skipLog) {
      logErrorToSession(message, stack)
    }

    // Forward to Sentry/analytics
    try {
      if (typeof win.emitError === 'function') {
        win.emitError(new Error(message))
      }
    } catch {
      // ignore
    }

    const previousErrors = getRecentErrors()
    const previousHtml = previousErrors.length > 1
      ? `<details style="margin-bottom:12px">
          <summary style="cursor:pointer;color:#888">Previous errors (${previousErrors.length})</summary>
          <pre style="white-space:pre-wrap;word-break:break-word;background:#0d0d1a;padding:12px;border-radius:4px;max-height:150px;overflow:auto;font-size:11px;line-height:1.4;margin-top:4px">${
            previousErrors.map(e => `[${e.timestamp}] ${e.message}`).join('\n')
          }</pre>
        </details>`
      : ''

    // GPU-safe: no position:fixed, no filter, no will-change, no opacity animations
    overlay.innerHTML = `
      <div style="position:absolute;top:0;left:0;right:0;bottom:0;z-index:99999;background:#1a1a2e;color:#e0e0e0;font-family:monospace;font-size:13px;overflow:auto;-webkit-overflow-scrolling:touch;padding:16px;box-sizing:border-box">
        <div style="border-bottom:3px solid #e74c3c;padding-bottom:12px;margin-bottom:12px;display:flex;align-items:flex-start;justify-content:space-between">
          <div>
            <h2 style="margin:0;color:#e74c3c;font-size:18px">JavaScript Error</h2>
            <p style="margin:4px 0 0;color:#888;font-size:11px">${new Date().toISOString()} &mdash; ${window.location.pathname}</p>
          </div>
          <button id="error-dismiss-btn" style="padding:6px 14px;border:1px solid #555;border-radius:4px;background:#2a2a3e;color:#aaa;font-family:monospace;font-size:13px;cursor:pointer;flex-shrink:0;margin-left:12px">✕ Dismiss</button>
        </div>
        <div style="margin-bottom:16px">
          <strong style="color:#ff6b6b">${message}</strong>
        </div>
        ${stack ? `<pre style="white-space:pre-wrap;word-break:break-word;background:#0d0d1a;padding:12px;border-radius:4px;max-height:200px;overflow:auto;font-size:11px;line-height:1.4;margin-bottom:12px">${stack}</pre>` : ''}
        ${previousHtml}
        <div>
          <button id="error-copy-btn" style="padding:10px 20px;margin-right:10px;margin-top:8px;border:1px solid #555;border-radius:4px;background:#2a2a3e;color:#e0e0e0;font-family:monospace;font-size:14px;cursor:pointer">Copy Error</button>
          <button id="error-reload-btn" style="padding:10px 20px;margin-top:8px;border:1px solid #e74c3c;border-radius:4px;background:#e74c3c;color:#e0e0e0;font-family:monospace;font-size:14px;cursor:pointer">Reload Page</button>
        </div>
      </div>
    `

    document.getElementById('error-dismiss-btn')?.addEventListener('click', () => {
      clearErrorLog()
      overlay.innerHTML = ''
    })
    document.getElementById('error-reload-btn')?.addEventListener('click', () => window.location.reload())
    document.getElementById('error-copy-btn')?.addEventListener('click', () => {
      let text = `Error: ${message}\n`
      if (stack) text += `\nStack:\n${stack}\n`
      if (previousErrors.length > 0) {
        text += `\nPrevious errors:\n`
        previousErrors.forEach(e => { text += `[${e.timestamp}] ${e.message}\n` })
      }
      text += `\nURL: ${window.location.href}\nUA: ${navigator.userAgent}\nTime: ${new Date().toISOString()}`
      navigator.clipboard.writeText(text).catch(() => {})
    })
  }

  window.onerror = (message, _source, _lineno, _colno, error) => {
    renderErrorOverlay(
      String(message),
      error?.stack
    )
    // Return true to prevent default browser error handling (which causes the white page)
    return true
  }

  window.addEventListener('unhandledrejection', (event) => {
    const reason = event.reason
    renderErrorOverlay(
      reason?.message || String(reason) || 'Unhandled promise rejection',
      reason?.stack
    )
  })

  // On page load, check for previous errors from a WebKit process crash.
  // Clear the log immediately so the same error doesn't replay on every refresh.
  const previousErrors = getRecentErrors()
  if (previousErrors.length > 0) {
    const lastError = previousErrors[previousErrors.length - 1]
    const timeSinceLastError = Date.now() - new Date(lastError.timestamp).getTime()
    // Only show if the last error was within the last 30 seconds (likely a crash/reload)
    if (timeSinceLastError < 30000) {
      clearErrorLog() // prevent the same error re-appearing on the next refresh
      renderErrorOverlay(
        `(recovered) ${lastError.message}`,
        lastError.stack,
        true // skipLog: don't chain a "(recovered)" entry into sessionStorage
      )
    }
  }
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
  const root = createRoot(container);
  root.render(
    <ErrorBoundary>
      <App />
    </ErrorBoundary>
  );
}
