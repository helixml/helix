import React, { FC, useEffect, createContext, useMemo, useState, useCallback } from 'react'
import useApi from '../hooks/useApi'
import useAccount from '../hooks/useAccount'

import {
  ISession,
  ISessionSummary,
  ISessionMetaUpdate,
} from '../types'

import {
  SESSION_PAGINATION_PAGE_LIMIT,
} from '../constants'

import {
  getSessionSummary,
} from '../utils/session'

export interface ISessionsContext {
  initialized: boolean,
  sessions: ISessionSummary[],
  loadSessions: (offset: number, limit: number) => void,
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

  const [ offset, setOffset ] = useState(0)
  const [ limit, setLimit ] = useState(SESSION_PAGINATION_PAGE_LIMIT)
  const [ initialized, setInitialized ] = useState(false)
  const [ sessions, setSessions ] = useState<ISessionSummary[]>([])

  const loadSessions = useCallback(async (offset: number, limit: number) => {
    const result = await api.get<ISessionSummary[]>('/api/v1/sessions')
    if(!result) return
    setSessions(result)
  }, [])

  const reloadSessions = useCallback(async () => {
    await loadSessions(offset, limit)
  }, [
    offset,
    limit,
    loadSessions,
  ])

  const deleteSession = useCallback(async (id: string): Promise<boolean> => {
    const result = await api.delete<ISession>(`/api/v1/sessions/${id}`)
    if(!result) return false
    await reloadSessions()
    return true
  }, [
    reloadSessions,
  ])

  const renameSession = useCallback(async (id: string, name: string): Promise<boolean> => {
    const result = await api.put<ISessionMetaUpdate, ISession>(`/api/v1/sessions/${id}/meta`, {
      id,
      name,
    })
    if(!result) return false
    await reloadSessions()
    return true
  }, [
    reloadSessions,
  ])

  const addSesssion = useCallback((session: ISession) => {
    const summary = getSessionSummary(session)
    setSessions(sessions => [summary].concat(sessions))
  }, [])

  useEffect(() => {
    if(!account.user) return
    reloadSessions()
    if(!initialized) {
      setInitialized(true)
    }
  }, [
    account.user,
    reloadSessions,
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