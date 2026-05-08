import type { QueryClient } from '@tanstack/react-query'
import type { TypesSession } from '../api/api'
import { GET_SESSION_QUERY_KEY } from '../services/sessionService'

// Variants of the session query key that useGetSession actually creates.
// useGetSession suffixes the key with 'full' or 'skip' depending on the
// skipInteractions option. setQueryData requires an exact match (unlike
// invalidateQueries, which prefix-matches), so we have to write to both.
const QUERY_VARIANTS = ['full', 'skip'] as const

// Synchronously flip the cached session config to external_agent_status="starting"
// when the user submits a chat to a paused desktop. The next 3s poll brings the
// authoritative backend value, harmlessly overwriting this optimistic state to
// "starting" (boot in flight), "running" (already up), or "absent" (failed).
//
// No-op when the cache already shows "starting" or "running" (avoids stomping
// fresher state from a poll that landed between the user's keystrokes and the
// click). Also no-op when the cache is empty for both variants.
export function optimisticallyMarkSessionStarting(
  queryClient: QueryClient,
  sessionId: string,
): void {
  if (!sessionId) return
  for (const variant of QUERY_VARIANTS) {
    queryClient.setQueryData(
      [...GET_SESSION_QUERY_KEY(sessionId), variant],
      (old: { data?: TypesSession } | undefined) => {
        if (!old?.data) return old
        const cfg = old.data.config ?? {}
        if (
          cfg.external_agent_status === 'running' ||
          cfg.external_agent_status === 'starting'
        ) {
          return old
        }
        return {
          ...old,
          data: {
            ...old.data,
            config: {
              ...cfg,
              external_agent_status: 'starting',
              status_message: cfg.status_message || 'Starting Desktop...',
            },
          },
        }
      },
    )
  }
  // Belt-and-braces: a prefix-matching invalidate kicks the next poll a bit
  // earlier than waiting for the 3s tick. The optimistic state above already
  // shows the spinner; this just shortens the time until it's confirmed.
  queryClient.invalidateQueries({ queryKey: GET_SESSION_QUERY_KEY(sessionId) })
}
