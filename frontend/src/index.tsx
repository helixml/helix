import React from 'react'
import ReactDOM from 'react-dom'
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

let render = () => {
  ReactDOM.render(
    <>
      <App />
    </>,
    document.getElementById("root")
  )
}

render()
