import React, { useEffect, createContext, useMemo, useState, useCallback, ReactNode } from 'react'
import useApi from '../hooks/useApi'
import useAccount from '../hooks/useAccount'

import {
  // ISession,
  ISessionSummary,
  ISessionMetaUpdate,
  ISessionsList,
  IPaginationState,
  SESSION_PAGINATION_PAGE_LIMIT,
} from '../types'

import { TypesSession, TypesSessionSummary, TypesSessionsList } from '../api/api'

import {
  getSessionSummary,
} from '../utils/session'

export interface ISessionsContext {
  initialized: boolean,
  loading: boolean,
  pagination: IPaginationState,
  sessions: TypesSessionSummary[],
  hasMoreSessions: boolean,
  advancePage: () => void,
  loadSessions: (query?: ISessionsQuery) => Promise<void>,
  addSesssion: (session: TypesSession) => void,
  deleteSession: (id: string) => Promise<boolean>,
  renameSession: (id: string, name: string) => Promise<boolean>,
}

export interface ISessionsQuery {
  org_id?: string,
  search_filter?: string,
}

export const SessionsContext = createContext<ISessionsContext>({
  initialized: false,
  loading: false,
  pagination: {
    total: 0,
    limit: 0,
    offset: 0,
  },
  hasMoreSessions: true,
  sessions: [],
  advancePage: () => {},
  loadSessions: async () => {},
  addSesssion: (session: TypesSession) => {},
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
  const [ sessions, setSessions ] = useState<TypesSessionSummary[]>([])

  const loadSessions = useCallback(async (query: ISessionsQuery = {}) => {
    // default query params
    const params: Record<string, any> = {
      offset: (page - 1) * SESSION_PAGINATION_PAGE_LIMIT,
      limit: SESSION_PAGINATION_PAGE_LIMIT,
    }

    // if we have a search filter - apply it
    if(query.search_filter) {
      params.search_filter = query.search_filter
    }

    // Determine the organization_id parameter value
    if (query.org_id) {
      // If specific org_id is provided, use it
      params.organization_id = query.org_id;
    } else if (account.organizationTools.organization) {
      // If we're in an org context, use the org ID
      params.organization_id = account.organizationTools.organization.id;
    }

    setLoading(true)
    const result = await api.get<TypesSessionsList>('/api/v1/sessions', {
      params,
    })
    setLoading(false)
    if(!result) return
    setSessions(result.sessions || [])
    setPagination({
      total: result.counter?.count || 0,
      limit: SESSION_PAGINATION_PAGE_LIMIT,
      offset: (page - 1) * SESSION_PAGINATION_PAGE_LIMIT,
    })
  }, [
    api,
    setLoading,
    page,
    account.organizationTools.orgID,
    account.organizationTools.organization,
  ])

  const hasMoreSessions = useMemo(() => {
    return sessions.length < pagination.total
  }, [
    sessions,
    pagination,
  ])

  // increase the limit whilst anchoring the offset
  // means we scroll down adding more sessions as we scroll
  const advancePage = useCallback(() => {
    setPage(page => page + 1)
  }, [])

  const deleteSession = useCallback(async (id: string): Promise<boolean> => {
    const result = await api.delete<TypesSession>(`/api/v1/sessions/${id}`)
    if(!result) return false
    await loadSessions()
    return true
  }, [
    page,
    loadSessions,
  ])

  const renameSession = useCallback(async (id: string, name: string): Promise<boolean> => {
    const result = await api.put<ISessionMetaUpdate, TypesSession>(`/api/v1/sessions/${id}/meta`, {
      id,
      name,
    })
    
    if(!result) return false
    await loadSessions()
    return true
  }, [
    loadSessions,
  ])

  const addSesssion = useCallback((session: TypesSession) => {
    const summary = getSessionSummary(session)
    setSessions(sessions => [summary as unknown as TypesSessionSummary].concat(sessions))
  }, [])

  useEffect(() => {
    if(!account.user) return
    // we wait until we have loaded the organization before we load the apps
    if(account.organizationTools.orgID && !account.organizationTools.organization) return
    loadSessions({
      org_id: account.organizationTools.organization?.id || '',
    })
    if(!initialized) {
      setInitialized(true)
    }
  }, [
    account.user,
    account.organizationTools.orgID,
    account.organizationTools.organization,
  ])

  return {
    initialized,
    loading,
    pagination,
    sessions,
    hasMoreSessions,
    advancePage,
    loadSessions,
    addSesssion,
    deleteSession,
    renameSession,
  }
}

export const SessionsContextProvider = ({ children }: { children: ReactNode }) => {
  const value = useSessionsContext()
  return (
    <SessionsContext.Provider value={ value }>
      { children }
    </SessionsContext.Provider>
  )
}