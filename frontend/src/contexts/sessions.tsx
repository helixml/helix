import React, { FC, useEffect, createContext, useMemo, useState, useCallback } from 'react'
import ReconnectingWebSocket from 'reconnecting-websocket'
import useApi from '../hooks/useApi'
import useAccount from '../hooks/useAccount'

import {
  ISession,
  IWebsocketEvent,
  WEBSOCKET_EVENT_TYPE_SESSION_UPDATE,
  WEBSOCKET_EVENT_TYPE_WORKER_TASK_RESPONSE,
} from '../types'

export interface ISessionsContext {
  initialized: boolean,
  sessions: ISession[],
  loadSessions: () => void,
}

export const SessionsContext = createContext<ISessionsContext>({
  initialized: false,
  sessions: [],
  loadSessions: () => {},
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
        console.log("got new session from backend over websocket!")
        const newSession: ISession = parsedData.session
        console.log(JSON.stringify(newSession, null, 4))
        setSessions(sessions => sessions.map(existingSession => {
          if(existingSession.id === newSession.id) return newSession
          return existingSession
        }))
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
  }), [
    initialized,
    sessions,
    loadSessions,
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