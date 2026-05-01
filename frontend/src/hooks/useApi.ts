import axios, { AxiosRequestConfig } from 'axios'
import { useContext, useCallback } from 'react'
import { Api } from '../api/api'

import {
  SnackbarContext,
} from '../contexts/snackbar'

import {
  LoadingContext,
} from '../contexts/loading'

import {
  extractErrorMessage,
} from './useErrorCallback'

import {
  reportError,
} from '../utils/analytics'

import {
  getCSRFToken,
  CSRF_HEADER_NAME,
} from '../utils/csrf'

const API_MOUNT = ""

export interface IApiOptions {
  snackbar?: boolean,
  loading?: boolean,
  errorCapture?: (err: string) => void,
}

// CSRF interceptor function - adds X-CSRF-Token header to state-changing requests
const csrfInterceptor = (config: any) => {
  const method = config.method?.toUpperCase()
  // Only add CSRF header for state-changing methods
  if (method === 'POST' || method === 'PUT' || method === 'DELETE' || method === 'PATCH') {
    const csrfToken = getCSRFToken()
    if (csrfToken) {
      config.headers[CSRF_HEADER_NAME] = csrfToken
    }
  }
  return config
}

// Create a singleton instance of the API client
// With BFF pattern, no security worker needed - cookies are sent automatically
const apiClientSingleton = new Api({
  baseURL: window.location.origin,
  secure: true,
  withCredentials: true, // Required for BFF pattern - send session cookies with requests
  // No securityWorker needed - session cookie is sent automatically
})

// Embed auth: pages loaded via /embed/* (e.g. inside an iframe in a third-party
// app like the Gatewaze newsletter editor) carry a Helix API key as
// ?access_token=... in the URL. The browser cookie won't be present in that
// context, so wire the token into Authorization headers for both axios and the
// generated API client. Strip the token from the visible URL afterwards so it
// doesn't leak via screenshots, history, or referrer.
const embedToken = (() => {
  if (typeof window === 'undefined') return null
  const params = new URLSearchParams(window.location.search)
  const token = params.get('access_token')
  if (!token) return null
  params.delete('access_token')
  const search = params.toString()
  window.history.replaceState(
    {},
    '',
    window.location.pathname + (search ? '?' + search : '') + window.location.hash,
  )
  return token
})()

if (embedToken) {
  const authValue = `Bearer ${embedToken}`
  axios.defaults.headers.common['Authorization'] = authValue
  apiClientSingleton.instance.defaults.headers.common['Authorization'] = authValue
  // Disable cookie sending so the server's auth middleware falls through
  // to Bearer-token auth. Otherwise an existing helix_session cookie (e.g.
  // because the user is also logged into Helix in the same browser) would
  // win and we'd authenticate as that user, not as the API key owner.
  axios.defaults.withCredentials = false
  apiClientSingleton.instance.defaults.withCredentials = false

  // Some components (e.g. handleStartPlanning in SpecTaskDetailContent)
  // call window.fetch directly instead of going through axios. Patch fetch
  // for same-origin requests so they pick up the embed Bearer token too.
  // Set credentials: 'omit' so any stray cookie doesn't preempt Bearer auth
  // on the server. CSRF middleware skips Bearer-auth requests, so this is
  // safe for state-changing endpoints.
  if (typeof window !== 'undefined' && typeof window.fetch === 'function') {
    const origFetch = window.fetch.bind(window)
    window.fetch = (input: RequestInfo | URL, init: RequestInit = {}) => {
      let isSameOrigin = false
      try {
        const urlStr =
          typeof input === 'string'
            ? input
            : input instanceof URL
              ? input.toString()
              : input.url
        const u = new URL(urlStr, window.location.origin)
        isSameOrigin = u.origin === window.location.origin
      } catch {
        // ignore — pass through unmodified
      }
      if (!isSameOrigin) return origFetch(input, init)

      const baseHeaders =
        input instanceof Request ? input.headers : init.headers
      const headers = new Headers(baseHeaders || {})
      if (!headers.has('Authorization')) {
        headers.set('Authorization', authValue)
      }
      return origFetch(input, { ...init, headers, credentials: 'omit' })
    }
  }

  // Browsers don't allow custom headers on WebSocket. Helix's WS handler
  // accepts ?access_token=<key> as a fallback (see auth_utils.go
  // getRequestToken), so for embed contexts we patch the constructor to
  // append it to same-origin URLs that don't already carry an auth param.
  if (typeof window !== 'undefined' && typeof window.WebSocket === 'function') {
    const OrigWebSocket = window.WebSocket
    const PatchedWebSocket = function (
      this: WebSocket,
      url: string | URL,
      protocols?: string | string[],
    ) {
      let finalUrl: string | URL = url
      try {
        const urlStr = typeof url === 'string' ? url : url.toString()
        const u = new URL(urlStr, window.location.origin)
        const sameHost = u.host === window.location.host
        if (sameHost && !u.searchParams.has('access_token')) {
          u.searchParams.set('access_token', embedToken)
          finalUrl = u.toString()
        }
      } catch {
        // ignore — pass through unmodified
      }
      return new OrigWebSocket(finalUrl, protocols)
    } as unknown as typeof WebSocket
    PatchedWebSocket.prototype = OrigWebSocket.prototype
    // Mirror the readyState constants so consumers using
    // `WebSocket.OPEN` etc. continue to work after replacement.
    Object.defineProperties(PatchedWebSocket, {
      CONNECTING: { value: OrigWebSocket.CONNECTING },
      OPEN: { value: OrigWebSocket.OPEN },
      CLOSING: { value: OrigWebSocket.CLOSING },
      CLOSED: { value: OrigWebSocket.CLOSED },
    })
    window.WebSocket = PatchedWebSocket
  }
}

// Add interceptors to the Api client's axios instance
apiClientSingleton.instance.interceptors.request.use(csrfInterceptor)

// Configure axios to send cookies with requests (same-origin).
// Skip when an embed token is in play — see embed-auth block above.
if (!embedToken) {
  axios.defaults.withCredentials = true
}

// Add interceptors for direct axios usage
axios.interceptors.request.use(csrfInterceptor)

// Response error interceptor: replace Axios's generic "Request failed with status code 500"
// with the actual error message from the backend response body. This ensures that
// catch blocks using `error.message` show the real error, not just the status code.
const enhanceErrorMessage = (error: any) => {
  const data = error.response?.data
  if (data && typeof data === 'string') {
    const body = data.trim()
    if (body.length > 0 && body.length < 1000 && !body.startsWith('<!')) {
      error.message = body
    }
  } else if (data && typeof data === 'object' && typeof data.message === 'string' && data.message.length > 0) {
    error.message = data.message
  }
  return Promise.reject(error)
}
axios.interceptors.response.use(undefined, enhanceErrorMessage)
apiClientSingleton.instance.interceptors.response.use(undefined, enhanceErrorMessage)

// Helper function to check if an error is auth-related
const isAuthError = (error: any): boolean => {
  // Check status code
  if (error.response?.status === 401 || error.response?.status === 403) {
    return true
  }

  // Check error message for common auth failure patterns
  const errorMessage = extractErrorMessage(error).toLowerCase()
  const authErrorPatterns = [
    'unauthorized',
    'token expired',
    'token invalid',
    'authentication failed',
    'access denied',
    'forbidden',
    'not authenticated',
    'invalid token',
    'expired token'
  ]

  return authErrorPatterns.some(pattern => errorMessage.includes(pattern))
}

export const useApi = () => {

  const snackbar = useContext(SnackbarContext)
  const loading = useContext(LoadingContext)

  const get = useCallback(async function<ResT = any>(url: string, axiosConfig?: AxiosRequestConfig, options?: IApiOptions): Promise<ResT | null> {
    if(options?.loading === true) loading.setLoading(true)
    try {
      const res = await axios.get<ResT>(`${API_MOUNT}${url}`, axiosConfig)
      if(options?.loading === true) loading.setLoading(false)
      return res.data
    } catch (e: any) {
      const errorMessage = extractErrorMessage(e)
      console.error(errorMessage)
      options?.errorCapture?.(errorMessage)
      if(options?.snackbar !== false && !isAuthError(e)) {
        const safeErrorMsg = typeof errorMessage === 'string' ? errorMessage : 'An error occurred'
        snackbar.setSnackbar(safeErrorMsg, 'error')
        reportError(new Error(safeErrorMsg))
      }
      if(options?.loading === true) loading.setLoading(false)
      return null
    }
  }, [])

  const post = useCallback(async function<ReqT = any, ResT = any>(url: string, data: ReqT, axiosConfig?: AxiosRequestConfig, options?: IApiOptions): Promise<ResT | null> {
    if(options?.loading === true) loading.setLoading(true)
    try {
      const res = await axios.post<ResT>(`${API_MOUNT}${url}`, data, axiosConfig)
      if(options?.loading === true) loading.setLoading(false)
      return res.data
    } catch (e: any) {
      const errorMessage = extractErrorMessage(e)
      console.error(errorMessage)
      options?.errorCapture?.(errorMessage)
      if(options?.snackbar !== false && !isAuthError(e)) {
        const safeErrorMsg = typeof errorMessage === 'string' ? errorMessage : 'An error occurred'
        snackbar.setSnackbar(safeErrorMsg, 'error')
        reportError(new Error(safeErrorMsg))
      }
      if(options?.loading === true) loading.setLoading(false)
      return null
    }
  }, [])

  const put = useCallback(async function<ReqT = any, ResT = any>(url: string, data: ReqT, axiosConfig?: AxiosRequestConfig, options?: IApiOptions): Promise<ResT | null> {
    if(options?.loading === true) loading.setLoading(true)
    try {
      const res = await axios.put<ResT>(`${API_MOUNT}${url}`, data, axiosConfig)
      if(res.status >= 400) {
        throw new Error(`${res.status} ${res.statusText}`)
      }
      if(options?.loading === true) loading.setLoading(false)
      return res.data
    } catch (e: any) {
      const errorMessage = extractErrorMessage(e)
      console.error(errorMessage)
      options?.errorCapture?.(errorMessage)
      if(options?.snackbar !== false && !isAuthError(e)) {
        const safeErrorMsg = typeof errorMessage === 'string' ? errorMessage : 'An error occurred'
        snackbar.setSnackbar(safeErrorMsg, 'error')
        reportError(new Error(safeErrorMsg))
        throw e
      }
      if(options?.loading === true) loading.setLoading(false)
      return null
    }
  }, [])

  const del = useCallback(async function<ResT = any>(url: string, axiosConfig?: AxiosRequestConfig, options?: IApiOptions): Promise<ResT | null> {
    if(options?.loading === true) loading.setLoading(true)
    try {
      const res = await axios.delete<ResT>(`${API_MOUNT}${url}`, axiosConfig)
      if(options?.loading === true) loading.setLoading(false)
      return res.data
    } catch (e: any) {
      const errorMessage = extractErrorMessage(e)
      console.error(errorMessage)
      options?.errorCapture?.(errorMessage)
      if(options?.snackbar !== false && !isAuthError(e)) {
        const safeErrorMsg = typeof errorMessage === 'string' ? errorMessage : 'An error occurred'
        snackbar.setSnackbar(safeErrorMsg, 'error')
        reportError(new Error(safeErrorMsg))
      }
      if(options?.loading === true) loading.setLoading(false)
      return null
    }
  }, [])

  const getApiClient = useCallback(() => {
    return apiClientSingleton.api
  }, [])

  const getV1Client = useCallback(() => {
    return apiClientSingleton.v1
  }, [])

  return {
    get,
    post,
    put,
    delete: del,
    getApiClient,
    getV1Client,
  }
}

export default useApi
