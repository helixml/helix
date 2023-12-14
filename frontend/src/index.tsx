import React from "react"
import ReactDOM from "react-dom"
import App from "./App"
import * as Sentry from "@sentry/react"

// TODO: make this env configurable
Sentry.init({
  dsn: 'https://5d8fa564ec0100afd0d5cde554363b52@o4506264252514304.ingest.sentry.io/4506395694006272',
  integrations: [
    new Sentry.BrowserTracing({
      
    }),
    new Sentry.Replay()
  ],
  // Set `tracePropagationTargets` to control for which URLs distributed tracing should be enabled
  tracePropagationTargets: ["localhost", /^https:\/\/app.tryhelix.ai/],
})

let render = () => {
  ReactDOM.render(
    <>
      <App />
    </>,
    document.getElementById("root")
  )
}

render()
