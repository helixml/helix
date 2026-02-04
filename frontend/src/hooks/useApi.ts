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

const API_MOUNT = ""

export interface IApiOptions {
  snackbar?: boolean,
  loading?: boolean,
  errorCapture?: (err: string) => void,
}

export const getTokenHeaders = (token: string) => {
  return {
    Authorization: `Bearer ${token}`,
  }
}

type SecurityDataType = { token: string }

// Create a singleton instance of the API client
// This ensures it's only initialized once, regardless of how many components use the hook
const apiClientSingleton = new Api({
  baseURL: window.location.origin,
  secure: true,
  securityWorker: (securityData: SecurityDataType | null) => {
    if (securityData && securityData.token) {
      return {
        headers: {
          Authorization: `Bearer ${securityData.token}`,
        }
      }
    }
    return {}
  }
})

// Track if we're currently refreshing to avoid multiple refresh attempts
let isRefreshing = false
let refreshPromise: Promise<boolean> | null = null

// Attempt to refresh the token and update in-memory state
const attemptTokenRefresh = async (): Promise<boolean> => {
  try {
    console.log('[API] Attempting token refresh...')
    // Refresh updates cookies on the backend
    await apiClientSingleton.v1AuthRefreshCreate()

    // Fetch user to get the new token for in-memory storage
    // The v1AuthUserList endpoint reads the fresh cookie and returns the token in the response
    const userResponse = await apiClientSingleton.v1AuthUserList()
    const newToken = (userResponse.data as any)?.token

    if (newToken) {
      // Update both axios defaults and OpenAPI client security data
      axios.defaults.headers.common = getTokenHeaders(newToken)
      apiClientSingleton.setSecurityData({ token: newToken })
      console.log('[API] Token refresh successful, in-memory token updated')
    } else {
      console.log('[API] Token refresh successful (cookie only, no token in response)')
    }

    return true
  } catch (refreshError) {
    console.error('[API] Token refresh failed:', refreshError)
    return false
  }
}

// Add response interceptor to handle:
// 1. X-Token-Refreshed header from transparent backend refresh
// 2. 401 errors with automatic token refresh
apiClientSingleton.instance.interceptors.response.use(
  (response) => {
    // Check if the backend transparently refreshed our token
    // This happens when the access token was expired but the refresh token was valid
    const newToken = response.headers['x-token-refreshed']
    if (newToken) {
      console.log('[API] Token refreshed transparently by backend, updating in-memory token')
      axios.defaults.headers.common = getTokenHeaders(newToken)
      apiClientSingleton.setSecurityData({ token: newToken })
      // Also update localStorage for direct fetch() calls
      localStorage.setItem('token', newToken)
    }
    return response
  },
  async (error) => {
    const originalRequest = error.config

    if (error.response?.status === 401) {
      const url = originalRequest?.url || ''
      const isAuthEndpoint = url.includes('/api/v1/auth/')

      // Don't try to refresh for auth endpoints (would cause infinite loop)
      // Also don't retry if we already tried refreshing for this request
      if (!isAuthEndpoint && !originalRequest._retry) {
        originalRequest._retry = true

        // If we're already refreshing, wait for that to complete
        if (isRefreshing && refreshPromise) {
          const refreshed = await refreshPromise
          if (refreshed) {
            // Delete old Authorization header - the fresh cookies will be used instead
            delete originalRequest.headers['Authorization']
            return apiClientSingleton.instance.request(originalRequest)
          }
        } else {
          // Start a new refresh
          isRefreshing = true
          refreshPromise = attemptTokenRefresh()

          try {
            const refreshed = await refreshPromise
            if (refreshed) {
              // Delete old Authorization header - the fresh cookies will be used instead
              // The backend will use the updated access_token cookie set by the refresh endpoint
              delete originalRequest.headers['Authorization']
              return apiClientSingleton.instance.request(originalRequest)
            }
          } finally {
            isRefreshing = false
            refreshPromise = null
          }
        }

        // Refresh failed, log for debugging
        console.error('[API] 401 Unauthorized after refresh attempt:', {
          url: url,
          method: originalRequest?.method,
          message: error.response?.data?.error || error.message
        })
      }
    }
    return Promise.reject(error)
  }
)

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
      console.log('Sending PUT request to:', `${API_MOUNT}${url}`);
      console.log('Request data:', data);
      const res = await axios.put<ResT>(`${API_MOUNT}${url}`, data, axiosConfig)
      if(res.status >= 400) {
        console.error(`API Error: ${res.status} ${res.statusText}`);
        console.error('Response data:', res.data);
        throw new Error(`${res.status} ${res.statusText}`)
      }
      if(options?.loading === true) loading.setLoading(false)
      return res.data
    } catch (e: any) {
      console.error('Full error object:', e);
      console.error('Error response:', e.response);
      const errorMessage = extractErrorMessage(e)
      console.error(errorMessage)
      options?.errorCapture?.(errorMessage)
      if(options?.snackbar !== false && !isAuthError(e)) {
        const safeErrorMsg = typeof errorMessage === 'string' ? errorMessage : 'An error occurred'
        snackbar.setSnackbar(safeErrorMsg, 'error')
        reportError(new Error(safeErrorMsg))
        // Throw the error anyways
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

  // this will work globally because we are applying this to the root import of axios
  // therefore we don't need to worry about passing the token around to other contexts
  // we can just call useApi() from anywhere and we will get the token injected into the request
  // because the top level account context has called this
  const setToken = useCallback(function(token: string) {
    axios.defaults.headers.common = token ? getTokenHeaders(token) : {}

    // Set token for OpenAPI client
    apiClientSingleton.setSecurityData({
      token: token,
    });

    // Force a direct modification of the client instance's default headers as a fallback
    try {
      apiClientSingleton.instance.defaults.headers.common['Authorization'] = `Bearer ${token}`;
    } catch (e) {
      console.error('Failed to set token directly on client instance:', e);
    }

    // Also set in localStorage for direct fetch() calls (e.g., filestoreService.ts)
    // This ensures all API call paths have access to the current token
    if (token) {
      localStorage.setItem('token', token)
    } else {
      localStorage.removeItem('token')
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
    setToken,
    getApiClient,
    getV1Client,
  }
}

export default useApi