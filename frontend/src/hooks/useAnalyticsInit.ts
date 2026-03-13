import { useEffect, useRef } from 'react'
import * as Sentry from '@sentry/react'
import { RudderAnalytics } from '@rudderstack/analytics-js'
import { useGetConfig } from '../services/userService'

const win = (window as any)

export default function useAnalyticsInit() {
  const { data: config } = useGetConfig()
  const initialized = useRef(false)

  useEffect(() => {
    if (!config || initialized.current) return
    initialized.current = true

    // Sentry
    if (config.sentry_dsn_frontend) {
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
        dsn: config.sentry_dsn_frontend,
        integrations: [
          new Sentry.BrowserTracing(),
          new Sentry.Replay(),
        ],
        tracesSampleRate: 0.1,
        replaysSessionSampleRate: 1.0,
        replaysOnErrorSampleRate: 1.0,
        beforeSend(event) {
          if (event.exception) {
            Sentry.showReportDialog({ eventId: event.event_id })
          }
          return event
        },
      })
    }

    // Google Analytics
    if (config.google_analytics_frontend) {
      const gaId = config.google_analytics_frontend
      const script = document.createElement('script')
      script.src = `https://www.googletagmanager.com/gtag/js?id=${gaId}`
      script.async = true
      document.head.appendChild(script)

      win.dataLayer = win.dataLayer || []
      function gtag() { win.dataLayer.push(arguments) }
      gtag('js', new Date())
      gtag('config', gaId)
    }

    // RudderStack
    if (config.rudderstack_write_key && config.rudderstack_data_plane_url) {
      const rudderAnalytics = new RudderAnalytics()
      rudderAnalytics.load(config.rudderstack_write_key, config.rudderstack_data_plane_url, {})
      win.setUserFunctions.push((user: any) => {
        const { token, ...safeUser } = user
        rudderAnalytics.identify(user.email, safeUser)
      })
      win.viewPageFunctions.push((state: any) => {
        const route = state.route
        rudderAnalytics.page(route.name, route.path, route)
      })
      win.emitEventFunctions.push((ev: any) => {
        rudderAnalytics.track(ev.name, ev)
      })
    }
  }, [config])
}
