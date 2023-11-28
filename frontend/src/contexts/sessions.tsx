import React, { FC, useEffect, createContext, useMemo, useState, useCallback } from 'react'
import ReconnectingWebSocket from 'reconnecting-websocket'
import useApi from '../hooks/useApi'
import useAccount from '../hooks/useAccount'

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

export interface ISessionsContext {
  initialized: boolean,
  sessions: ISessionSummary[],
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
  const [ sessions, setSessions ] = useState<ISessionSummary[]>([])

  const loadSessions = useCallback(async () => {
    const result = await api.get<ISessionSummary[]>('/api/v1/sessions')
    if(!result) return
    setSessions(result)
  }, [])

  const deleteSession = useCallback(async (id: string): Promise<boolean> => {
    const result = await api.delete<ISession>(`/api/v1/sessions/${id}`)
    if(!result) return false
    await loadSessions()
    return true
  }, [])

  const renameSession = useCallback(async (id: string, name: string): Promise<boolean> => {
    const result = await api.put<ISessionMetaUpdate, ISession>(`/api/v1/sessions/${id}/meta`, {
      id,
      name,
    })
    if(!result) return false
    await loadSessions()
    return true
  }, [])

  const addSesssion = useCallback((session: ISession) => {
    const summary = getSessionSummary(session)
    setSessions(sessions => [summary].concat(sessions))
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