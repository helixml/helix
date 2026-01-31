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

// Add response interceptor to log 401 errors for debugging
// NOTE: We intentionally DON'T redirect on 401 here - the account context handles auth state
// Redirecting on any 401 was too aggressive and caused refresh loops when APIs returned 401
// for other reasons (e.g., missing project access, not session expiration)
apiClientSingleton.instance.interceptors.response.use(
  (response) => response,
  (error) => {
    if (error.response?.status === 401) {
      const url = error.config?.url || ''
      const isAuthEndpoint = url.includes('/api/v1/auth/')

      if (!isAuthEndpoint) {
        // Log the 401 error for debugging (but don't redirect)
        console.error('[API] 401 Unauthorized error:', {
          url: url,
          method: error.config?.method,
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