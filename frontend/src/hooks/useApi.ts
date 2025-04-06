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
      if(options?.snackbar !== false) {
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
      if(options?.snackbar !== false) {
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
      if(options?.snackbar !== false) {
        const safeErrorMsg = typeof errorMessage === 'string' ? errorMessage : 'An error occurred'
        snackbar.setSnackbar(safeErrorMsg, 'error')
        reportError(new Error(safeErrorMsg))
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
      if(options?.snackbar !== false) {
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