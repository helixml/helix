import { useEffect, useRef } from 'react'
import { useGetConfig } from '../services/userService'

const win = (window as any)

export default function useAnalyticsInit() {
  const { data: config } = useGetConfig()
  const initialized = useRef(false)

  useEffect(() => {
    if (!config || initialized.current) return
    initialized.current = true

    // Google Analytics
    if (config.google_analytics_frontend) {
      const gaId = config.google_analytics_frontend
      const script = document.createElement('script')
      script.src = `https://www.googletagmanager.com/gtag/js?id=${gaId}`
      script.async = true
      document.head.appendChild(script)

      win.dataLayer = win.dataLayer || []
      function gtag(...args: any[]) { win.dataLayer.push(args) }
      gtag('js', new Date())
      gtag('config', gaId)
    }
  }, [config])
}
