/**
 * SessionPromptQueue - the single queue view for a session that has no spec
 * task (org-chat / bot sessions). It is session-keyed and DB-backed, so it is
 * the authoritative queue: it surfaces prompts still waiting for the agent
 * (pending/sending) AND failed prompts, including the ones auto-dispatched by
 * the org graph (enqueueAgentMessage). Spec-task pages use RobustPromptInput's
 * own backend-backed queue instead; plain sessions use this one.
 *
 * Failed prompts are classified (via classifyPromptQueueEntry, shared with
 * RobustPromptInput) so a wedged/crashed agent surfaces the same Restart
 * affordance here as on spec-task pages.
 */
import React, { useState } from 'react'
import { Box, Button, Chip, Stack, Typography } from '@mui/material'
import ScheduleIcon from '@mui/icons-material/Schedule'
import RestartAltIcon from '@mui/icons-material/RestartAlt'
import { useQuery } from '@tanstack/react-query'
import useApi from '../../hooks/useApi'
import { listSessionPromptHistory } from '../../services/promptHistoryService'
import { TypesPromptHistoryEntry } from '../../api/api'
import { classifyPromptQueueEntry } from '../../utils/promptQueueStatus'

interface SessionPromptQueueProps {
  sessionId: string
}

// 'pending'/'sending' are still-in-flight; 'failed' is a stalled/errored prompt
// that needs surfacing (with Restart when the agent is wedged).
const VISIBLE_STATUSES = new Set(['pending', 'sending', 'failed'])

const SessionPromptQueue: React.FC<SessionPromptQueueProps> = ({ sessionId }) => {
  const api = useApi()
  const apiClient = api.getApiClient()
  const [isRestarting, setIsRestarting] = useState(false)

  const { data } = useQuery({
    queryKey: ['session-prompt-queue', sessionId],
    enabled: !!sessionId,
    // Poll while open so queued items appear/clear promptly (mirrors the
    // spec-task queue's 2s poll cadence).
    refetchInterval: 2000,
    queryFn: async () => listSessionPromptHistory(apiClient, sessionId),
  })

  const visible: TypesPromptHistoryEntry[] = (data?.entries || []).filter(
    (e) => e.status && VISIBLE_STATUSES.has(e.status),
  )

  if (visible.length === 0) return null

  const handleRestart = () => {
    if (!apiClient || !sessionId || isRestarting) return
    setIsRestarting(true)
    apiClient
      .v1SessionsRestartAgentCreate(sessionId)
      .catch((err: unknown) => console.error('Failed to restart agent thread:', err))
      .finally(() => setIsRestarting(false))
  }

  const queuedCount = visible.filter((e) => e.status !== 'failed').length

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
          {queuedCount > 0
            ? `${queuedCount} queued — delivered when the agent is idle`
            : 'Prompt queue'}
        </Typography>
      </Stack>
      <Stack spacing={0.5}>
        {visible.map((e) => {
          const status = classifyPromptQueueEntry({
            status: e.status,
            errorMessage: e.error_message,
            nextRetryAtMs: e.next_retry_at ? Date.parse(e.next_retry_at) : undefined,
            retryCount: e.retry_count,
          })
          const isFailed = status.isFailed
          return (
            <Stack key={e.id} spacing={0.5}>
              <Stack direction="row" alignItems="center" spacing={1}>
                <Chip
                  label={e.status}
                  size="small"
                  sx={{ height: 18, fontSize: '0.65rem' }}
                  color={
                    isFailed
                      ? status.showRestart
                        ? 'error'
                        : 'warning'
                      : e.status === 'sending'
                        ? 'primary'
                        : 'default'
                  }
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
                  sx={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}
                >
                  {e.content}
                </Typography>
              </Stack>
              {isFailed && (
                <Stack direction="row" alignItems="center" spacing={1} sx={{ pl: 0.5 }}>
                  <Typography
                    variant="caption"
                    sx={{
                      color: status.showRestart ? 'error.main' : 'warning.main',
                      fontWeight: status.showRestart ? 600 : 'inherit',
                    }}
                  >
                    {status.isCrashed
                      ? 'Agent crashed. Click Restart to recover.'
                      : status.isStuckTransient
                        ? "Agent isn't responding — click Restart to recover."
                        : 'Failed — retrying…'}
                  </Typography>
                  {status.showRestart && (
                    <Button
                      size="small"
                      variant="outlined"
                      color="error"
                      startIcon={<RestartAltIcon sx={{ fontSize: 16 }} />}
                      disabled={isRestarting}
                      onClick={handleRestart}
                      sx={{ py: 0, minHeight: 24, fontSize: '0.7rem', textTransform: 'none' }}
                    >
                      {isRestarting ? 'Restarting…' : 'Restart'}
                    </Button>
                  )}
                </Stack>
              )}
            </Stack>
          )
        })}
      </Stack>
    </Box>
  )
}

export default SessionPromptQueue
