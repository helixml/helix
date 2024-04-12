import React, { FC, useState, useCallback, useMemo } from 'react'
import useApi from '../hooks/useApi'

import {
  IApp,
  IAppConfig,
  IAppType,
  APP_TYPE_GITHUB,
  APP_TYPE_HELIX,
} from '../types'

import {
  generateAmusingName,
} from '../utils/names'

export const useApps = () => {
  const api = useApi()
  
  const [ data, setData ] = useState<IApp[]>([])

  const helixApps = useMemo(() => {
    return data.filter(app => app.app_type == APP_TYPE_HELIX)
  }, [
    data,
  ])

  const githubApps = useMemo(() => {
    return data.filter(app => app.app_type == APP_TYPE_GITHUB)
  }, [
    data,
  ])

  const loadData = useCallback(async () => {
    const result = await api.get<IApp[]>(`/api/v1/apps`, undefined, {
      snackbar: true,
    })
    if(!result) return
    setData(result)
  }, [])

  const createApp = useCallback(async (
    name: string,
    description: string,
    app_type: IAppType,
    config: IAppConfig,
  ): Promise<IApp | undefined> => {
    const result = await api.post<Partial<IApp>, IApp>(`/api/v1/apps`, {
      name: name ? name: generateAmusingName(),
      description,
      app_type,
      config,
    }, {}, {
      snackbar: true,
    })
    if(!result) return
    loadData()
    return result
  }, [
    loadData,
  ])

  const updateApp = useCallback(async (id: string, data: Partial<IApp>): Promise<IApp | undefined> => {
    const result = await api.put<Partial<IApp>, IApp>(`/api/v1/apps/${id}`, data, {}, {
      snackbar: true,
    })
    if(!result) return
    loadData()
    return result
  }, [
    loadData,
  ])

  const deleteApp = useCallback(async (id: string): Promise<boolean | undefined> => {
    await api.delete(`/api/v1/apps/${id}`, {}, {
      snackbar: true,
    })
    loadData()
    return true
  }, [
    loadData,
  ])

  return {
    data,
    helixApps,
    githubApps,
    loadData,
    createApp,
    updateApp,
    deleteApp,
  }
}

export default useApps