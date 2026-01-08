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
  TextField,
  InputAdornment,
} from '@mui/material'
import SendIcon from '@mui/icons-material/Send'
import HistoryIcon from '@mui/icons-material/History'
import RefreshIcon from '@mui/icons-material/Refresh'
import ErrorOutlineIcon from '@mui/icons-material/ErrorOutline'
import CheckCircleOutlineIcon from '@mui/icons-material/CheckCircleOutline'
import HourglassEmptyIcon from '@mui/icons-material/HourglassEmpty'
import CloudOffIcon from '@mui/icons-material/CloudOff'
import CloudQueueIcon from '@mui/icons-material/CloudQueue'
import EditIcon from '@mui/icons-material/Edit'
import CheckIcon from '@mui/icons-material/Check'
import CloseIcon from '@mui/icons-material/Close'
import PauseCircleOutlineIcon from '@mui/icons-material/PauseCircleOutline'
import DragIndicatorIcon from '@mui/icons-material/DragIndicator'
import BoltIcon from '@mui/icons-material/Bolt'
import QueueIcon from '@mui/icons-material/Queue'
import PushPinIcon from '@mui/icons-material/PushPin'
import PushPinOutlinedIcon from '@mui/icons-material/PushPinOutlined'
import SearchIcon from '@mui/icons-material/Search'
import ImageIcon from '@mui/icons-material/Image'
import {
  DndContext,
  closestCenter,
  KeyboardSensor,
  PointerSensor,
  useSensor,
  useSensors,
  DragEndEvent,
} from '@dnd-kit/core'
import {
  SortableContext,
  sortableKeyboardCoordinates,
  useSortable,
  verticalListSortingStrategy,
} from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { usePromptHistory, PromptHistoryEntry } from '../../hooks/usePromptHistory'
import { Api } from '../../api/api'

interface RobustPromptInputProps {
  sessionId: string
  onSend: (message: string, interrupt?: boolean) => Promise<void>
  placeholder?: string
  disabled?: boolean
  maxHeight?: number
  // Optional backend sync props
  specTaskId?: string
  projectId?: string
  apiClient?: Api<unknown>['api']
  // Called when the input component height changes (queue added, textarea resized)
  onHeightChange?: () => void
  // Text to append to the draft (e.g., uploaded file paths)
  // Pass a new unique value each time to trigger an append
  appendText?: string
  // Called when an image is pasted - parent should upload and return the file path
  onImagePaste?: (file: File) => Promise<string | null>
}

// Props for sortable queue item
interface SortableQueueItemProps {
  entry: PromptHistoryEntry
  index: number
  totalCount: number
  isSending: boolean
  isEditing: boolean
  editingContent: string
  setEditingContent: (content: string) => void
  editTextareaRef: React.RefObject<HTMLTextAreaElement | null>
  handleEditKeyDown: (e: React.KeyboardEvent<HTMLTextAreaElement>) => void
  handleSaveEdit: () => void
  handleCancelEdit: () => void
  handleStartEdit: (entry: PromptHistoryEntry) => void
  handleRemoveFromQueue: (id: string) => void
  handleToggleInterrupt: (id: string) => void
  truncateContent: (content: string, maxLen?: number) => string
}

// Sortable queue item component
const SortableQueueItem: FC<SortableQueueItemProps> = ({
  entry,
  index,
  totalCount,
  isSending,
  isEditing,
  editingContent,
  setEditingContent,
  editTextareaRef,
  handleEditKeyDown,
  handleSaveEdit,
  handleCancelEdit,
  handleStartEdit,
  handleRemoveFromQueue,
  handleToggleInterrupt,
  truncateContent,
}) => {
  const {
    attributes,
    listeners,
    setNodeRef,
    transform,
    transition,
    isDragging,
  } = useSortable({ id: entry.id })

  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
    opacity: isDragging ? 0.5 : 1,
  }

  const isFailed = entry.status === 'failed'

  return (
    <Box
      ref={setNodeRef}
      style={style}
      sx={{
        display: 'flex',
        alignItems: isEditing ? 'flex-start' : 'center',
        gap: 0.5,
        px: 1,
        py: isEditing ? 1 : 0.5,
        borderBottom: index < totalCount - 1 ? '1px solid' : 'none',
        borderColor: 'divider',
        bgcolor: isDragging
          ? (theme) => alpha(theme.palette.primary.main, 0.12)
          : isEditing
            ? (theme) => alpha(theme.palette.info.main, 0.12)
            : isFailed
              ? (theme) => alpha(theme.palette.error.main, 0.08)
              : 'transparent',
        transition: 'background-color 0.2s',
        '&:hover': !isEditing && !isSending && !isDragging ? {
          bgcolor: (theme) => alpha(theme.palette.action.hover, 0.04),
          '& .drag-handle': { opacity: 1 },
        } : undefined,
      }}
    >
      {/* Drag handle - only show when not sending and not editing */}
      {!isSending && !isEditing && (
        <Box
          {...attributes}
          {...listeners}
          className="drag-handle"
          sx={{
            display: 'flex',
            alignItems: 'center',
            cursor: 'grab',
            color: 'text.secondary',
            opacity: 0.4,
            transition: 'opacity 0.15s',
            '&:hover': { opacity: 1, color: 'text.primary' },
            '&:active': { cursor: 'grabbing' },
            flexShrink: 0,
            p: 0.25,
            mr: 0.25,
          }}
        >
          <DragIndicatorIcon sx={{ fontSize: 16 }} />
        </Box>
      )}

      {/* Status indicator */}
      {isSending ? (
        <CircularProgress size={14} sx={{ flexShrink: 0, mt: isEditing ? 0.5 : 0, ml: isEditing ? 0 : 2.5 }} />
      ) : isFailed ? (
        <ErrorOutlineIcon sx={{ fontSize: 16, color: 'error.main', flexShrink: 0, mt: isEditing ? 0.5 : 0 }} />
      ) : isEditing ? (
        <EditIcon sx={{ fontSize: 16, color: 'info.main', flexShrink: 0, mt: 0.5, ml: 2.5 }} />
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
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.25 }}>
          {/* Interrupt toggle */}
          <Tooltip title={entry.interrupt !== false ? "Interrupt mode - click to queue after current" : "Queue mode - click to interrupt"}>
            <IconButton
              size="small"
              onClick={(e) => {
                e.stopPropagation()
                handleToggleInterrupt(entry.id)
              }}
              sx={{
                p: 0.5,
                color: entry.interrupt !== false ? 'warning.main' : 'info.main',
                opacity: 0.7,
                '&:hover': { opacity: 1 },
              }}
            >
              {entry.interrupt !== false ? (
                <BoltIcon sx={{ fontSize: 14 }} />
              ) : (
                <QueueIcon sx={{ fontSize: 14 }} />
              )}
            </IconButton>
          </Tooltip>
          {/* Remove */}
          <Tooltip title="Remove from queue">
            <IconButton
              size="small"
              onClick={(e) => {
                e.stopPropagation()
                handleRemoveFromQueue(entry.id)
              }}
              sx={{ p: 0.5 }}
            >
              <CloseIcon sx={{ fontSize: 16 }} />
            </IconButton>
          </Tooltip>
        </Box>
      )}
    </Box>
  )
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
  onHeightChange,
  appendText,
  onImagePaste,
}) => {
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const editTextareaRef = useRef<HTMLTextAreaElement>(null)
  const [sendingId, setSendingId] = useState<string | null>(null)
  const [historyMenuAnchor, setHistoryMenuAnchor] = useState<null | HTMLElement>(null)
  const [showHistoryHint, setShowHistoryHint] = useState(false)
  const [historySearchQuery, setHistorySearchQuery] = useState('')
  const [isOnline, setIsOnline] = useState(navigator.onLine)
  const [showQueue, setShowQueue] = useState(true)
  const [interruptMode, setInterruptMode] = useState(false) // false = queue after (default), true = interrupt
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
    updateInterrupt,
    removeFromQueue,
    reorderQueue,
    pendingPrompts,
    failedPrompts,
    clearDraft,
    pinPrompt,
  } = usePromptHistory({ sessionId, specTaskId, projectId, apiClient })

  // Track previous appendText to detect changes
  const prevAppendTextRef = useRef<string | undefined>(undefined)

  // Handle prepending text from parent (e.g., uploaded file paths)
  useEffect(() => {
    if (appendText && appendText !== prevAppendTextRef.current) {
      // Strip any unique key suffix (format: "text#123")
      const textToPrepend = appendText.replace(/#\d+$/, '')
      // Prepend to draft with proper spacing
      setDraft(prev => {
        const needsSpace = prev.length > 0 && !prev.startsWith(' ') && !prev.startsWith('\n')
        return textToPrepend + (needsSpace ? ' ' : '') + prev
      })
      prevAppendTextRef.current = appendText
      // Focus the textarea
      textareaRef.current?.focus()
    }
  }, [appendText, setDraft])

  // DnD sensors
  const sensors = useSensors(
    useSensor(PointerSensor, {
      activationConstraint: {
        distance: 8, // Require 8px drag before activating (allows clicks)
      },
    }),
    useSensor(KeyboardSensor, {
      coordinateGetter: sortableKeyboardCoordinates,
    })
  )

  // Handle drag end
  const handleDragEnd = useCallback((event: DragEndEvent) => {
    const { active, over } = event
    if (over && active.id !== over.id) {
      reorderQueue(active.id as string, over.id as string)
    }
  }, [reorderQueue])

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

  // Check if backend queue processing is enabled
  // When specTaskId, projectId, and apiClient are all provided, the backend handles queue processing
  const backendQueueEnabled = !!(specTaskId && projectId && apiClient)

  // Process queue - send pending messages one at a time
  // When backend queue processing is enabled, the backend handles sending after sync
  // When disabled, frontend sends directly via onSend
  const processQueue = useCallback(async () => {
    // When backend queue is enabled, let the backend handle sending
    // The backend processes the queue after sync (interrupt prompts immediately,
    // queue prompts after message_completed events)
    if (backendQueueEnabled) return

    // Prevent concurrent processing
    if (processingRef.current || !isOnline || disabled) return

    // Build queue sorted: interrupt mode first, then queue mode, within each by timestamp
    const queuedMessages = [...failedPrompts, ...pendingPrompts].sort((a, b) => {
      const aInterrupt = a.interrupt !== false
      const bInterrupt = b.interrupt !== false
      if (aInterrupt && !bInterrupt) return -1
      if (!aInterrupt && bInterrupt) return 1
      return a.timestamp - b.timestamp
    })

    // If editing, find the index of the message being edited
    // Block that message and everything after it (maintain ordering)
    let editingIndex = -1
    if (editingId) {
      editingIndex = queuedMessages.findIndex(m => m.id === editingId)
    }

    // Find next message to send (only messages before the editing one)
    const nextToSend = queuedMessages.find((m, index) => {
      // Don't send if currently sending
      if (m.id === sendingId) return false
      // Don't send if already sent
      if (m.status === 'sent') return false
      // If editing, don't send this message or anything after it
      if (editingIndex !== -1 && index >= editingIndex) return false
      return true
    })

    if (!nextToSend) return

    processingRef.current = true
    setSendingId(nextToSend.id)

    try {
      // Pass interrupt flag to backend - true means interrupt current work, false means queue after
      await onSend(nextToSend.content, nextToSend.interrupt !== false)
      markAsSent(nextToSend.id)
    } catch (error) {
      console.error('Failed to send message:', error)
      markAsFailed(nextToSend.id)
    } finally {
      setSendingId(null)
      processingRef.current = false
    }
  }, [backendQueueEnabled, isOnline, disabled, failedPrompts, pendingPrompts, sendingId, editingId, onSend, markAsSent, markAsFailed])

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

    const oldHeight = textarea.offsetHeight
    textarea.style.height = 'auto'
    const newHeight = Math.min(Math.max(textarea.scrollHeight, 40), maxHeight)
    textarea.style.height = `${newHeight}px`

    // Notify parent if height changed
    if (oldHeight !== newHeight && onHeightChange) {
      onHeightChange()
    }
  }, [maxHeight, onHeightChange])

  useEffect(() => {
    adjustHeight()
  }, [draft, adjustHeight])

  // Notify parent when queue changes (affects overall height)
  const queueLength = pendingPrompts.length + failedPrompts.length
  useEffect(() => {
    if (onHeightChange) {
      // Small delay to allow Collapse animation to start
      const timer = setTimeout(onHeightChange, 50)
      return () => clearTimeout(timer)
    }
  }, [queueLength, onHeightChange])

  // Queue a new message
  const handleSend = useCallback(async () => {
    const content = draft.trim()
    if (!content || disabled) return

    // Add to queue with pending status, passing interrupt mode
    saveToHistory(content, interruptMode)
    clearDraft()

    // If online and nothing sending, start processing immediately
    if (isOnline && !processingRef.current) {
      setTimeout(processQueue, 100)
    }
  }, [draft, disabled, saveToHistory, clearDraft, isOnline, processQueue, interruptMode])

  // Remove from queue
  const handleRemoveFromQueue = useCallback((entryId: string) => {
    removeFromQueue(entryId)
  }, [removeFromQueue])

  // Toggle interrupt mode for a queued message
  const handleToggleInterrupt = useCallback((entryId: string) => {
    const entry = [...failedPrompts, ...pendingPrompts].find(e => e.id === entryId)
    if (entry) {
      updateInterrupt(entryId, entry.interrupt === false)
    }
  }, [failedPrompts, pendingPrompts, updateInterrupt])

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
  // Enter = queue mode (non-interrupt), Ctrl+Enter = interrupt mode
  const handleKeyDown = useCallback((e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      // Ctrl+Enter = interrupt mode, Enter = queue mode
      const useInterrupt = e.ctrlKey || e.metaKey // metaKey for Mac Cmd key
      // Temporarily set the mode for this send, then restore
      const originalMode = interruptMode
      setInterruptMode(useInterrupt)
      // Need to call saveToHistory directly with the correct mode since handleSend uses state
      const content = draft.trim()
      if (content && !disabled) {
        saveToHistory(content, useInterrupt)
        clearDraft()
        if (isOnline && !processingRef.current) {
          setTimeout(processQueue, 100)
        }
      }
      // Restore original mode
      setInterruptMode(originalMode)
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
  }, [draft, disabled, interruptMode, saveToHistory, clearDraft, isOnline, processQueue, navigateUp, navigateDown])

  // Handle paste events for images
  const handlePaste = useCallback(async (e: React.ClipboardEvent<HTMLTextAreaElement>) => {
    console.log('[RobustPromptInput] Paste event received, onImagePaste:', !!onImagePaste)
    if (!onImagePaste) return

    const items = e.clipboardData?.items
    console.log('[RobustPromptInput] Clipboard items:', items?.length, Array.from(items || []).map(i => i.type))
    if (!items) return

    for (let i = 0; i < items.length; i++) {
      const item = items[i]
      if (item.type.startsWith('image/')) {
        console.log('[RobustPromptInput] Found image in clipboard:', item.type)
        e.preventDefault()
        const blob = item.getAsFile()
        if (blob) {
          // Create a File with a generated name based on timestamp
          const timestamp = new Date().toISOString().replace(/[:.]/g, '-').slice(0, 19)
          const extension = item.type === 'image/png' ? 'png' : 'jpg'
          const file = new File([blob], `pasted-image-${timestamp}.${extension}`, { type: item.type })

          console.log('[RobustPromptInput] Uploading pasted image:', file.name)
          // Call parent to upload and get the path
          const filePath = await onImagePaste(file)
          console.log('[RobustPromptInput] Upload result:', filePath)
          if (filePath) {
            // Prepend the file path to the draft
            setDraft(prev => {
              const needsSpace = prev.length > 0 && !prev.startsWith(' ') && !prev.startsWith('\n')
              return filePath + (needsSpace ? ' ' : '') + prev
            })
          }
        }
        break
      }
    }
  }, [onImagePaste, setDraft])

  // Track drag state for visual feedback
  const [isDraggingOver, setIsDraggingOver] = useState(false)

  // Handle drag enter - show visual feedback
  const handleDragEnter = useCallback((e: React.DragEvent<HTMLTextAreaElement>) => {
    e.preventDefault()
    e.stopPropagation()
    if (onImagePaste) {
      setIsDraggingOver(true)
    }
  }, [onImagePaste])

  // Handle drag leave - hide visual feedback
  const handleDragLeave = useCallback((e: React.DragEvent<HTMLTextAreaElement>) => {
    e.preventDefault()
    e.stopPropagation()
    setIsDraggingOver(false)
  }, [])

  // Handle drag over - prevent default to allow drop
  const handleDragOver = useCallback((e: React.DragEvent<HTMLTextAreaElement>) => {
    e.preventDefault()
    e.stopPropagation()
    if (onImagePaste) {
      e.dataTransfer.dropEffect = 'copy'
    }
  }, [onImagePaste])

  // Handle drop events for files
  const handleDrop = useCallback(async (e: React.DragEvent<HTMLTextAreaElement>) => {
    e.preventDefault()
    e.stopPropagation()
    setIsDraggingOver(false)

    if (!onImagePaste) return

    const files = Array.from(e.dataTransfer.files)
    console.log('[RobustPromptInput] Dropped files:', files.map(f => f.name))
    for (const file of files) {
      // Upload the file and prepend path to draft
      const filePath = await onImagePaste(file)
      if (filePath) {
        setDraft(prev => {
          const needsSpace = prev.length > 0 && !prev.startsWith(' ') && !prev.startsWith('\n')
          return filePath + (needsSpace ? ' ' : '') + prev
        })
      }
    }
  }, [onImagePaste, setDraft])

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

  // All queued messages (pending + failed), sorted: interrupt mode first, then queue mode
  const queuedMessages = [...failedPrompts, ...pendingPrompts].sort((a, b) => {
    // Interrupt mode (true or undefined) comes first
    const aInterrupt = a.interrupt !== false
    const bInterrupt = b.interrupt !== false
    if (aInterrupt && !bInterrupt) return -1
    if (!aInterrupt && bInterrupt) return 1
    // Within same mode, maintain original order by timestamp
    return a.timestamp - b.timestamp
  })
  const sentHistory = history.filter(h => h.status === 'sent')
  const hasHistory = sentHistory.length > 0

  return (
    <Box
      className="prompt-input-container"
      data-prompt-input="true"
      sx={{ position: 'relative' }}
    >
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
                ? 'Editing - paused from here'
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

          {/* Queue items with drag and drop */}
          <Box sx={{ maxHeight: 200, overflowY: 'auto' }}>
            <DndContext
              sensors={sensors}
              collisionDetection={closestCenter}
              onDragEnd={handleDragEnd}
            >
              <SortableContext
                items={queuedMessages.map(m => m.id)}
                strategy={verticalListSortingStrategy}
              >
                {queuedMessages.map((entry, index) => (
                  <SortableQueueItem
                    key={entry.id}
                    entry={entry}
                    index={index}
                    totalCount={queuedMessages.length}
                    isSending={entry.id === sendingId}
                    isEditing={entry.id === editingId}
                    editingContent={editingContent}
                    setEditingContent={setEditingContent}
                    editTextareaRef={editTextareaRef}
                    handleEditKeyDown={handleEditKeyDown}
                    handleSaveEdit={handleSaveEdit}
                    handleCancelEdit={handleCancelEdit}
                    handleStartEdit={handleStartEdit}
                    handleRemoveFromQueue={handleRemoveFromQueue}
                    handleToggleInterrupt={handleToggleInterrupt}
                    truncateContent={truncateContent}
                  />
                ))}
              </SortableContext>
            </DndContext>
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
          onPaste={handlePaste}
          onDragEnter={handleDragEnter}
          onDragLeave={handleDragLeave}
          onDragOver={handleDragOver}
          onDrop={handleDrop}
          placeholder={isDraggingOver ? 'Drop file to upload...' : (isOnline ? placeholder : 'Offline - messages will queue')}
          disabled={disabled}
          sx={{
            flex: 1,
            resize: 'none',
            border: isDraggingOver ? '2px dashed' : 'none',
            borderColor: 'primary.main',
            borderRadius: isDraggingOver ? 1 : 0,
            outline: 'none',
            bgcolor: isDraggingOver ? (theme) => alpha(theme.palette.primary.main, 0.08) : 'transparent',
            color: 'text.primary',
            fontFamily: 'inherit',
            fontSize: '0.875rem',
            lineHeight: 1.5,
            p: 0.5,
            minHeight: 40,
            maxHeight: maxHeight,
            overflowY: 'auto',
            transition: 'background-color 0.15s, border 0.15s',
            '&::placeholder': {
              color: isDraggingOver ? 'primary.main' : (!isOnline ? 'warning.main' : 'text.secondary'),
              opacity: isDraggingOver ? 1 : 0.7,
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

        {/* Interrupt mode toggle */}
        <Tooltip
          title={
            <Box>
              <Typography variant="body2" sx={{ fontWeight: 600 }}>
                {interruptMode ? 'Interrupt Mode' : 'Queue Mode'}
              </Typography>
              <Typography variant="caption" sx={{ display: 'block', mt: 0.5 }}>
                {interruptMode
                  ? 'Messages sent immediately, interrupting current conversation'
                  : 'Messages wait until current conversation completes'
                }
              </Typography>
              <Typography variant="caption" sx={{ display: 'block', mt: 1, color: 'grey.400' }}>
                Keyboard: Enter = queue, Ctrl+Enter = interrupt
              </Typography>
            </Box>
          }
        >
          <IconButton
            size="small"
            onClick={() => setInterruptMode(!interruptMode)}
            sx={{
              flexShrink: 0,
              color: interruptMode ? 'warning.main' : 'info.main',
              bgcolor: (theme) => alpha(
                interruptMode ? theme.palette.warning.main : theme.palette.info.main,
                0.1
              ),
              '&:hover': {
                bgcolor: (theme) => alpha(
                  interruptMode ? theme.palette.warning.main : theme.palette.info.main,
                  0.2
                ),
              },
            }}
          >
            {interruptMode ? (
              <BoltIcon fontSize="small" />
            ) : (
              <QueueIcon fontSize="small" />
            )}
          </IconButton>
        </Tooltip>

        {/* Send button */}
        <Tooltip title="Add to queue (Enter = queue, Ctrl+Enter = interrupt)">
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
          Enter = queue, Ctrl+Enter = interrupt, Shift+Enter = new line
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
        onClose={() => {
          setHistoryMenuAnchor(null)
          setHistorySearchQuery('')
        }}
        anchorOrigin={{ vertical: 'top', horizontal: 'left' }}
        transformOrigin={{ vertical: 'bottom', horizontal: 'left' }}
        slotProps={{
          paper: {
            sx: {
              maxHeight: 450,
              minWidth: 400,
              maxWidth: 550,
            },
          },
        }}
      >
        {/* Search field */}
        <Box sx={{ px: 2, py: 1.5, borderBottom: '1px solid', borderColor: 'divider' }}>
          <TextField
            size="small"
            fullWidth
            placeholder="Search history..."
            value={historySearchQuery}
            onChange={(e) => setHistorySearchQuery(e.target.value)}
            onClick={(e) => e.stopPropagation()}
            onKeyDown={(e) => e.stopPropagation()}
            InputProps={{
              startAdornment: (
                <InputAdornment position="start">
                  <SearchIcon sx={{ fontSize: 18, color: 'text.secondary' }} />
                </InputAdornment>
              ),
            }}
            sx={{
              '& .MuiOutlinedInput-root': {
                fontSize: '0.875rem',
              },
            }}
          />
        </Box>

        {(() => {
          // Filter and sort history: pinned first, then by timestamp, filtered by search
          const filteredHistory = sentHistory
            .filter(entry => {
              if (!historySearchQuery.trim()) return true
              return entry.content.toLowerCase().includes(historySearchQuery.toLowerCase())
            })
            .sort((a, b) => {
              // Pinned items first
              if (a.pinned && !b.pinned) return -1
              if (!a.pinned && b.pinned) return 1
              // Then by timestamp (newest first)
              return b.timestamp - a.timestamp
            })
            .slice(0, 30)

          const pinnedCount = filteredHistory.filter(e => e.pinned).length

          if (filteredHistory.length === 0) {
            return (
              <MenuItem disabled>
                <ListItemText
                  primary={historySearchQuery ? 'No matching prompts' : 'No history yet'}
                  secondary={historySearchQuery ? 'Try a different search term' : 'Your sent messages will appear here'}
                />
              </MenuItem>
            )
          }

          return (
            <>
              {/* Pinned section header */}
              {pinnedCount > 0 && (
                <Box sx={{ px: 2, py: 0.75, bgcolor: 'background.default' }}>
                  <Typography variant="caption" sx={{ fontWeight: 600, color: 'warning.main', display: 'flex', alignItems: 'center', gap: 0.5 }}>
                    <PushPinIcon sx={{ fontSize: 14 }} />
                    Pinned ({pinnedCount})
                  </Typography>
                </Box>
              )}

              {filteredHistory.map((entry, index) => {
                const isPinned = entry.pinned
                const isFirstUnpinned = index > 0 && !isPinned && filteredHistory[index - 1]?.pinned

                return (
                  <Box key={entry.id}>
                    {/* Show "Recent" header before first unpinned item */}
                    {isFirstUnpinned && (
                      <Box sx={{ px: 2, py: 0.75, bgcolor: 'background.default', borderTop: '1px solid', borderColor: 'divider' }}>
                        <Typography variant="caption" sx={{ fontWeight: 600, color: 'text.secondary' }}>
                          Recent
                        </Typography>
                      </Box>
                    )}
                    {/* Show "Recent" header at top if no pinned items */}
                    {index === 0 && pinnedCount === 0 && (
                      <Box sx={{ px: 2, py: 0.75, bgcolor: 'background.default' }}>
                        <Typography variant="caption" sx={{ fontWeight: 600, color: 'text.secondary' }}>
                          Recent Prompts
                        </Typography>
                      </Box>
                    )}
                    <MenuItem
                      onClick={() => {
                        setDraft(entry.content)
                        setHistoryMenuAnchor(null)
                        setHistorySearchQuery('')
                        textareaRef.current?.focus()
                      }}
                      sx={{
                        borderLeft: isPinned ? '3px solid' : '3px solid transparent',
                        borderColor: isPinned ? 'warning.main' : 'transparent',
                        bgcolor: isPinned ? (theme) => alpha(theme.palette.warning.main, 0.04) : 'transparent',
                        '&:hover .pin-button': { opacity: 1 },
                      }}
                    >
                      <ListItemIcon>
                        {isPinned ? (
                          <PushPinIcon fontSize="small" sx={{ color: 'warning.main' }} />
                        ) : (
                          <CheckCircleOutlineIcon fontSize="small" sx={{ color: 'success.main', opacity: 0.6 }} />
                        )}
                      </ListItemIcon>
                      <ListItemText
                        primary={truncateContent(entry.content)}
                        secondary={formatTime(entry.timestamp)}
                        primaryTypographyProps={{ noWrap: true, fontSize: '0.875rem' }}
                        secondaryTypographyProps={{ fontSize: '0.75rem' }}
                      />
                      {/* Pin/unpin button */}
                      <Tooltip title={isPinned ? 'Unpin' : 'Pin for quick access'}>
                        <IconButton
                          className="pin-button"
                          size="small"
                          onClick={(e) => {
                            e.stopPropagation()
                            pinPrompt(entry.id, !isPinned)
                          }}
                          sx={{
                            ml: 1,
                            opacity: isPinned ? 0.8 : 0.3,
                            transition: 'opacity 0.15s',
                            color: isPinned ? 'warning.main' : 'text.secondary',
                            '&:hover': {
                              color: isPinned ? 'warning.dark' : 'warning.main',
                            },
                          }}
                        >
                          {isPinned ? (
                            <PushPinIcon sx={{ fontSize: 16 }} />
                          ) : (
                            <PushPinOutlinedIcon sx={{ fontSize: 16 }} />
                          )}
                        </IconButton>
                      </Tooltip>
                    </MenuItem>
                  </Box>
                )
              })}
            </>
          )
        })()}
      </Menu>
    </Box>
  )
}

export default RobustPromptInput
