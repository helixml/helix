import { createRoot } from 'react-dom/client';
import App from './App'
import * as Sentry from '@sentry/react'
import { RudderAnalytics } from '@rudderstack/analytics-js'

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

if(win.HELIX_SENTRY_DSN) {
  win.setUserFunctions.push((user: any) => {
    Sentry.setUser({
      email: user.email,
      name: user.name,
    })
  })
  win.emitErrorFunctions.push((error: any) => {
    Sentry.captureException(error)
  })
  Sentry.init({
    dsn: win.HELIX_SENTRY_DSN,
    integrations: [
      new Sentry.BrowserTracing(),
      new Sentry.Replay()
    ],
    // Set tracesSampleRate to 1.0 to capture 100%
    // of transactions for performance monitoring.
    tracesSampleRate: 0.1,
    replaysSessionSampleRate: 1.0,
    replaysOnErrorSampleRate: 1.0,
    beforeSend(event, hint) {
      // Check if it is an exception, and if so, show the report dialog
      if (event.exception) {
        Sentry.showReportDialog({
          eventId: event.event_id,
        })
      }
      return event
    },
  })
}

if(win.RUDDERSTACK_WRITE_KEY && win.RUDDERSTACK_DATA_PLANE_URL) {
  const rudderAnalytics = new RudderAnalytics()
  rudderAnalytics.load(win.RUDDERSTACK_WRITE_KEY, win.RUDDERSTACK_DATA_PLANE_URL, {})
  win.setUserFunctions.push((user: any) => {
    const {
      token,
      ...safeUser
    } = user
    console.log(`emitting rudderstack user: ${user.email}`)
    console.log(safeUser)
    rudderAnalytics.identify(user.email, safeUser)
  })
  win.viewPageFunctions.push((state: any) => {
    const route = state.route
    console.log(`emitting rudderstack page: ${route.name}`)
    console.log(route)
    rudderAnalytics.page(route.name, route.path, route)
  })
  win.emitErrorFunctions.push((err: any) => {

  })
  win.emitEventFunctions.push((ev: any) => {
    console.log(`emitting rudderstack event: ${ev.name}`)
    console.log(ev)
    rudderAnalytics.track(ev.name, ev)
  })
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
      console.log('[Cursor:iframe] cursor changed:', lastCursor, '→', cursor, 'target:', (e.target as Element).tagName, (e.target as Element).className)
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