import axios, { AxiosRequestConfig } from 'axios'
import { useContext, useCallback } from 'react'

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
        snackbar.setSnackbar(errorMessage, 'error')
        reportError(new Error(errorMessage))
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
        snackbar.setSnackbar(errorMessage, 'error')
        reportError(new Error(errorMessage))
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
        snackbar.setSnackbar(errorMessage, 'error')
        reportError(new Error(errorMessage))
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
        snackbar.setSnackbar(errorMessage, 'error')
        reportError(new Error(errorMessage))
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
  }, [])

  return {
    get,
    post,
    put,
    delete: del,
    setToken,
  }
}

export default useApi