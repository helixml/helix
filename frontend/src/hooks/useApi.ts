import axios, { AxiosRequestConfig } from 'axios'
import { useContext, useCallback } from 'react'

import {
  SnackbarContext,
} from '../contexts/snackbar'

import {
  extractErrorMessage,
} from './useErrorCallback'

const API_MOUNT = ""

export interface IApiOptions {
  snackbar?: boolean,
}

export const getTokenHeaders = (token: string) => {
  return {
    Authorization: `Bearer ${token}`,
  }
}

export const useApi = () => {

  const snackbar = useContext(SnackbarContext)

  const get = useCallback(async function<ResT = any>(url: string, axiosConfig?: AxiosRequestConfig, options?: IApiOptions): Promise<ResT | null> {
    try {
      const res = await axios.get<ResT>(`${API_MOUNT}${url}`, axiosConfig)
      return res.data
    } catch (e: any) {
      const errorMessage = extractErrorMessage(e)
      if(options?.snackbar !== false) snackbar.setSnackbar(errorMessage, 'error')
      return null
    }
  }, [])

  const post = useCallback(async function<ReqT = any, ResT = any>(url: string, data: ReqT, axiosConfig?: AxiosRequestConfig, options?: IApiOptions): Promise<ResT | null> {
    try {
      const res = await axios.post<ResT>(`${API_MOUNT}${url}`, data, axiosConfig)
      return res.data
    } catch (e: any) {
      const errorMessage = extractErrorMessage(e)
      if(options?.snackbar !== false) snackbar.setSnackbar(errorMessage, 'error')
      return null
    }
  }, [])

  const put = useCallback(async function<ReqT = any, ResT = any>(url: string, data: ReqT, axiosConfig?: AxiosRequestConfig, options?: IApiOptions): Promise<ResT | null> {
    try {
      const res = await axios.put<ResT>(`${API_MOUNT}${url}`, data, axiosConfig)
      return res.data
    } catch (e: any) {
      const errorMessage = extractErrorMessage(e)
      if(options?.snackbar !== false) snackbar.setSnackbar(errorMessage, 'error')
      return null
    }
  }, [])

  const del = useCallback(async function<ResT = any>(url: string, axiosConfig?: AxiosRequestConfig, options?: IApiOptions): Promise<ResT | null> {
    try {
      const res = await axios.delete<ResT>(`${API_MOUNT}${url}`, axiosConfig)
      return res.data
    } catch (e: any) {
      const errorMessage = extractErrorMessage(e)
      if(options?.snackbar !== false) snackbar.setSnackbar(errorMessage, 'error')
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