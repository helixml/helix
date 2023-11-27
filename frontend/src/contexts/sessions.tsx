import React, { FC, useEffect, createContext, useMemo, useState, useCallback } from 'react'
import ReconnectingWebSocket from 'reconnecting-websocket'
import useApi from '../hooks/useApi'
import useAccount from '../hooks/useAccount'

import {
  ISession,
  ISessionMetaUpdate,
  IWebsocketEvent,
  WEBSOCKET_EVENT_TYPE_SESSION_UPDATE,
  WEBSOCKET_EVENT_TYPE_WORKER_TASK_RESPONSE,
  WORKER_TASK_RESPONSE_TYPE_PROGRESS,
  WORKER_TASK_RESPONSE_TYPE_STREAM,
  SESSION_CREATOR_SYSTEM,
} from '../types'

export interface ISessionsContext {
  initialized: boolean,
  sessions: ISession[],
  loadSessions: () => void,
  addSesssion: (session: ISession) => void,
  deleteSession: (id: string) => Promise<boolean>,
  renameSession: (id: string, name: string) => Promise<boolean>,
}

export const SessionsContext = createContext<ISessionsContext>({
  initialized: false,
  sessions: [],
  loadSessions: () => {},
  addSesssion: (session: ISession) => {},
  deleteSession: (id: string) => Promise.resolve(false),
  renameSession: (id: string, name: string) => Promise.resolve(false),
})

export const useSessionsContext = (): ISessionsContext => {
  const api = useApi()
  const account = useAccount()
  const [ initialized, setInitialized ] = useState(false)
  const [ sessions, setSessions ] = useState<ISession[]>([])

  const loadSessions = useCallback(async () => {
    const result = await api.get<ISession[]>('/api/v1/sessions')
    if(!result) return
    setSessions(result)
  }, [])

  const deleteSession = useCallback(async (id: string): Promise<boolean> => {
    const result = await api.delete<ISession>(`/api/v1/sessions/${id}`, {}, {
      loading: true,
    })
    if(!result) return false
    await loadSessions()
    return true
  }, [])

  const renameSession = useCallback(async (id: string, name: string): Promise<boolean> => {
    const result = await api.put<ISessionMetaUpdate, ISession>(`/api/v1/sessions/${id}/meta`, {
      id,
      name,
    }, {}, {
      loading: true,
    })
    if(!result) return false
    await loadSessions()
    return true
  }, [])

  const addSesssion = useCallback((session: ISession) => {
    setSessions(sessions => [session].concat(sessions))
  }, [])

  const initialize = useCallback(async () => {
    await loadSessions()
    setInitialized(true)
  }, [
    loadSessions,
  ])

  useEffect(() => {
    if(!account.user) return
    initialize()
  }, [
    account.user,
  ])

  useEffect(() => {
    if(!account.user?.token) return
    const wsProtocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const wsHostname = window.location.hostname
    const url = `${wsProtocol}//${wsHostname}/api/v1/ws/user?access_token=${account.user?.token}`
    const rws = new ReconnectingWebSocket(url)
    rws.addEventListener('message', (event) => {
      const parsedData = JSON.parse(event.data) as IWebsocketEvent
      // we have a session update message
      if(parsedData.type === WEBSOCKET_EVENT_TYPE_SESSION_UPDATE && parsedData.session) {
        // console.log("got new session from backend over websocket!")
        const newSession: ISession = parsedData.session
        // console.log(JSON.stringify(newSession, null, 4))
        setSessions(sessions => sessions.map(existingSession => {
          if(existingSession.id === newSession.id) return newSession
          return existingSession
        }))
      } else if(parsedData.type == WEBSOCKET_EVENT_TYPE_WORKER_TASK_RESPONSE && parsedData.worker_task_response) {
        const workerResponse = parsedData.worker_task_response
        // console.log("got new workerResponse from backend over websocket!")
        // console.log(JSON.stringify(workerResponse, null, 4))
        if(workerResponse.type == WORKER_TASK_RESPONSE_TYPE_STREAM) {
          setSessions(sessions => sessions.map(existingSession => {
            if(existingSession.id != workerResponse.session_id) return existingSession
            const systemInteractions = existingSession.interactions.filter(i => i.creator == SESSION_CREATOR_SYSTEM)
            if(systemInteractions.length <= 0) return existingSession
            const lastSystemInteraction = systemInteractions[systemInteractions.length - 1]
            const newInteractions = existingSession.interactions.map(i => {
              if(i.id != lastSystemInteraction.id) return i
              return Object.assign({}, i, {
                message: i.message + workerResponse.message,
              })
            })
            const updatedSession = Object.assign({}, existingSession, {
              interactions: newInteractions,
            })
            return updatedSession
          }))
        } else if(workerResponse.type == WORKER_TASK_RESPONSE_TYPE_PROGRESS) {
          setSessions(sessions => sessions.map(existingSession => {
            if(existingSession.id != workerResponse.session_id) return existingSession
            const systemInteractions = existingSession.interactions.filter(i => i.creator == SESSION_CREATOR_SYSTEM)
            if(systemInteractions.length <= 0) return existingSession
            const lastSystemInteraction = systemInteractions[systemInteractions.length - 1]
            const newInteractions = existingSession.interactions.map(i => {
              if(i.id != lastSystemInteraction.id) return i
              return Object.assign({}, i, {
                progress: workerResponse.progress,
                status: workerResponse.status,
              })
            })
            const updatedSession = Object.assign({}, existingSession, {
              interactions: newInteractions,
            })
            return updatedSession
          }))
        }
      }
    })
    rws.addEventListener('open', () => {
      // if we need to send any messages on connect, do it here
    })
    return () => rws.close()
  }, [
    account.user?.token,
  ])

  const contextValue = useMemo<ISessionsContext>(() => ({
    initialized,
    sessions,
    loadSessions,
    addSesssion,
    deleteSession,
    renameSession,
  }), [
    initialized,
    sessions,
    loadSessions,
    addSesssion,
    deleteSession,
    renameSession,
  ])

  return contextValue
}

export const SessionsContextProvider: FC = ({ children }) => {
  const value = useSessionsContext()
  return (
    <SessionsContext.Provider value={ value }>
      { children }
    </SessionsContext.Provider>
  )
}