import React, { FC, useEffect, useState, useCallback } from 'react'
import useApi from '../hooks/useApi'
import useAccount from '../hooks/useAccount'
import useWebsocket from './useWebsocket'
import useSnackbar from './useSnackbar'

import {
  ISession,
  WEBSOCKET_EVENT_TYPE_SESSION_UPDATE,
} from '../types'

export const useSession = (session_id: string) => {
  const api = useApi()
  const account = useAccount()
  const snackbar = useSnackbar()

  const [ data, setData ] = useState<ISession>()

  const loadSession = useCallback(async (id: string) => {
    const result = await api.get<ISession>(`/api/v1/sessions/${id}`)
    if(!result) return
    setData(result)
  }, [])
  
  const reload = useCallback(() => {
    if(!session_id) return
    loadSession(session_id)
  }, [
    session_id,
    loadSession,
  ])

  const retryTextFinetune = useCallback(async (id: string) => {
    const result = await api.put(`/api/v1/sessions/${id}/finetune/text/retry`, undefined, {}, {
      loading: true,
    })
    if(!result) return
    loadSession(session_id)
    snackbar.success('Text finetune retry requested')
  }, [
    session_id,
    loadSession,
  ])

  useEffect(() => {
    if(!account.user) return
    if(session_id) {
      loadSession(session_id)
      return  
    } else {
      setData(undefined)
    }
  }, [
    account.user,
    session_id,
  ])

  useWebsocket(session_id, (parsedData) => {
    console.log(parsedData)
    if(parsedData.type === WEBSOCKET_EVENT_TYPE_SESSION_UPDATE && parsedData.session) {
      const newSession: ISession = parsedData.session
      setData(newSession)
    }
  })

  return {
    data,
    reload,
    retryTextFinetune,
  }
}

export default useSession