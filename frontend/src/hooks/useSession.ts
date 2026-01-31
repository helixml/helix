import React, { FC, useState, useCallback } from 'react'
import useApi from '../hooks/useApi'
import useSnackbar from './useSnackbar'

import {  
  ISessionSummary,  
} from '../types'

import { TypesSession } from '../api/api'

export const useSession = () => {
  const api = useApi()
  const snackbar = useSnackbar()
  
  const [ data, setData ] = useState<TypesSession>()
  const [ summary, setSummary ] = useState<ISessionSummary>()

  const loadSession = useCallback(async (id: string) => {
    const result = await api.get<TypesSession>(`/api/v1/sessions/${id}`, undefined, {
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
    const result = await loadSession(data.id || '')
    return result
  }, [
    data,
  ])


  return {
    data,
    summary,
    reload,    
    loadSession,
    loadSessionSummary,    
    setData,    
  }
}

export default useSession