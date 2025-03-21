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
  hasMoreSessions: boolean,
  advancePage: () => void,
  loadSessions: (query?: ISessionsQuery) => Promise<void>,
  loadSessionsIfChanged: (query?: ISessionsQuery) => Promise<void>,
  addSesssion: (session: ISession) => void,
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
  loadSessionsIfChanged: async () => {},
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
    const result = await api.get<ISessionsList>('/api/v1/sessions', {
      params,
    })
    setLoading(false)
    if(!result) return
    setSessions(result.sessions || [])
    setPagination({
      total: result.counter.count,
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

  const loadSessionsIfChanged = useCallback(async (query: ISessionsQuery = {}) => {
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

    // Don't set loading state to avoid UI flicker
    const result = await api.get<ISessionsList>('/api/v1/sessions', {
      params,
    })
    
    if(!result) return
    
    // Check if the data has actually changed before updating state
    const newSessions = result.sessions || [];
    const currentIds = new Set(sessions.map(s => s.session_id));
    const newIds = new Set(newSessions.map(s => s.session_id));
    
    const hasChanges = 
      newSessions.length !== sessions.length || 
      newSessions.some(newSession => {
        // Check if this session exists in the current set
        if (!currentIds.has(newSession.session_id)) return true;
        
        // Find the current version to compare
        const current = sessions.find(s => s.session_id === newSession.session_id);
        
        // Compare relevant fields that would affect display
        return current?.name !== newSession.name || 
               current?.updated !== newSession.updated ||
               current?.interaction_id !== newSession.interaction_id;
      });
    
    // Only update state if there are actual changes
    if (hasChanges) {
      console.log('banana: Sessions data changed, updating state');
      setSessions(newSessions);
      setPagination({
        total: result.counter.count,
        limit: SESSION_PAGINATION_PAGE_LIMIT,
        offset: (page - 1) * SESSION_PAGINATION_PAGE_LIMIT,
      });
    } else {
      console.log('banana: Sessions data unchanged, skipping update');
    }
  }, [
    api,
    page,
    sessions,
    account.organizationTools.orgID,
    account.organizationTools.organization,
  ]);

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
    loadSessionsIfChanged,
    addSesssion,
    deleteSession,
    renameSession,
  }
}

export const SessionsContextProvider: FC = ({ children }) => {
  const value = useSessionsContext()
  return (
    <SessionsContext.Provider value={ value }>
      { children }
    </SessionsContext.Provider>
  )
}