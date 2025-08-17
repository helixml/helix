import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import useApi from '../hooks/useApi';
import { TypesSession, TypesSessionSummary } from '../api/api';
import { useState, useEffect, useCallback } from 'react';

export const sessionStepsQueryKey = (id: string) => [
  "session-steps",
  id
];

export const getSessionQueryKey = (id: string) => [
  "session",
  id
];

export const listSessionsQueryKey = (orgId?: string, page?: number, pageSize?: number, search?: string) => [
  "sessions",
  orgId,
  page,
  pageSize,
  search
];

// useListSessionSteps returns the steps for a session, it includes
// steps for all interactions in the session
export function useListSessionSteps(sessionId: string, options?: { enabled?: boolean }) {
  const api = useApi()
  const apiClient = api.getApiClient()  

  return useQuery({
    queryKey: sessionStepsQueryKey(sessionId),
    queryFn: () => apiClient.v1SessionsStepInfoDetail(sessionId),
    enabled: options?.enabled ?? true
  })
}

export function useGetSession(sessionId: string, options?: { enabled?: boolean }) {
  const api = useApi()
  const apiClient = api.getApiClient()

  return useQuery({
    queryKey: getSessionQueryKey(sessionId),
    queryFn: () => apiClient.v1SessionsDetail(sessionId),
    enabled: options?.enabled ?? true
  })
}

export function useListSessions(orgId?: string, search?: string, page?: number, pageSize?: number, options?: { enabled?: boolean }) {
  const api = useApi()
  const apiClient = api.getApiClient()
  
  return useQuery({
    queryKey: listSessionsQueryKey(orgId, page ?? 0, pageSize ?? 0, search ?? ''),
    queryFn: () => apiClient.v1SessionsList({
      org_id: orgId,
      search: search,
      page: page,
      page_size: pageSize,
    }),
    enabled: options?.enabled ?? true
  })
}

export function useUpdateSession(sessionId: string, options?: { enabled?: boolean }) {
  const api = useApi()
  const apiClient = api.getApiClient()
  const queryClient = useQueryClient()
  
  return useMutation({
    mutationFn: (sessionData: TypesSession) => apiClient.v1SessionsUpdate(sessionId, sessionData),
    onSuccess: (updatedSession) => {
      // Invalidate the specific session query to refresh the session data
      queryClient.invalidateQueries({ queryKey: getSessionQueryKey(sessionId) })
      // Invalidate all sessions queries to refresh the list after update
      queryClient.invalidateQueries({ queryKey: ["sessions"] })
      // Optionally update the cache directly with the new data
      queryClient.setQueryData(getSessionQueryKey(sessionId), updatedSession)
    }
  })
}


export function useDeleteSession(sessionId: string, options?: { enabled?: boolean }) {
  const api = useApi()
  const apiClient = api.getApiClient()
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: () => apiClient.v1SessionsDelete(sessionId),
    onSuccess: () => {
      // Invalidate all sessions queries to refresh the list after deletion
      queryClient.invalidateQueries({ queryKey: ["sessions"] })
    }
  })
}

export function invalidateSessionsQuery() {
  const queryClient = useQueryClient()
  queryClient.invalidateQueries({ queryKey: ["sessions"] })
}
