/**
 * RobustPromptInput - A beautiful, reliable prompt input for agent sessions
 *
 * Features:
 * - Message queue: keep typing while previous messages send
 * - Queue multiple messages while offline, send when connection returns
 * - Auto-expanding textarea that grows with content
 * - Draft auto-save to localStorage (never lose a prompt)
 * - History navigation with up/down arrow keys
 * - Dropdown menu for browsing and resending history
 * - Visual queue showing pending/sending/failed messages
 * - Retry mechanism for failed sends
 * - Recovery on page reload
 */

import React, { FC, useRef, useEffect, useState, useCallback } from 'react'
import {
  Box,
  IconButton,
  CircularProgress,
  Tooltip,
  Menu,
  MenuItem,
  ListItemText,
  ListItemIcon,
  Typography,
  Divider,
  Chip,
  alpha,
  Collapse,
  LinearProgress,
} from '@mui/material'
import SendIcon from '@mui/icons-material/Send'
import HistoryIcon from '@mui/icons-material/History'
import RefreshIcon from '@mui/icons-material/Refresh'
import DeleteOutlineIcon from '@mui/icons-material/DeleteOutline'
import ErrorOutlineIcon from '@mui/icons-material/ErrorOutline'
import CheckCircleOutlineIcon from '@mui/icons-material/CheckCircleOutline'
import HourglassEmptyIcon from '@mui/icons-material/HourglassEmpty'
import CloudOffIcon from '@mui/icons-material/CloudOff'
import CloudQueueIcon from '@mui/icons-material/CloudQueue'
import EditIcon from '@mui/icons-material/Edit'
import CheckIcon from '@mui/icons-material/Check'
import CloseIcon from '@mui/icons-material/Close'
import PauseCircleOutlineIcon from '@mui/icons-material/PauseCircleOutline'
import { usePromptHistory, PromptHistoryEntry } from '../../hooks/usePromptHistory'
import { Api } from '../../api/api'

interface RobustPromptInputProps {
  sessionId: string
  onSend: (message: string) => Promise<void>
  placeholder?: string
  disabled?: boolean
  maxHeight?: number
  // Optional backend sync props
  specTaskId?: string
  projectId?: string
  apiClient?: Api<unknown>['api']
}

const RobustPromptInput: FC<RobustPromptInputProps> = ({
  sessionId,
  onSend,
  placeholder = 'Send message to agent...',
  disabled = false,
  maxHeight = 200,
  specTaskId,
  projectId,
  apiClient,
}) => {
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const editTextareaRef = useRef<HTMLTextAreaElement>(null)
  const [sendingId, setSendingId] = useState<string | null>(null)
  const [historyMenuAnchor, setHistoryMenuAnchor] = useState<null | HTMLElement>(null)
  const [showHistoryHint, setShowHistoryHint] = useState(false)
  const [isOnline, setIsOnline] = useState(navigator.onLine)
  const [showQueue, setShowQueue] = useState(true)
  const processingRef = useRef(false)

  // Editing state for queued messages
  const [editingId, setEditingId] = useState<string | null>(null)
  const [editingContent, setEditingContent] = useState('')

  const {
    draft,
    setDraft,
    history,
    historyIndex,
    navigateUp,
    navigateDown,
    saveToHistory,
    markAsSent,
    markAsFailed,
    updateContent,
    removeFromQueue,
    pendingPrompts,
    failedPrompts,
    clearDraft,
  } = usePromptHistory({ sessionId, specTaskId, projectId, apiClient })

  // Monitor online status
  useEffect(() => {
    const handleOnline = () => setIsOnline(true)
    const handleOffline = () => setIsOnline(false)

    window.addEventListener('online', handleOnline)
    window.addEventListener('offline', handleOffline)

    return () => {
      window.removeEventListener('online', handleOnline)
      window.removeEventListener('offline', handleOffline)
    }
  }, [])

  // Process queue - send pending messages one at a time
  const processQueue = useCallback(async () => {
    // Prevent concurrent processing
    if (processingRef.current || !isOnline || disabled) return

    // Find next message to send (pending, not currently sending, not being edited)
    const queuedMessages = [...failedPrompts, ...pendingPrompts]
    const nextToSend = queuedMessages.find(m =>
      m.id !== sendingId &&
      m.id !== editingId &&  // Skip message being edited
      m.status !== 'sent'
    )

    if (!nextToSend) return

    processingRef.current = true
    setSendingId(nextToSend.id)

    try {
      await onSend(nextToSend.content)
      markAsSent(nextToSend.id)
    } catch (error) {
      console.error('Failed to send message:', error)
      markAsFailed(nextToSend.id)
    } finally {
      setSendingId(null)
      processingRef.current = false
    }
  }, [isOnline, disabled, failedPrompts, pendingPrompts, sendingId, editingId, onSend, markAsSent, markAsFailed])

  // Auto-process queue when online and messages are pending
  useEffect(() => {
    if (isOnline && (pendingPrompts.length > 0 || failedPrompts.length > 0) && !processingRef.current) {
      const timer = setTimeout(processQueue, 500)
      return () => clearTimeout(timer)
    }
  }, [isOnline, pendingPrompts.length, failedPrompts.length, processQueue])

  // Continue processing queue after each send
  useEffect(() => {
    if (!sendingId && isOnline && (pendingPrompts.length > 0 || failedPrompts.length > 0)) {
      const timer = setTimeout(processQueue, 300)
      return () => clearTimeout(timer)
    }
  }, [sendingId, isOnline, pendingPrompts.length, failedPrompts.length, processQueue])

  // Auto-resize textarea
  const adjustHeight = useCallback(() => {
    const textarea = textareaRef.current
    if (!textarea) return

    textarea.style.height = 'auto'
    const newHeight = Math.min(Math.max(textarea.scrollHeight, 40), maxHeight)
    textarea.style.height = `${newHeight}px`
  }, [maxHeight])

  useEffect(() => {
    adjustHeight()
  }, [draft, adjustHeight])

  // Queue a new message
  const handleSend = useCallback(async () => {
    const content = draft.trim()
    if (!content || disabled) return

    // Add to queue with pending status
    saveToHistory(content)
    clearDraft()

    // If online and nothing sending, start processing immediately
    if (isOnline && !processingRef.current) {
      setTimeout(processQueue, 100)
    }
  }, [draft, disabled, saveToHistory, clearDraft, isOnline, processQueue])

  // Remove from queue
  const handleRemoveFromQueue = useCallback((entryId: string) => {
    removeFromQueue(entryId)
  }, [removeFromQueue])

  // Start editing a queued message
  const handleStartEdit = useCallback((entry: PromptHistoryEntry) => {
    // Don't allow editing a message that's currently sending
    if (entry.id === sendingId) return

    setEditingId(entry.id)
    setEditingContent(entry.content)

    // Focus the edit textarea after render
    setTimeout(() => {
      editTextareaRef.current?.focus()
      editTextareaRef.current?.select()
    }, 50)
  }, [sendingId])

  // Save edited message
  const handleSaveEdit = useCallback(() => {
    if (!editingId) return

    const trimmedContent = editingContent.trim()
    if (trimmedContent) {
      updateContent(editingId, trimmedContent)
    } else {
      // If content is empty, remove the message
      removeFromQueue(editingId)
    }

    setEditingId(null)
    setEditingContent('')
  }, [editingId, editingContent, updateContent, removeFromQueue])

  // Cancel editing
  const handleCancelEdit = useCallback(() => {
    setEditingId(null)
    setEditingContent('')
  }, [])

  // Handle key events in edit textarea
  const handleEditKeyDown = useCallback((e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      handleSaveEdit()
    } else if (e.key === 'Escape') {
      e.preventDefault()
      handleCancelEdit()
    }
  }, [handleSaveEdit, handleCancelEdit])

  // Handle key events
  const handleKeyDown = useCallback((e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      handleSend()
      return
    }

    const textarea = textareaRef.current
    if (!textarea) return

    const isAtStart = textarea.selectionStart === 0 && textarea.selectionEnd === 0
    const isAtEnd = textarea.selectionStart === draft.length && textarea.selectionEnd === draft.length

    if (e.key === 'ArrowUp' && isAtStart) {
      if (navigateUp()) {
        e.preventDefault()
        setShowHistoryHint(true)
        setTimeout(() => setShowHistoryHint(false), 2000)
      }
    } else if (e.key === 'ArrowDown' && isAtEnd) {
      if (navigateDown()) {
        e.preventDefault()
      }
    }
  }, [draft, handleSend, navigateUp, navigateDown])

  // Format timestamp
  const formatTime = (timestamp: number): string => {
    const diffMs = Date.now() - timestamp
    const diffMins = Math.floor(diffMs / 60000)
    const diffHours = Math.floor(diffMins / 60)

    if (diffMins < 1) return 'just now'
    if (diffMins < 60) return `${diffMins}m ago`
    if (diffHours < 24) return `${diffHours}h ago`
    return new Date(timestamp).toLocaleDateString()
  }

  const truncateContent = (content: string, maxLen: number = 60): string => {
    const firstLine = content.split('\n')[0]
    if (firstLine.length <= maxLen) return firstLine
    return firstLine.substring(0, maxLen - 3) + '...'
  }

  // All queued messages (pending + failed)
  const queuedMessages = [...failedPrompts, ...pendingPrompts]
  const sentHistory = history.filter(h => h.status === 'sent')
  const hasHistory = sentHistory.length > 0

  return (
    <Box sx={{ position: 'relative' }}>
      {/* Queued messages display */}
      <Collapse in={showQueue && queuedMessages.length > 0}>
        <Box
          sx={{
            mb: 1.5,
            borderRadius: 1.5,
            border: '1px solid',
            borderColor: editingId ? 'info.main' : isOnline ? 'primary.dark' : 'warning.dark',
            bgcolor: (theme) => alpha(theme.palette.background.paper, 0.5),
            overflow: 'hidden',
            transition: 'border-color 0.2s',
          }}
        >
          {/* Queue header */}
          <Box
            sx={{
              display: 'flex',
              alignItems: 'center',
              gap: 1,
              px: 1.5,
              py: 0.75,
              bgcolor: editingId ? 'info.dark' : isOnline ? 'primary.dark' : 'warning.dark',
              borderBottom: '1px solid',
              borderColor: 'divider',
            }}
          >
            {editingId ? (
              <PauseCircleOutlineIcon sx={{ fontSize: 16 }} />
            ) : isOnline ? (
              <CloudQueueIcon sx={{ fontSize: 16 }} />
            ) : (
              <CloudOffIcon sx={{ fontSize: 16 }} />
            )}
            <Typography variant="caption" sx={{ flex: 1, fontWeight: 600 }}>
              {editingId
                ? 'Editing message - queue paused'
                : isOnline
                  ? 'Sending queue'
                  : 'Offline - will send when connected'}
            </Typography>
            <Chip
              label={queuedMessages.length}
              size="small"
              sx={{ height: 18, fontSize: '0.7rem' }}
            />
          </Box>

          {/* Queue items */}
          <Box sx={{ maxHeight: 200, overflowY: 'auto' }}>
            {queuedMessages.map((entry, index) => {
              const isSending = entry.id === sendingId
              const isFailed = entry.status === 'failed'
              const isEditing = entry.id === editingId

              return (
                <Box
                  key={entry.id}
                  sx={{
                    display: 'flex',
                    alignItems: isEditing ? 'flex-start' : 'center',
                    gap: 1,
                    px: 1.5,
                    py: isEditing ? 1 : 0.75,
                    borderBottom: index < queuedMessages.length - 1 ? '1px solid' : 'none',
                    borderColor: 'divider',
                    bgcolor: isEditing
                      ? (theme) => alpha(theme.palette.info.main, 0.12)
                      : isFailed
                        ? (theme) => alpha(theme.palette.error.main, 0.08)
                        : 'transparent',
                    transition: 'background-color 0.2s',
                    '&:hover': !isEditing && !isSending ? {
                      bgcolor: (theme) => alpha(theme.palette.action.hover, 0.04),
                    } : undefined,
                  }}
                >
                  {/* Status indicator */}
                  {isSending ? (
                    <CircularProgress size={14} sx={{ flexShrink: 0, mt: isEditing ? 0.5 : 0 }} />
                  ) : isFailed ? (
                    <ErrorOutlineIcon sx={{ fontSize: 16, color: 'error.main', flexShrink: 0, mt: isEditing ? 0.5 : 0 }} />
                  ) : isEditing ? (
                    <EditIcon sx={{ fontSize: 16, color: 'info.main', flexShrink: 0, mt: 0.5 }} />
                  ) : (
                    <HourglassEmptyIcon sx={{ fontSize: 16, color: 'text.secondary', flexShrink: 0 }} />
                  )}

                  {/* Message content - either edit mode or display mode */}
                  {isEditing ? (
                    <Box sx={{ flex: 1, minWidth: 0 }}>
                      <Box
                        component="textarea"
                        ref={editTextareaRef}
                        value={editingContent}
                        onChange={(e) => setEditingContent(e.target.value)}
                        onKeyDown={handleEditKeyDown}
                        onBlur={handleSaveEdit}
                        sx={{
                          width: '100%',
                          resize: 'none',
                          border: '1px solid',
                          borderColor: 'info.main',
                          borderRadius: 1,
                          outline: 'none',
                          bgcolor: 'background.paper',
                          color: 'text.primary',
                          fontFamily: 'inherit',
                          fontSize: '0.875rem',
                          lineHeight: 1.5,
                          p: 1,
                          minHeight: 60,
                          maxHeight: 120,
                          overflowY: 'auto',
                          '&:focus': {
                            borderColor: 'info.light',
                            boxShadow: (theme) => `0 0 0 2px ${alpha(theme.palette.info.main, 0.25)}`,
                          },
                        }}
                      />
                      <Box sx={{ display: 'flex', gap: 0.5, mt: 0.5, justifyContent: 'flex-end' }}>
                        <Typography variant="caption" sx={{ color: 'text.secondary', flex: 1 }}>
                          Enter to save, Esc to cancel
                        </Typography>
                        <Tooltip title="Cancel (Esc)">
                          <IconButton
                            size="small"
                            onClick={handleCancelEdit}
                            sx={{ p: 0.25 }}
                          >
                            <CloseIcon sx={{ fontSize: 14 }} />
                          </IconButton>
                        </Tooltip>
                        <Tooltip title="Save (Enter)">
                          <IconButton
                            size="small"
                            onClick={handleSaveEdit}
                            color="primary"
                            sx={{ p: 0.25 }}
                          >
                            <CheckIcon sx={{ fontSize: 14 }} />
                          </IconButton>
                        </Tooltip>
                      </Box>
                    </Box>
                  ) : (
                    <Box
                      sx={{
                        flex: 1,
                        minWidth: 0,
                        cursor: isSending ? 'default' : 'pointer',
                        '&:hover': !isSending ? {
                          '& .edit-hint': { opacity: 1 },
                        } : undefined,
                      }}
                      onClick={() => !isSending && handleStartEdit(entry)}
                    >
                      <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
                        <Typography
                          variant="body2"
                          sx={{
                            overflow: 'hidden',
                            textOverflow: 'ellipsis',
                            whiteSpace: 'nowrap',
                            color: isFailed ? 'error.main' : 'text.primary',
                            flex: 1,
                          }}
                        >
                          {truncateContent(entry.content, 50)}
                        </Typography>
                        {!isSending && (
                          <EditIcon
                            className="edit-hint"
                            sx={{
                              fontSize: 14,
                              color: 'text.secondary',
                              opacity: 0,
                              transition: 'opacity 0.15s',
                              flexShrink: 0,
                            }}
                          />
                        )}
                      </Box>
                      {isFailed && (
                        <Typography variant="caption" sx={{ color: 'error.main' }}>
                          Failed to send - will retry
                        </Typography>
                      )}
                    </Box>
                  )}

                  {/* Actions - only show when not editing */}
                  {!isEditing && !isSending && (
                    <Tooltip title="Remove from queue">
                      <IconButton
                        size="small"
                        onClick={(e) => {
                          e.stopPropagation()
                          handleRemoveFromQueue(entry.id)
                        }}
                        sx={{ p: 0.5 }}
                      >
                        <DeleteOutlineIcon sx={{ fontSize: 16 }} />
                      </IconButton>
                    </Tooltip>
                  )}
                </Box>
              )
            })}
          </Box>

          {/* Sending progress */}
          {sendingId && <LinearProgress sx={{ height: 2 }} />}
        </Box>
      </Collapse>

      {/* History navigation hint */}
      {showHistoryHint && historyIndex >= 0 && (
        <Box
          sx={{
            position: 'absolute',
            top: -28,
            left: '50%',
            transform: 'translateX(-50%)',
            px: 1.5,
            py: 0.5,
            borderRadius: 1,
            bgcolor: 'primary.main',
            color: 'primary.contrastText',
            fontSize: '0.75rem',
            zIndex: 1,
            whiteSpace: 'nowrap',
          }}
        >
          Browsing history ({historyIndex + 1}/{sentHistory.length}) - ↓ to return
        </Box>
      )}

      {/* Input container */}
      <Box
        sx={{
          display: 'flex',
          gap: 1,
          alignItems: 'flex-end',
          bgcolor: 'background.paper',
          borderRadius: 2,
          border: '1px solid',
          borderColor: !isOnline
            ? 'warning.main'
            : historyIndex >= 0
              ? 'info.main'
              : 'divider',
          transition: 'border-color 0.2s, box-shadow 0.2s',
          boxShadow: !isOnline
            ? (theme) => `0 0 0 2px ${alpha(theme.palette.warning.main, 0.2)}`
            : historyIndex >= 0
              ? (theme) => `0 0 0 2px ${alpha(theme.palette.info.main, 0.2)}`
              : 'none',
          '&:focus-within': {
            borderColor: 'primary.main',
            boxShadow: (theme) => `0 0 0 2px ${alpha(theme.palette.primary.main, 0.2)}`,
          },
          p: 1,
        }}
      >
        {/* History button */}
        {hasHistory && (
          <Tooltip title="Browse prompt history (↑/↓ to navigate)">
            <IconButton
              size="small"
              onClick={(e) => setHistoryMenuAnchor(e.currentTarget)}
              sx={{
                color: historyIndex >= 0 ? 'info.main' : 'text.secondary',
                flexShrink: 0,
              }}
            >
              <HistoryIcon fontSize="small" />
            </IconButton>
          </Tooltip>
        )}

        {/* Textarea */}
        <Box
          component="textarea"
          ref={textareaRef}
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={handleKeyDown}
          placeholder={isOnline ? placeholder : 'Offline - messages will queue'}
          disabled={disabled}
          sx={{
            flex: 1,
            resize: 'none',
            border: 'none',
            outline: 'none',
            bgcolor: 'transparent',
            color: 'text.primary',
            fontFamily: 'inherit',
            fontSize: '0.875rem',
            lineHeight: 1.5,
            p: 0.5,
            minHeight: 40,
            maxHeight: maxHeight,
            overflowY: 'auto',
            '&::placeholder': {
              color: !isOnline ? 'warning.main' : 'text.secondary',
              opacity: 0.7,
            },
            '&:disabled': {
              opacity: 0.6,
              cursor: 'not-allowed',
            },
          }}
        />

        {/* Offline indicator */}
        {!isOnline && (
          <Tooltip title="You're offline - messages will queue and send when connected">
            <CloudOffIcon sx={{ color: 'warning.main', fontSize: 20, flexShrink: 0 }} />
          </Tooltip>
        )}

        {/* Send button */}
        <Tooltip title="Add to queue (Enter)">
          <span>
            <IconButton
              onClick={handleSend}
              disabled={!draft.trim() || disabled}
              color="primary"
              sx={{
                flexShrink: 0,
                bgcolor: draft.trim() ? 'primary.main' : 'transparent',
                color: draft.trim() ? 'primary.contrastText' : 'text.secondary',
                '&:hover': {
                  bgcolor: draft.trim() ? 'primary.dark' : undefined,
                },
                '&.Mui-disabled': {
                  bgcolor: 'transparent',
                  color: 'text.disabled',
                },
              }}
            >
              <SendIcon fontSize="small" />
            </IconButton>
          </span>
        </Tooltip>
      </Box>

      {/* Keyboard hint */}
      <Box
        sx={{
          display: 'flex',
          justifyContent: 'space-between',
          mt: 0.5,
          px: 0.5,
        }}
      >
        <Typography variant="caption" sx={{ color: 'text.secondary', opacity: 0.7 }}>
          Enter to send, Shift+Enter for new line
        </Typography>
        <Box sx={{ display: 'flex', gap: 2 }}>
          {queuedMessages.length > 0 && (
            <Typography variant="caption" sx={{ color: 'primary.main' }}>
              {queuedMessages.length} in queue
            </Typography>
          )}
          {hasHistory && (
            <Typography variant="caption" sx={{ color: 'text.secondary', opacity: 0.7 }}>
              ↑/↓ history
            </Typography>
          )}
        </Box>
      </Box>

      {/* History menu */}
      <Menu
        anchorEl={historyMenuAnchor}
        open={Boolean(historyMenuAnchor)}
        onClose={() => setHistoryMenuAnchor(null)}
        anchorOrigin={{ vertical: 'top', horizontal: 'left' }}
        transformOrigin={{ vertical: 'bottom', horizontal: 'left' }}
        slotProps={{
          paper: {
            sx: {
              maxHeight: 400,
              minWidth: 350,
              maxWidth: 500,
            },
          },
        }}
      >
        {sentHistory.length > 0 ? (
          <>
            <Box sx={{ px: 2, py: 1, bgcolor: 'background.default' }}>
              <Typography variant="caption" sx={{ fontWeight: 600, color: 'text.secondary' }}>
                Recent Prompts
              </Typography>
            </Box>
            {sentHistory.slice().reverse().slice(0, 20).map((entry, index) => (
              <MenuItem
                key={entry.id}
                onClick={() => {
                  setDraft(entry.content)
                  setHistoryMenuAnchor(null)
                  textareaRef.current?.focus()
                }}
                sx={{
                  borderLeft: historyIndex === index ? '3px solid' : '3px solid transparent',
                  borderColor: historyIndex === index ? 'primary.main' : 'transparent',
                }}
              >
                <ListItemIcon>
                  <CheckCircleOutlineIcon fontSize="small" sx={{ color: 'success.main', opacity: 0.6 }} />
                </ListItemIcon>
                <ListItemText
                  primary={truncateContent(entry.content)}
                  secondary={formatTime(entry.timestamp)}
                  primaryTypographyProps={{ noWrap: true, fontSize: '0.875rem' }}
                  secondaryTypographyProps={{ fontSize: '0.75rem' }}
                />
                {index < 9 && (
                  <Typography variant="caption" sx={{ color: 'text.disabled', fontFamily: 'monospace', ml: 1 }}>
                    ↑{index + 1}
                  </Typography>
                )}
              </MenuItem>
            ))}
          </>
        ) : (
          <MenuItem disabled>
            <ListItemText
              primary="No history yet"
              secondary="Your sent messages will appear here"
            />
          </MenuItem>
        )}
      </Menu>
    </Box>
  )
}

export default RobustPromptInput
