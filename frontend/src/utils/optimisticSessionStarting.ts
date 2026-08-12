import type { QueryClient } from '@tanstack/react-query'
import type { TypesSession } from '../api/api'
import { GET_SESSION_QUERY_KEY } from '../services/sessionService'

// Variants of the session query key that useGetSession actually creates.
// useGetSession suffixes the key with 'full' or 'skip' depending on the
// skipInteractions option. setQueryData requires an exact match (unlike
// invalidateQueries, which prefix-matches), so we have to write to both.
const QUERY_VARIANTS = ['full', 'skip'] as const

// Synchronously flip the cached session config to external_agent_status="starting"
// when the user submits a chat to a paused desktop. The next useSandboxState 3s
// poll reconciles to the authoritative backend value — by which point the
// backend's synchronous syncPromptHistory mark has already written "starting"
// to the DB row, so the optimistic patch and the refetch agree.
//
// We deliberately do NOT call invalidateQueries here: an immediate refetch
// races the asynchronous wake goroutine on the backend and overwrites the
// optimistic write with a still-stale "stopped" row, causing the spinner to
// flicker off before the 3s poll catches up. See spec
// 002047_yet-again-sending-a/design.md for the full race analysis.
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
}
