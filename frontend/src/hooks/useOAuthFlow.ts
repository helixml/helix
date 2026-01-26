import { useState, useCallback } from 'react'
import useApi from './useApi'

interface StartOAuthFlowOptions {
  providerId: string
  scopes?: string[]
  onSuccess?: () => void
  onError?: (error: string) => void
}

interface UseOAuthFlowResult {
  startOAuthFlow: (options: StartOAuthFlowOptions) => Promise<void>
  isLoading: boolean
  error: string | null
}

/**
 * Hook for starting OAuth flows with optional scope specification.
 * Opens a popup for the OAuth provider and handles completion detection.
 */
export function useOAuthFlow(): UseOAuthFlowResult {
  const api = useApi()
  const [isLoading, setIsLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const startOAuthFlow = useCallback(async (options: StartOAuthFlowOptions) => {
    const { providerId, scopes, onSuccess, onError } = options

    try {
      setIsLoading(true)
      setError(null)

      // Build URL with optional scopes
      let url = `/api/v1/oauth/flow/start/${providerId}`
      if (scopes && scopes.length > 0) {
        url += `?scopes=${encodeURIComponent(scopes.join(','))}`
      }

      const response = await api.get(url)
      const authUrl = response.auth_url || response?.data?.auth_url

      if (!authUrl) {
        const errMsg = 'Failed to get authorization URL'
        setError(errMsg)
        onError?.(errMsg)
        setIsLoading(false)
        return
      }

      // Open popup
      const width = 800
      const height = 700
      const left = (window.innerWidth - width) / 2
      const top = (window.innerHeight - height) / 2
      const popup = window.open(
        authUrl,
        'oauth-popup',
        `width=${width},height=${height},left=${left},top=${top},toolbar=0,location=0,menubar=0`
      )

      if (!popup) {
        const errMsg = 'Popup blocked! Please allow popups for this site.'
        setError(errMsg)
        onError?.(errMsg)
        setIsLoading(false)
        return
      }

      // Listen for OAuth completion message
      const handleMessage = (event: MessageEvent) => {
        if (event.data?.type === 'oauth-success') {
          cleanup()
          onSuccess?.()
        }
      }

      // Poll for popup close as fallback
      const pollInterval = setInterval(() => {
        if (popup.closed) {
          cleanup()
          onSuccess?.()
        }
      }, 500)

      const cleanup = () => {
        window.removeEventListener('message', handleMessage)
        clearInterval(pollInterval)
        setIsLoading(false)
      }

      window.addEventListener('message', handleMessage)
    } catch (err: any) {
      console.error('Failed to start OAuth flow:', err)
      const errMsg = 'Failed to start OAuth authentication'
      setError(errMsg)
      onError?.(errMsg)
      setIsLoading(false)
    }
  }, [api])

  return { startOAuthFlow, isLoading, error }
}

export default useOAuthFlow
