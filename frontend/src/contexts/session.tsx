import React, { FC, useEffect, createContext, useMemo, useState, useCallback } from 'react'
import ReconnectingWebSocket from 'reconnecting-websocket'
import useApi from '../hooks/useApi'
import useAccount from '../hooks/useAccount'
import useRouter from '../hooks/useRouter'

import {
  ISession,
  ISessionSummary,
  ISessionMetaUpdate,
  IWebsocketEvent,
  WEBSOCKET_EVENT_TYPE_SESSION_UPDATE,
  WEBSOCKET_EVENT_TYPE_WORKER_TASK_RESPONSE,
  WORKER_TASK_RESPONSE_TYPE_PROGRESS,
  WORKER_TASK_RESPONSE_TYPE_STREAM,
  SESSION_CREATOR_SYSTEM,
} from '../types'

import {
  getSessionSummary,
} from '../utils/session'

export interface ISessionContext {
  id: string,
  data?: ISession,
  setID: {
    (id: string): void,
  },
  reload: {
    (): void,
  }
}

export const SessionContext = createContext<ISessionContext>({
  id: '',
  data: undefined,
  setID: () => {},
  reload: () => {},
})

export const useSessionContext = (): ISessionContext => {
  const api = useApi()
  const account = useAccount()

  const [ id, setID ] = useState('')
  const [ data, setData ] = useState<ISession>()

  const loadSession = useCallback(async (id: string) => {
    const result = await api.get<ISession>(`/api/v1/sessions/${id}`)
    if(!result) return
    setData(result)
  }, [])
  
  const reload = useCallback(() => {
    if(!id) return
    loadSession(id)
  }, [
    id,
    loadSession,
  ])

  useEffect(() => {
    if(!account.user) return
    if(id) {
      loadSession(id)
      return  
    } else {
      setData(undefined)
    }
  }, [
    account.user,
    id,
  ])

  useEffect(() => {
    if(!account.user?.token) return
    const wsProtocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const wsHostname = window.location.hostname
    const url = `${wsProtocol}//${wsHostname}/api/v1/ws/user?access_token=${account.user?.token}`
    const rws = new ReconnectingWebSocket(url)
    rws.addEventListener('message', (event) => {
      const parsedData = JSON.parse(event.data) as IWebsocketEvent
      if(parsedData.type === WEBSOCKET_EVENT_TYPE_SESSION_UPDATE && parsedData.session) {
        const newSession: ISession = parsedData.session
        setData(newSession)
      }

      // we have a session update message
      // if(parsedData.type == WEBSOCKET_EVENT_TYPE_WORKER_TASK_RESPONSE && parsedData.worker_task_response) {
      //   const workerResponse = parsedData.worker_task_response
      //   // console.log("got new workerResponse from backend over websocket!")
      //   // console.log(JSON.stringify(workerResponse, null, 4))
      //   if(workerResponse.type == WORKER_TASK_RESPONSE_TYPE_STREAM) {
      //     setSessions(sessions => sessions.map(existingSession => {
      //       if(existingSession.id != workerResponse.session_id) return existingSession
      //       const systemInteractions = existingSession.interactions.filter(i => i.creator == SESSION_CREATOR_SYSTEM)
      //       if(systemInteractions.length <= 0) return existingSession
      //       const lastSystemInteraction = systemInteractions[systemInteractions.length - 1]
      //       const newInteractions = existingSession.interactions.map(i => {
      //         if(i.id != lastSystemInteraction.id) return i
      //         return Object.assign({}, i, {
      //           message: i.message + workerResponse.message,
      //         })
      //       })
      //       const updatedSession = Object.assign({}, existingSession, {
      //         interactions: newInteractions,
      //       })
      //       return updatedSession
      //     }))
      //   } else if(workerResponse.type == WORKER_TASK_RESPONSE_TYPE_PROGRESS) {
      //     setSessions(sessions => sessions.map(existingSession => {
      //       if(existingSession.id != workerResponse.session_id) return existingSession
      //       const systemInteractions = existingSession.interactions.filter(i => i.creator == SESSION_CREATOR_SYSTEM)
      //       if(systemInteractions.length <= 0) return existingSession
      //       const lastSystemInteraction = systemInteractions[systemInteractions.length - 1]
      //       const newInteractions = existingSession.interactions.map(i => {
      //         if(i.id != lastSystemInteraction.id) return i
      //         return Object.assign({}, i, {
      //           progress: workerResponse.progress,
      //           status: workerResponse.status,
      //         })
      //       })
      //       const updatedSession = Object.assign({}, existingSession, {
      //         interactions: newInteractions,
      //       })
      //       return updatedSession
      //     }))
      //   }
      // }
    })
    rws.addEventListener('open', () => {
      // if we need to send any messages on connect, do it here
    })
    return () => rws.close()
  }, [
    account.user?.token,
  ])

  const contextValue = useMemo<ISessionContext>(() => ({
    id,
    data,
    setID,
    reload,
  }), [
    id,
    data,
    setID,
    reload,
  ])

  return contextValue
}

export const SessionContextProvider: FC = ({ children }) => {
  const value = useSessionContext()
  return (
    <SessionContext.Provider value={ value }>
      { children }
    </SessionContext.Provider>
  )
}