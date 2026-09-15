/**
 * SessionPromptQueue - the queue panel for a session that has no spec task
 * (org agents, project chats). Same chrome and rows as the spec-task queue
 * inside RobustPromptInput — attached to the top of the composer — so the two
 * surfaces read identically; only the data source differs (session-keyed
 * prompt history via useSessionPromptQueue, which the org graph also feeds).
 *
 * Failed prompts are classified with classifyPromptQueueEntry, shared with
 * RobustPromptInput, so a wedged agent gets the same Restart affordance.
 */
import React, { useState } from 'react'
import { Box, Button, CircularProgress, IconButton, Tooltip, Typography } from '@mui/material'
import { alpha } from '@mui/material/styles'
import { CircleAlert, Hourglass, ListStart, RotateCcw, X, Zap } from 'lucide-react'

import { getChatColors } from './chatStyles'
import { classifyPromptQueueEntry } from '../../utils/promptQueueStatus'
import type { SessionPromptQueueEntry } from './useSessionPromptQueue'

interface SessionPromptQueueProps {
  sessionId: string
  entries: SessionPromptQueueEntry[]
  onRemove: (entryId: string) => Promise<unknown>
  onRestartAgent: () => Promise<unknown>
}

const firstLine = (content: string, maxLen = 60): string => {
  const line = content.split('\n')[0]
  return line.length <= maxLen ? line : `${line.substring(0, maxLen - 3)}...`
}

const SessionPromptQueue: React.FC<SessionPromptQueueProps> = ({ entries, onRemove, onRestartAgent }) => {
  const [isRestarting, setIsRestarting] = useState(false)
  if (entries.length === 0) return null

  const handleRestart = () => {
    if (isRestarting) return
    setIsRestarting(true)
    onRestartAgent()
      .catch((err: unknown) => console.error('Failed to restart agent thread:', err))
      .finally(() => setIsRestarting(false))
  }

  return (
    <Box
      sx={{
        borderRadius: '20px 20px 0 0',
        border: '1px solid',
        borderBottom: 0,
        borderColor: (theme) => getChatColors(theme).border,
        bgcolor: (theme) => getChatColors(theme).composerSurface,
        overflow: 'hidden',
      }}
    >
      <Box
        sx={{
          display: 'flex',
          alignItems: 'center',
          gap: 0.75,
          px: 2,
          pt: 1.25,
          pb: 0.75,
          color: (theme) => getChatColors(theme).subtle,
          borderBottom: '1px solid',
          borderColor: (theme) => getChatColors(theme).border,
        }}
      >
        <ListStart size={14} />
        <Typography variant="caption" sx={{ flex: 1, fontWeight: 500, letterSpacing: '0.01em' }}>
          {`${entries.length} queued`}
        </Typography>
      </Box>
      <Box sx={{ maxHeight: 200, overflowY: 'auto' }}>
        {entries.map((entry, index) => {
          const status = classifyPromptQueueEntry({
            status: entry.status,
            errorMessage: entry.error_message,
            nextRetryAtMs: entry.next_retry_at ? Date.parse(entry.next_retry_at) : undefined,
            retryCount: entry.retry_count,
          })
          const isFailed = status.isFailed
          const isTransient = status.isTransientFailure && !status.showRestart
          return (
            <Box
              key={entry.id}
              sx={{
                display: 'flex',
                alignItems: 'center',
                gap: 0.75,
                px: 1.5,
                py: 0.75,
                borderBottom: index < entries.length - 1 ? '1px solid' : 'none',
                borderColor: (theme) => getChatColors(theme).border,
                bgcolor: isFailed
                  ? (theme) => alpha(isTransient ? theme.palette.warning.main : theme.palette.error.main, 0.08)
                  : 'transparent',
                '&:hover': { bgcolor: (theme) => alpha(theme.palette.text.primary, 0.025) },
              }}
            >
              {isFailed ? (
                <CircleAlert size={16} style={{ flexShrink: 0, marginLeft: 20 }} />
              ) : (
                <Hourglass size={14} style={{ flexShrink: 0, marginLeft: 22, opacity: 0.58 }} />
              )}
              <Box sx={{ flex: 1, minWidth: 0 }}>
                <Typography
                  variant="body2"
                  sx={{
                    overflow: 'hidden',
                    textOverflow: 'ellipsis',
                    whiteSpace: 'nowrap',
                    color: isFailed
                      ? (isTransient ? 'warning.main' : 'error.main')
                      : (theme) => getChatColors(theme).assistantForeground,
                  }}
                >
                  {firstLine(entry.content || '')}
                </Typography>
                {isFailed && (
                  <Typography
                    variant="caption"
                    sx={{
                      display: 'block',
                      color: status.showRestart ? 'error.main' : 'warning.main',
                      fontWeight: status.showRestart ? 600 : 'inherit',
                    }}
                  >
                    {status.isCrashed
                      ? 'The assistant stopped unexpectedly. Restart to recover.'
                      : status.isStuckTransient
                        ? "The assistant isn't responding. Restart to recover."
                        : status.isTransientFailure
                          ? 'Waiting for the assistant — retrying…'
                          : 'This message failed to send.'}
                  </Typography>
                )}
              </Box>
              {entry.interrupt && (
                <Tooltip title="Interrupts the current turn">
                  <Zap size={13} style={{ flexShrink: 0, opacity: 0.7 }} />
                </Tooltip>
              )}
              {status.showRestart && (
                <Button
                  size="small"
                  variant="outlined"
                  color="error"
                  startIcon={isRestarting ? <CircularProgress size={12} color="inherit" /> : <RotateCcw size={14} />}
                  disabled={isRestarting}
                  onClick={handleRestart}
                  sx={{ py: 0, minHeight: 24, fontSize: '0.7rem', textTransform: 'none', flexShrink: 0 }}
                >
                  {isRestarting ? 'Restarting…' : 'Restart'}
                </Button>
              )}
              <Tooltip title="Remove from queue">
                <IconButton
                  size="small"
                  aria-label="Remove from queue"
                  onClick={() => {
                    onRemove(entry.id).catch((err: unknown) => console.warn('Failed to delete prompt from backend:', err))
                  }}
                  sx={{ p: 0.5, flexShrink: 0, color: 'text.secondary' }}
                >
                  <X size={14} />
                </IconButton>
              </Tooltip>
            </Box>
          )
        })}
      </Box>
    </Box>
  )
}

export default SessionPromptQueue
