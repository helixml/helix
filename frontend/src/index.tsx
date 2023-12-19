import React from "react"
import ReactDOM from "react-dom"
import App from "./App"
import * as Sentry from "@sentry/react"

const win = (window as any)
win.sentryUser = {}

if(win.HELIX_SENTRY_DSN) {
  win.setUser = (user: any) => {
    Sentry.setUser({
      email: user.email,
      name: user.name,
    })
  }
  Sentry.init({
    dsn: win.HELIX_SENTRY_DSN,
    integrations: [
      new Sentry.BrowserTracing(),
      new Sentry.Replay()
    ],
    // Set tracesSampleRate to 1.0 to capture 100%
    // of transactions for performance monitoring.
    tracesSampleRate: 1.0,

    // Capture Replay for 100% of all sessions,
    // TODO: reduce this value later (sentry costs $ yo)
    // plus for 100% of sessions with an error
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

let render = () => {
  ReactDOM.render(
    <>
      <App />
    </>,
    document.getElementById("root")
  )
}

render()
