/**
 * SessionPromptQueue - read-only view of a session's pending prompt queue.
 *
 * Surfaces the session-scoped prompt queue (org-chat / bot sessions that have no
 * spec task) so you can see what's queued for the agent. Automated / org-graph
 * dispatched messages are enqueued onto this same queue by the backend
 * (enqueueAgentMessage), deferred until the agent is idle; this shows them while
 * they wait and clears them once delivered.
 *
 * Read-only by design — full queue management (reorder / delete / interrupt
 * toggle) lives in RobustPromptInput for spec-task pages; parity here can be a
 * follow-up.
 */
import React from 'react'
import { Box, Chip, Stack, Typography } from '@mui/material'
import ScheduleIcon from '@mui/icons-material/Schedule'
import { useQuery } from '@tanstack/react-query'
import useApi from '../../hooks/useApi'
import { listSessionPromptHistory } from '../../services/promptHistoryService'
import { TypesPromptHistoryEntry } from '../../api/api'

interface SessionPromptQueueProps {
  sessionId: string
}

// Only these statuses are "in the queue" (not yet delivered/streaming to the
// agent). 'sending' is included so a just-dispatched-but-not-acknowledged prompt
// stays visible until the agent actually starts streaming — matching the
// spec-task queue's semantics.
const QUEUED_STATUSES = new Set(['pending', 'sending'])

const SessionPromptQueue: React.FC<SessionPromptQueueProps> = ({ sessionId }) => {
  const api = useApi()
  const apiClient = api.getApiClient()

  const { data } = useQuery({
    queryKey: ['session-prompt-queue', sessionId],
    enabled: !!sessionId,
    // Poll while open so queued items appear/clear promptly (mirrors the
    // spec-task queue's 2s poll cadence).
    refetchInterval: 2000,
    queryFn: async () => listSessionPromptHistory(apiClient, sessionId),
  })

  const queued: TypesPromptHistoryEntry[] = (data?.entries || []).filter(
    (e) => e.status && QUEUED_STATUSES.has(e.status),
  )

  if (queued.length === 0) return null

  return (
    <Box
      sx={{
        px: 1.5,
        py: 1,
        borderBottom: (theme) =>
          `1px solid ${theme.palette.mode === 'light' ? 'rgba(0,0,0,0.08)' : 'rgba(255,255,255,0.08)'}`,
      }}
    >
      <Stack direction="row" alignItems="center" spacing={1} sx={{ mb: 0.5 }}>
        <ScheduleIcon sx={{ fontSize: 16, color: 'text.secondary' }} />
        <Typography variant="caption" color="text.secondary">
          {queued.length} queued — delivered when the agent is idle
        </Typography>
      </Stack>
      <Stack spacing={0.5}>
        {queued.map((e) => (
          <Stack key={e.id} direction="row" alignItems="center" spacing={1}>
            <Chip
              label={e.status}
              size="small"
              sx={{ height: 18, fontSize: '0.65rem' }}
              color={e.status === 'sending' ? 'primary' : 'default'}
            />
            {e.interrupt ? (
              <Chip
                label="interrupt"
                size="small"
                color="warning"
                sx={{ height: 18, fontSize: '0.65rem' }}
              />
            ) : null}
            <Typography
              variant="body2"
              color="text.secondary"
              sx={{
                overflow: 'hidden',
                textOverflow: 'ellipsis',
                whiteSpace: 'nowrap',
              }}
            >
              {e.content}
            </Typography>
          </Stack>
        ))}
      </Stack>
    </Box>
  )
}

export default SessionPromptQueue
