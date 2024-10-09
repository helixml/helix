import React, { FC, useState, useCallback } from 'react'
import useApi from '../hooks/useApi'
import useSnackbar from './useSnackbar'

import {
  ISession,
  ISessionSummary,
  IBot,
  ISessionConfig,
} from '../types'

export const useSession = () => {
  const api = useApi()
  const snackbar = useSnackbar()
  
  const [ data, setData ] = useState<ISession>()
  const [ summary, setSummary ] = useState<ISessionSummary>()

  const loadSession = useCallback(async (id: string) => {
    const result = await api.get<ISession>(`/api/v1/sessions/${id}`, undefined, {
      snackbar: true,
    })
    if(!result) return
    setData(result)
    return result
  }, [])

  const loadSessionSummary = useCallback(async (id: string) => {
    const result = await api.get<ISessionSummary>(`/api/v1/sessions/${id}/summary`)
    if(!result) return
    setSummary(result)
  }, [])
  
  const reload = useCallback(async () => {
    if(!data) return
    const result = await loadSession(data.id)
    return result
  }, [
    data,
  ])

  const retryTextFinetune = useCallback(async (id: string) => {
    const result = await api.put(`/api/v1/sessions/${id}/finetune/text/retry`, undefined, {}, {
      loading: true,
    })
    if(!result) return
    loadSession(id)
    snackbar.success('Text finetune retry requested')
  }, [
    loadSession,
  ])

  const clone = useCallback(async (sessionID: string, interactionID: string): Promise<undefined | ISession> => {
    const result = await api.put<undefined, ISession>(`/api/v1/sessions/${sessionID}/clone/${interactionID}`, undefined, {}, {
      loading: true,
    })
    if(!result) return
    snackbar.success('Session cloned')
    return result
    // router.navigate("session", {session_id: result.id})
  }, [])

  const updateConfig = useCallback(async (sessionID: string, config: ISessionConfig): Promise<undefined | ISessionConfig> => {
    const result = await api.put<ISessionConfig, ISessionConfig>(`/api/v1/sessions/${sessionID}/config`, config, {}, {
      loading: true,
    })
    if(!result) return
    snackbar.success('Session sharing updated')
    return result
  }, [])

  return {
    data,
    summary,
    reload,
    retryTextFinetune,
    loadSession,
    loadSessionSummary,
    clone,
    setData,
    updateConfig,
  }
}

export default useSession