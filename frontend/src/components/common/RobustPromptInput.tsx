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
  Chip,
  alpha,
  Collapse,
  LinearProgress,
  TextField,
  InputAdornment,
} from '@mui/material'
import {
  History,
  SendHorizontal,
  ListStart,
  CircleAlert,
  CheckCircle,
  Hourglass,
  CloudOff,
  Cloud,
  Pencil,
  Check,
  X,
  CirclePause,
  GripVertical,
  Zap,
  Pin,
  PinOff,
  Search,
  Image,
  Paperclip,
  FileText,
  Camera,
} from 'lucide-react'
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

// Attachment that's pending to be sent with the message
// Supports offline queueing - file data is stored until upload completes
interface PendingAttachment {
  id: string
  name: string
  path?: string // Set once uploaded, undefined while pending upload
  file?: File // The file data, kept until uploaded (for offline support)
  type: 'image' | 'text' | 'file'
  previewUrl?: string // For images, a data URL for preview
  uploadStatus: 'pending' | 'uploading' | 'uploaded' | 'failed'
  error?: string // Error message if upload failed
}

// Threshold for converting large text paste to file attachment (10KB)
const LARGE_TEXT_THRESHOLD = 10 * 1024

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
  // Called when a file is uploaded (image, text, or other file)
  // Parent should upload and return the file path, or null on failure
  onFileUpload?: (file: File) => Promise<string | null>
  // Deprecated: use onFileUpload instead
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

  // Force re-render every second for failed items with retry countdown
  const [, forceUpdate] = useState(0)
  useEffect(() => {
    if (isFailed && entry.nextRetryAt && entry.nextRetryAt > Date.now()) {
      const interval = setInterval(() => forceUpdate(n => n + 1), 1000)
      return () => clearInterval(interval)
    }
  }, [isFailed, entry.nextRetryAt])

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
          <GripVertical size={16} />
        </Box>
      )}

      {/* Status indicator */}
      {isSending ? (
        <CircularProgress size={14} sx={{ flexShrink: 0, mt: isEditing ? 0.5 : 0, ml: isEditing ? 0 : 2.5 }} />
      ) : isFailed ? (
        <CircleAlert size={16} style={{ color: 'inherit', flexShrink: 0, marginTop: isEditing ? 4 : 0 }} />
      ) : isEditing ? (
        <Pencil size={16} style={{ flexShrink: 0, marginTop: 4, marginLeft: 20 }} />
      ) : (
        <Hourglass size={16} style={{ flexShrink: 0 }} />
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
                <X size={14} />
              </IconButton>
            </Tooltip>
            <Tooltip title="Save (Enter)">
              <IconButton
                size="small"
                onClick={handleSaveEdit}
                color="primary"
                sx={{ p: 0.25 }}
              >
                <Check size={14} />
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
              <Pencil
                className="edit-hint"
                size={14}
                style={{
                  opacity: 0,
                  transition: 'opacity 0.15s',
                  flexShrink: 0,
                }}
              />
            )}
          </Box>
          {isFailed && (
            <Typography variant="caption" sx={{ color: 'error.main' }}>
              {entry.nextRetryAt ? (
                (() => {
                  const secondsUntilRetry = Math.max(0, Math.ceil((entry.nextRetryAt - Date.now()) / 1000))
                  return secondsUntilRetry > 0
                    ? `Failed - retrying in ${secondsUntilRetry}s`
                    : 'Failed - retrying now...'
                })()
              ) : (
                'Failed - will retry'
              )}
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
                <Zap size={14} />
              ) : (
                <ListStart size={14} />
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
              <X size={16} />
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
  onFileUpload,
  onImagePaste,
}) => {
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const editTextareaRef = useRef<HTMLTextAreaElement>(null)
  const fileInputRef = useRef<HTMLInputElement>(null)
  const [sendingId, setSendingId] = useState<string | null>(null)
  // Pending attachments that will be sent with the message
  const [attachments, setAttachments] = useState<PendingAttachment[]>([])
  const [isUploading, setIsUploading] = useState(false)

  // Use onFileUpload if provided, otherwise fall back to onImagePaste for backwards compat
  const handleFileUploadCallback = onFileUpload || onImagePaste

  // Check if we're on a mobile device for camera support
  const isMobile = /Android|webOS|iPhone|iPad|iPod|BlackBerry|IEMobile|Opera Mini/i.test(navigator.userAgent)
  const [historyMenuAnchor, setHistoryMenuAnchor] = useState<null | HTMLElement>(null)
  const [showHistoryHint, setShowHistoryHint] = useState(false)
  const [historySearchQuery, setHistorySearchQuery] = useState('')
  const [isOnline, setIsOnline] = useState(navigator.onLine)
  const [showQueue, setShowQueue] = useState(true)
  const [interruptMode, setInterruptMode] = useState(false) // false = queue after (default), true = interrupt

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

  // Backend queue processing is ALWAYS enabled
  // The backend handles processing prompts after they're synced via usePromptHistory
  // Frontend only needs to save to history and sync - no direct sending or retry logic needed

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
    // Allow sending if there's content OR attachments
    if ((!content && attachments.length === 0) || disabled) return

    // Check if any attachments are still uploading
    const uploadingAttachments = attachments.filter(a => a.uploadStatus === 'uploading' || a.uploadStatus === 'pending')
    if (uploadingAttachments.length > 0) {
      // Wait for uploads to complete - don't send yet
      // TODO: Could show a snackbar message here
      return
    }

    // Build the message with attachment paths prepended
    const uploadedAttachments = attachments.filter(a => a.uploadStatus === 'uploaded' && a.path)
    const attachmentPaths = uploadedAttachments.map(a => a.path!).join(' ')
    const fullContent = attachmentPaths
      ? (content ? `${attachmentPaths} ${content}` : attachmentPaths)
      : content

    // Add to queue with pending status, passing interrupt mode
    saveToHistory(fullContent, interruptMode)
    clearDraft()

    // Clear attachments after adding to queue
    setAttachments(prev => {
      // Revoke object URLs to prevent memory leaks
      prev.forEach(a => {
        if (a.previewUrl) URL.revokeObjectURL(a.previewUrl)
      })
      return []
    })
    // Backend handles processing after sync - no need to call processQueue
  }, [draft, disabled, attachments, saveToHistory, clearDraft, interruptMode])

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
      const content = draft.trim()

      // Allow sending if there's content OR attachments
      if ((!content && attachments.length === 0) || disabled) return

      // Check if any attachments are still uploading
      const uploadingAttachments = attachments.filter(a => a.uploadStatus === 'uploading' || a.uploadStatus === 'pending')
      if (uploadingAttachments.length > 0) {
        // Wait for uploads to complete
        return
      }

      // Build the message with attachment paths prepended
      const uploadedAttachments = attachments.filter(a => a.uploadStatus === 'uploaded' && a.path)
      const attachmentPaths = uploadedAttachments.map(a => a.path!).join(' ')
      const fullContent = attachmentPaths
        ? (content ? `${attachmentPaths} ${content}` : attachmentPaths)
        : content

      // Add to queue with pending status
      saveToHistory(fullContent, useInterrupt)
      clearDraft()

      // Clear attachments
      setAttachments(prev => {
        prev.forEach(a => {
          if (a.previewUrl) URL.revokeObjectURL(a.previewUrl)
        })
        return []
      })
      // Backend handles processing after sync
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
  }, [draft, disabled, attachments, saveToHistory, clearDraft, navigateUp, navigateDown])

  // Add a file as an attachment (queues for upload, uploads if online)
  const addFileAsAttachment = useCallback((file: File): string => {
    // Determine type based on file mime type
    const type: PendingAttachment['type'] = file.type.startsWith('image/')
      ? 'image'
      : file.type.startsWith('text/') || file.name.match(/\.(txt|md|json|xml|csv|log|js|ts|py|java|c|cpp|h|hpp|css|html|yaml|yml)$/i)
        ? 'text'
        : 'file'

    // Create preview URL for images
    let previewUrl: string | undefined
    if (type === 'image') {
      previewUrl = URL.createObjectURL(file)
    }

    const id = `${Date.now()}-${Math.random().toString(36).slice(2)}`
    const attachment: PendingAttachment = {
      id,
      name: file.name,
      file, // Store the file for offline upload later
      type,
      previewUrl,
      uploadStatus: 'pending',
    }
    setAttachments(prev => [...prev, attachment])
    return id
  }, [])

  // Upload a single attachment
  const uploadAttachment = useCallback(async (attachmentId: string) => {
    if (!handleFileUploadCallback) return

    setAttachments(prev => prev.map(a =>
      a.id === attachmentId ? { ...a, uploadStatus: 'uploading' as const, error: undefined } : a
    ))

    const attachment = attachments.find(a => a.id === attachmentId)
    if (!attachment?.file) return

    try {
      const filePath = await handleFileUploadCallback(attachment.file)
      if (filePath) {
        setAttachments(prev => prev.map(a =>
          a.id === attachmentId
            ? { ...a, path: filePath, uploadStatus: 'uploaded' as const, file: undefined }
            : a
        ))
      } else {
        setAttachments(prev => prev.map(a =>
          a.id === attachmentId
            ? { ...a, uploadStatus: 'failed' as const, error: 'Upload returned no path' }
            : a
        ))
      }
    } catch (err) {
      const errorMessage = err instanceof Error ? err.message : 'Upload failed'
      setAttachments(prev => prev.map(a =>
        a.id === attachmentId
          ? { ...a, uploadStatus: 'failed' as const, error: errorMessage }
          : a
      ))
    }
  }, [handleFileUploadCallback, attachments])

  // Process pending uploads when online
  const processPendingUploads = useCallback(async () => {
    if (!isOnline || !handleFileUploadCallback) return

    const pendingAttachments = attachments.filter(a => a.uploadStatus === 'pending' && a.file)
    for (const attachment of pendingAttachments) {
      await uploadAttachment(attachment.id)
    }
  }, [isOnline, handleFileUploadCallback, attachments, uploadAttachment])

  // Auto-upload when file is added or when coming back online
  useEffect(() => {
    if (isOnline) {
      const pendingAttachments = attachments.filter(a => a.uploadStatus === 'pending' && a.file)
      if (pendingAttachments.length > 0) {
        processPendingUploads()
      }
    }
  }, [isOnline, attachments])

  // Add a file and trigger upload if online
  const uploadAndAddAttachment = useCallback(async (file: File) => {
    const attachmentId = addFileAsAttachment(file)
    // Upload will be triggered by the useEffect when attachments change
    // This ensures proper state updates
  }, [addFileAsAttachment])

  // Remove an attachment
  const removeAttachment = useCallback((id: string) => {
    setAttachments(prev => {
      const toRemove = prev.find(a => a.id === id)
      if (toRemove?.previewUrl) {
        URL.revokeObjectURL(toRemove.previewUrl)
      }
      return prev.filter(a => a.id !== id)
    })
  }, [])

  // Handle file input change (from browse button)
  const handleFileInputChange = useCallback(async (e: React.ChangeEvent<HTMLInputElement>) => {
    const files = Array.from(e.target.files || [])
    for (const file of files) {
      await uploadAndAddAttachment(file)
    }
    // Reset the input so the same file can be selected again
    if (fileInputRef.current) {
      fileInputRef.current.value = ''
    }
  }, [uploadAndAddAttachment])

  // Open file browser
  const handleBrowseClick = useCallback(() => {
    fileInputRef.current?.click()
  }, [])

  // Handle paste events for images and large text
  const handlePaste = useCallback(async (e: React.ClipboardEvent<HTMLTextAreaElement>) => {
    if (!handleFileUploadCallback) return

    const items = e.clipboardData?.items
    if (!items) return

    // Check for images first
    for (let i = 0; i < items.length; i++) {
      const item = items[i]
      if (item.type.startsWith('image/')) {
        e.preventDefault()
        const blob = item.getAsFile()
        if (blob) {
          const timestamp = new Date().toISOString().replace(/[:.]/g, '-').slice(0, 19)
          const extension = item.type === 'image/png' ? 'png' : 'jpg'
          const file = new File([blob], `pasted-image-${timestamp}.${extension}`, { type: item.type })
          await uploadAndAddAttachment(file)
        }
        return
      }
    }

    // Check for large text paste - convert to text file attachment
    const pastedText = e.clipboardData?.getData('text/plain')
    if (pastedText && pastedText.length > LARGE_TEXT_THRESHOLD) {
      e.preventDefault()
      const timestamp = new Date().toISOString().replace(/[:.]/g, '-').slice(0, 19)
      const file = new File([pastedText], `pasted-text-${timestamp}.txt`, { type: 'text/plain' })
      await uploadAndAddAttachment(file)
      // Add a note to the draft about the attached file
      setDraft(prev => {
        const note = '[Large text pasted as attachment]'
        if (prev.includes(note)) return prev
        const needsSpace = prev.length > 0 && !prev.startsWith(' ') && !prev.startsWith('\n')
        return note + (needsSpace ? ' ' : '') + prev
      })
    }
  }, [handleFileUploadCallback, uploadAndAddAttachment, setDraft])

  // Track drag state for visual feedback
  const [isDraggingOver, setIsDraggingOver] = useState(false)

  // Handle drag enter - show visual feedback
  const handleDragEnter = useCallback((e: React.DragEvent<HTMLTextAreaElement>) => {
    e.preventDefault()
    e.stopPropagation()
    if (handleFileUploadCallback) {
      setIsDraggingOver(true)
    }
  }, [handleFileUploadCallback])

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
    if (handleFileUploadCallback) {
      e.dataTransfer.dropEffect = 'copy'
    }
  }, [handleFileUploadCallback])

  // Handle drop events for files
  const handleDrop = useCallback(async (e: React.DragEvent<HTMLTextAreaElement>) => {
    e.preventDefault()
    e.stopPropagation()
    setIsDraggingOver(false)

    if (!handleFileUploadCallback) return

    const files = Array.from(e.dataTransfer.files)
    for (const file of files) {
      await uploadAndAddAttachment(file)
    }
  }, [handleFileUploadCallback, uploadAndAddAttachment])

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
              <CirclePause size={16} />
            ) : isOnline ? (
              <Cloud size={16} />
            ) : (
              <CloudOff size={16} />
            )}
            <Typography variant="caption" sx={{ flex: 1, fontWeight: 600 }}>
              {editingId
                ? 'Editing - paused from here'
                : isOnline
                  ? 'Message queue (saved locally)'
                  : 'Offline - saved locally, will send when connected'}
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

      {/* Hidden file input for browse functionality */}
      <input
        ref={fileInputRef}
        type="file"
        multiple
        accept="image/*,text/*,.txt,.md,.json,.xml,.csv,.log,.js,.ts,.py,.java,.c,.cpp,.h,.hpp,.css,.html,.yaml,.yml,.pdf,.doc,.docx"
        onChange={handleFileInputChange}
        style={{ display: 'none' }}
      />

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

      {/* Attachments display */}
      {attachments.length > 0 && (
        <Box
          sx={{
            display: 'flex',
            flexWrap: 'wrap',
            gap: 0.75,
            mb: 1,
            p: 1,
            borderRadius: 1.5,
            border: '1px solid',
            borderColor: 'divider',
            bgcolor: (theme) => alpha(theme.palette.background.paper, 0.5),
          }}
        >
          {attachments.map((attachment) => (
            <Chip
              key={attachment.id}
              size="small"
              icon={
                attachment.uploadStatus === 'uploading' ? (
                  <CircularProgress size={14} sx={{ ml: 0.5 }} />
                ) : attachment.uploadStatus === 'failed' ? (
                  <CircleAlert size={16} />
                ) : attachment.type === 'image' ? (
                  attachment.previewUrl ? (
                    <Box
                      component="img"
                      src={attachment.previewUrl}
                      alt=""
                      sx={{
                        width: 20,
                        height: 20,
                        objectFit: 'cover',
                        borderRadius: 0.5,
                        ml: 0.5,
                      }}
                    />
                  ) : (
                    <Image size={16} />
                  )
                ) : attachment.type === 'text' ? (
                  <FileText size={16} />
                ) : (
                  <Paperclip size={16} />
                )
              }
              label={
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
                  <Typography
                    variant="caption"
                    sx={{
                      maxWidth: 120,
                      overflow: 'hidden',
                      textOverflow: 'ellipsis',
                      whiteSpace: 'nowrap',
                    }}
                  >
                    {attachment.name}
                  </Typography>
                  {attachment.uploadStatus === 'pending' && !isOnline && (
                    <CloudOff size={12} />
                  )}
                </Box>
              }
              onDelete={() => removeAttachment(attachment.id)}
              sx={{
                bgcolor: attachment.uploadStatus === 'failed'
                  ? (theme) => alpha(theme.palette.error.main, 0.1)
                  : attachment.uploadStatus === 'pending'
                    ? (theme) => alpha(theme.palette.warning.main, 0.1)
                    : (theme) => alpha(theme.palette.primary.main, 0.1),
                borderColor: attachment.uploadStatus === 'failed'
                  ? 'error.main'
                  : attachment.uploadStatus === 'pending'
                    ? 'warning.main'
                    : 'primary.main',
                border: '1px solid',
                '& .MuiChip-deleteIcon': {
                  fontSize: 16,
                },
              }}
            />
          ))}
        </Box>
      )}

      {/* Input container */}
      <Box
        sx={{
          display: 'flex',
          flexDirection: 'column',
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
        {/* Textarea - full width at top */}
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
            width: '100%',
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
            minHeight: 50,
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

        {/* Buttons row at bottom */}
        <Box
          sx={{
            display: 'flex',
            alignItems: 'center',
            gap: 0.5,
            mt: 1,
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
                <History size={20} />
              </IconButton>
            </Tooltip>
          )}

          {/* Attach file button */}
          {handleFileUploadCallback && (
            <Tooltip title="Attach file">
              <IconButton
                size="small"
                onClick={handleBrowseClick}
                disabled={disabled}
                sx={{
                  color: 'text.secondary',
                  flexShrink: 0,
                  '&:hover': {
                    color: 'primary.main',
                  },
                }}
              >
                <Paperclip size={20} />
              </IconButton>
            </Tooltip>
          )}

          {/* Camera button (mobile only) */}
          {handleFileUploadCallback && isMobile && (
            <Tooltip title="Take photo">
              <IconButton
                size="small"
                onClick={() => {
                  // Create a temporary input with capture for camera
                  const input = document.createElement('input')
                  input.type = 'file'
                  input.accept = 'image/*'
                  input.capture = 'environment' // Use rear camera by default
                  input.onchange = async (e) => {
                    const files = (e.target as HTMLInputElement).files
                    if (files && files.length > 0) {
                      await uploadAndAddAttachment(files[0])
                    }
                  }
                  input.click()
                }}
                disabled={disabled}
                sx={{
                  color: 'text.secondary',
                  flexShrink: 0,
                  '&:hover': {
                    color: 'primary.main',
                  },
                }}
              >
                <Camera size={20} />
              </IconButton>
            </Tooltip>
          )}

          {/* Spacer */}
          <Box sx={{ flex: 1 }} />

          {/* Offline indicator */}
          {!isOnline && (
            <Tooltip title="You're offline - messages will queue and send when connected">
              <CloudOff size={20} style={{ flexShrink: 0 }} />
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
                  Keyboard: Enter = queue | Ctrl+Enter = interrupt
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
                <Zap size={20} />
              ) : (
                <ListStart size={20} />
              )}
            </IconButton>
          </Tooltip>

          {/* Send button */}
          {(() => {
            const hasContent = draft.trim().length > 0
            const uploadedAttachments = attachments.filter(a => a.uploadStatus === 'uploaded')
            const pendingUploads = attachments.filter(a => a.uploadStatus === 'uploading' || a.uploadStatus === 'pending')
            const canSend = (hasContent || uploadedAttachments.length > 0) && pendingUploads.length === 0 && !disabled

            return (
              <Tooltip
                title={
                  pendingUploads.length > 0
                    ? `Uploading ${pendingUploads.length} file${pendingUploads.length > 1 ? 's' : ''}...`
                    : 'Add to queue (Enter = queue, Ctrl+Enter = interrupt)'
                }
              >
                <span>
                  <IconButton
                    onClick={handleSend}
                    disabled={!canSend}
                    color={canSend ? 'secondary' : 'primary'}
                    sx={{
                      flexShrink: 0,
                      width: 30,
                      height: 30,
                      bgcolor: canSend ? 'secondary.main' : 'transparent',
                      color: canSend ? 'secondary.contrastText' : 'text.secondary',
                      '&:hover': {
                        bgcolor: canSend ? 'secondary.dark' : undefined,
                      },
                      '&.Mui-disabled': {
                        bgcolor: pendingUploads.length > 0 ? (theme) => alpha(theme.palette.secondary.main, 0.3) : 'transparent',
                        color: 'text.disabled',
                      },
                    }}
                  >
                    {pendingUploads.length > 0 ? (
                      <CircularProgress size={16} sx={{ color: 'secondary.main' }} />
                    ) : (
                      <SendHorizontal size={18} />
                    )}
                  </IconButton>
                </span>
              </Tooltip>
            )
          })()}
        </Box>
      </Box>

      {/* Keyboard hint */}
      <Box
        sx={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          gap: 2,
          mt: 0.75,
          px: 0.5,
          flexWrap: 'wrap',
        }}
      >
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
          <ListStart size={12} style={{ opacity: 0.6 }} />
          <Typography variant="caption" sx={{ color: 'text.secondary', opacity: 0.7 }}>
            Enter = queue
          </Typography>
        </Box>
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
          <Zap size={12} style={{ opacity: 0.6 }} />
          <Typography variant="caption" sx={{ color: 'text.secondary', opacity: 0.7 }}>
            Ctrl+Enter = interrupt
          </Typography>
        </Box>
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
          <SendHorizontal size={12} style={{ opacity: 0.6 }} />
          <Typography variant="caption" sx={{ color: 'text.secondary', opacity: 0.7 }}>
            Shift+Enter = new line
          </Typography>
        </Box>
        {hasHistory && (
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
            <History size={12} style={{ opacity: 0.6 }} />
            <Typography variant="caption" sx={{ color: 'text.secondary', opacity: 0.7 }}>
              ↑/↓ history
            </Typography>
          </Box>
        )}
        {queuedMessages.length > 0 && (
          <Typography variant="caption" sx={{ color: 'primary.main', fontWeight: 500 }}>
            {queuedMessages.length} in queue
          </Typography>
        )}
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
                  <Search size={18} />
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
                    <Pin size={14} />
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
                          <Pin size={20} />
                        ) : (
                          <CheckCircle size={20} style={{ opacity: 0.6 }} />
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
                            <Pin size={16} />
                          ) : (
                            <PinOff size={16} />
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
