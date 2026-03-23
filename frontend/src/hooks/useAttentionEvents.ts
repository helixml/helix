import { useCallback, useRef } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import useApi from './useApi'

export interface AttentionEvent {
  id: string
  user_id: string
  organization_id: string
  project_id: string
  spec_task_id: string
  event_type: AttentionEventType
  title: string
  description?: string
  created_at: string
  acknowledged_at?: string | null
  dismissed_at?: string | null
  snoozed_until?: string | null
  idempotency_key?: string
  metadata?: Record<string, unknown>
  project_name?: string
  spec_task_name?: string
}

export type AttentionEventType =
  | 'specs_pushed'
  | 'agent_interaction_completed'
  | 'spec_failed'
  | 'implementation_failed'
  | 'pr_ready'

const QUERY_KEY = ['attention-events']

export function useAttentionEvents(enabled: boolean = true) {
  const api = useApi()
  const queryClient = useQueryClient()
  // Track previous event IDs so consumers can detect genuinely new arrivals
  const prevEventIdsRef = useRef<Set<string>>(new Set())

  const query = useQuery<AttentionEvent[]>({
    queryKey: QUERY_KEY,
    queryFn: async () => {
      const events = await api.get<AttentionEvent[]>('/api/v1/attention-events?active=true', undefined, {
        snackbar: false,
      })
      return events || []
    },
    enabled,
    refetchInterval: 10_000,
  })

  // Compute new (unacknowledged) events that appeared since last render cycle
  const newEvents: AttentionEvent[] = []
  if (query.data) {
    for (const event of query.data) {
      if (!event.acknowledged_at && !prevEventIdsRef.current.has(event.id)) {
        newEvents.push(event)
      }
    }
    // Update the ref so next cycle only detects truly new ones
    const currentIds = new Set(query.data.map((e) => e.id))
    prevEventIdsRef.current = currentIds
  }

  const invalidate = useCallback(() => {
    queryClient.invalidateQueries({ queryKey: QUERY_KEY })
  }, [queryClient])

  const acknowledgeMutation = useMutation({
    mutationFn: async (eventId: string) => {
      await api.put(`/api/v1/attention-events/${eventId}`, { acknowledge: true }, undefined, {
        snackbar: false,
      })
    },
    onSuccess: invalidate,
  })

  const dismissMutation = useMutation({
    mutationFn: async (eventId: string) => {
      await api.put(`/api/v1/attention-events/${eventId}`, { dismiss: true }, undefined, {
        snackbar: false,
      })
    },
    onSuccess: invalidate,
  })

  const snoozeMutation = useMutation({
    mutationFn: async ({ eventId, until }: { eventId: string; until: Date }) => {
      await api.put(
        `/api/v1/attention-events/${eventId}`,
        { snoozed_until: until.toISOString() },
        undefined,
        { snackbar: false },
      )
    },
    onSuccess: invalidate,
  })

  const dismissAllMutation = useMutation({
    mutationFn: async () => {
      await api.post('/api/v1/attention-events/dismiss-all', {}, undefined, {
        snackbar: false,
      })
    },
    onSuccess: invalidate,
  })

  const acknowledge = useCallback(
    (eventId: string) => acknowledgeMutation.mutate(eventId),
    [acknowledgeMutation],
  )

  const dismiss = useCallback(
    (eventId: string) => dismissMutation.mutate(eventId),
    [dismissMutation],
  )

  const snooze = useCallback(
    (eventId: string, durationMs: number = 60 * 60 * 1000) => {
      snoozeMutation.mutate({ eventId, until: new Date(Date.now() + durationMs) })
    },
    [snoozeMutation],
  )

  const dismissAll = useCallback(
    () => dismissAllMutation.mutate(),
    [dismissAllMutation],
  )

  return {
    events: query.data || [],
    newEvents,
    isLoading: query.isLoading,
    totalCount: query.data?.length || 0,
    unreadCount: (query.data ?? []).filter(e => !e.acknowledged_at).length,
    hasNew: (query.data ?? []).some(e => !e.acknowledged_at),
    acknowledge,
    dismiss,
    snooze,
    dismissAll,
    refetch: invalidate,
  }
}