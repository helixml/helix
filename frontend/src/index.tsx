import React from "react"
import ReactDOM from "react-dom"
import App from "./App"
import * as Sentry from "@sentry/react"

const win = (window as any)

if(win.HELIX_SENTRY_DSN) {
  Sentry.init({
    dsn: win.HELIX_SENTRY_DSN,
    integrations: [
      new Sentry.BrowserTracing({
        
      }),
      new Sentry.Replay()
    ],
    // Set tracesSampleRate to 1.0 to capture 100%
    // of transactions for performance monitoring.
    tracesSampleRate: 1.0,

    // Capture Replay for 10% of all sessions,
    // plus for 100% of sessions with an error
    replaysSessionSampleRate: 0.1,
    replaysOnErrorSampleRate: 1.0,
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
