import React, { FC, useEffect, createContext, useMemo, useState, useCallback } from 'react'
import useApi from '../hooks/useApi'
import useAccount from '../hooks/useAccount'

import {
  ISession,
  ISessionSummary,
  ISessionMetaUpdate,
  ISessionsList,
  IPaginationState,
  SESSION_PAGINATION_PAGE_LIMIT,
} from '../types'

import {
  getSessionSummary,
} from '../utils/session'

export interface ISessionsContext {
  initialized: boolean,
  loading: boolean,
  pagination: IPaginationState,
  sessions: ISessionSummary[],
  advancePage: () => void,
  loadSessions: () => void,
  addSesssion: (session: ISession) => void,
  deleteSession: (id: string) => Promise<boolean>,
  renameSession: (id: string, name: string) => Promise<boolean>,
}

export const SessionsContext = createContext<ISessionsContext>({
  initialized: false,
  loading: false,
  pagination: {
    total: 0,
    limit: 0,
    offset: 0,
  },
  sessions: [],
  advancePage: () => {},
  loadSessions: () => {},
  addSesssion: (session: ISession) => {},
  deleteSession: (id: string) => Promise.resolve(false),
  renameSession: (id: string, name: string) => Promise.resolve(false),
})

export const useSessionsContext = (): ISessionsContext => {
  const api = useApi()
  const account = useAccount()

  // in this model - offset is always zero - i.e. we are not paginating
  // just increasing the limit as we scroll down
  const [ loading, setLoading ] = useState(false)
  const [ page, setPage ] = useState(1)
  const [ pagination, setPagination ] = useState<IPaginationState>({
    total: 0,
    limit: SESSION_PAGINATION_PAGE_LIMIT,
    offset: 0,
  })
  
  const [ initialized, setInitialized ] = useState(false)
  const [ sessions, setSessions ] = useState<ISessionSummary[]>([])

  const loadSessions = useCallback(async () => {
    const limit = page * SESSION_PAGINATION_PAGE_LIMIT
    // this means we have already loaded all the sessions
    if(limit > pagination.total && pagination.total > 0) return
    setLoading(true)
    const result = await api.get<ISessionsList>('/api/v1/sessions', {
      params: {
        limit,
        offset: 0,
      }
    })
    if(!result) {
      setLoading(false)
      return
    }
    setSessions(result.sessions)
    setPagination({
      total: result.counter.count,
      limit,
      offset: 0,
    })
    setLoading(false)
  }, [
    page,
    pagination,
  ])

  // increase the limit whilst anchoring the offset
  // means we scroll down adding more sessions as we scroll
  const advancePage = useCallback(() => {
    setPage(page => page + 1)
  }, [])

  const deleteSession = useCallback(async (id: string): Promise<boolean> => {
    const result = await api.delete<ISession>(`/api/v1/sessions/${id}`)
    if(!result) return false
    await loadSessions()
    return true
  }, [
    page,
    loadSessions,
  ])

  const renameSession = useCallback(async (id: string, name: string): Promise<boolean> => {
    const result = await api.put<ISessionMetaUpdate, ISession>(`/api/v1/sessions/${id}/meta`, {
      id,
      name,
    })
    if(!result) return false
    await loadSessions()
    return true
  }, [
    loadSessions,
  ])

  const addSesssion = useCallback((session: ISession) => {
    const summary = getSessionSummary(session)
    setSessions(sessions => [summary].concat(sessions))
  }, [])

  useEffect(() => {
    if(!account.user) return
    loadSessions()
    if(!initialized) {
      setInitialized(true)
    }
  }, [
    account.user,
    page,
  ])

  const contextValue = useMemo<ISessionsContext>(() => ({
    initialized,
    loading,
    pagination,
    sessions,
    advancePage,
    loadSessions,
    addSesssion,
    deleteSession,
    renameSession,
  }), [
    initialized,
    loading,
    pagination,
    sessions,
    advancePage,
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