import React, { FC, useState, useCallback } from 'react'
import useApi from '../hooks/useApi'
import useSnackbar from './useSnackbar'

import {
  // ISession,
  ISessionSummary,
  // IBot,
  // ISessionConfig,
} from '../types'

export const useSession = () => {
  const api = useApi()
  const snackbar = useSnackbar()
  
  // const [ data, setData ] = useState<ISession>()
  const [ summary, setSummary ] = useState<ISessionSummary>()


  const loadSessionSummary = useCallback(async (id: string) => {
    const result = await api.get<ISessionSummary>(`/api/v1/sessions/${id}/summary`)
    if(!result) return
    setSummary(result)
  }, [])
  
  return {
    summary,
    loadSessionSummary,
  }
}

export default useSession