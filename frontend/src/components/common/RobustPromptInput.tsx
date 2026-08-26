/**
 * RobustPromptInput - A beautiful, reliable prompt input for agent sessions
 *
 * Features:
 * - Message queue: keep typing while previous messages send
 * - Queue multiple messages while offline, send when connection returns
 * - Auto-expanding textarea that grows with content
 * - Draft auto-save to localStorage (never lose a prompt)
 * - Visual queue showing pending/sending/failed messages
 * - Retry mechanism for failed sends
 * - Recovery on page reload
 */

import React, { FC, useRef, useEffect, useState, useCallback, useMemo, useLayoutEffect } from 'react'
import {
  Box,
  IconButton,
  CircularProgress,
  Tooltip,
  Typography,
  alpha,
  Collapse,
  LinearProgress,
  Chip,
} from '@mui/material'
import {
  SendHorizontal,
  ListStart,
  CircleAlert,
  CheckCircle,
  Hourglass,
  CloudOff,
  Pencil,
  Check,
  X,
  GripVertical,
  Zap,
  Paperclip,
  Camera,
  Square,
  MessageCircle,
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
import { classifyPromptQueueEntry } from '../../utils/promptQueueStatus'
import {
  nextQueueVisibilityDeadline,
  selectVisibleQueuedPrompts,
} from '../../utils/promptQueueVisibility'
import { getChatColors } from '../session/chatStyles'
import ChatAttachmentTray from './ChatAttachmentTray'
import ContextMenuModal from '../widgets/ContextMenuModal'
import SandboxComposerSuggestions from './SandboxComposerSuggestions'
import { useSandboxComposerSuggestions } from './useSandboxComposerSuggestions'
import SandboxPromptEditor, {
  getSandboxPromptEditorCursor,
  setSandboxPromptEditorCursor,
} from './SandboxPromptEditor'
import {
  applySandboxComposerSuggestion,
  detectSandboxComposerTrigger,
  SandboxComposerSuggestion,
} from './sandboxComposerSuggestions.logic'
import {
  buildMessageWithAttachments,
  createPendingChatAttachment,
  filesFromClipboard,
  PendingChatAttachment,
  validateChatAttachmentFiles,
} from './chatAttachments'
import {
  appendWorkspaceReviewComments,
  type WorkspaceReviewComment,
} from '../workspace-inspector/workspaceReviewComments'
import { SessionContextUsageIndicator } from './ContextUsageIndicator'

// Threshold for converting large text paste to file attachment (10KB)
const LARGE_TEXT_THRESHOLD = 10 * 1024
const NO_REVIEW_COMMENTS: readonly WorkspaceReviewComment[] = []

interface RobustPromptInputProps {
  sessionId: string
  onSend: (message: string, interrupt?: boolean, attachments?: File[]) => Promise<void | boolean>
  placeholder?: string
  disabled?: boolean
  maxHeight?: number
  /**
   * Fill the height available instead of hugging the text.
   *
   * A phone composing a new task has a whole screen and a keyboard; a card
   * floating in the middle of it wastes both. In fill mode the shell drops its
   * card chrome, the text starts at the top, and the actions dock at the
   * bottom — the shape every mobile compose screen has.
   */
  fill?: boolean
  /** Removes the composer's top corners when an attached drawer is rendered above it. */
  hasAttachedHeader?: boolean
  sendMode?: 'queued' | 'direct'
  inlineImageAttachments?: boolean
  deferredFileAttachments?: boolean
  attachmentAccept?: string
  attachmentMaxBytes?: number
  attachmentMaxCount?: number
  validateAttachment?: (file: File) => string | null
  leadingActions?: React.ReactNode
  trailingActions?: React.ReactNode
  showContextUsage?: boolean
  contextMenuAppId?: string
  formatContextMenuInsert?: (text: string) => string
  autoFocus?: boolean
  // Enables @workspace-path and $skill completion for connected sandboxes.
  enableSandboxCompletions?: boolean
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
  // Called when user clicks the cancel button to stop the agent's current turn
  onCancel?: () => void | Promise<void>
  // Whether the agent is currently processing (has a waiting interaction)
  isAgentBusy?: boolean
  // Whether cancellation is waiting for acknowledgement from the agent
  isCancelling?: boolean
  // Fires synchronously inside handleSend the moment the user submits a
  // prompt, before the local queue persist or the backend sync POST. The
  // parent uses this hook to do optimistic UI updates (e.g. flip the cached
  // session.config.external_agent_status to "starting" so the desktop
  // viewer shows the spinner without waiting for the next 3s poll).
  // Must be cheap and synchronous — runs in the user's click handler.
  onWillSend?: () => void
  reviewComments?: readonly WorkspaceReviewComment[]
  onRemoveReviewComment?: (commentId: string) => void
  onReviewCommentsSent?: () => void
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
  handleRestartAgent: () => void
  isRestarting: boolean
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
  handleRestartAgent,
  isRestarting,
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
  // Classification (transient vs crashed vs stuck → Restart) is shared with
  // SessionPromptQueue via classifyPromptQueueEntry so both queues agree.
  const { isCrashed, isTransientFailure, isStuckTransient, showRestart } = classifyPromptQueueEntry({
    status: entry.status,
    errorMessage: entry.errorMessage,
    nextRetryAtMs: entry.nextRetryAt,
    retryCount: entry.retryCount,
  })
  const failColor = showRestart ? 'error.main' : isTransientFailure ? 'warning.main' : 'error.main'

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
        gap: 0.75,
        px: 1.5,
        py: isEditing ? 1 : 0.75,
        borderBottom: index < totalCount - 1 ? '1px solid' : 'none',
        borderColor: (theme) => getChatColors(theme).border,
        bgcolor: isDragging
          ? (theme) => alpha(theme.palette.primary.main, 0.12)
          : isEditing
            ? (theme) => alpha(theme.palette.text.primary, 0.05)
            : isFailed
              ? (theme) => alpha(
                  (isTransientFailure && !showRestart) ? theme.palette.warning.main : theme.palette.error.main,
                  0.08,
                )
              : 'transparent',
        transition: 'background-color 0.2s',
        '&:hover': !isEditing && !isSending && !isDragging ? {
          bgcolor: (theme) => alpha(theme.palette.text.primary, 0.025),
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
            opacity: 0.3,
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
        <Hourglass size={14} style={{ flexShrink: 0, opacity: 0.58 }} />
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
          {/* minWidth: 0 lets the Typography ellipsise instead of forcing the
              row to its intrinsic min-content width. On mobile the latter
              pushed the actions box (delete button) past the queue container's
              `overflow: hidden` clip so users couldn't dismiss stuck items.
              See design/2026-04-30-queue-and-other-stuck-state-bugs.md. */}
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, minWidth: 0 }}>
            <Typography
              variant="body2"
              sx={{
                overflow: 'hidden',
                textOverflow: 'ellipsis',
                whiteSpace: 'nowrap',
                color: isFailed ? failColor : 'text.primary',
                fontSize: '0.8125rem',
                flex: 1,
                minWidth: 0,
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
            <Box>
              <Typography variant="caption" sx={{ color: failColor, display: 'block', fontWeight: showRestart ? 600 : 'inherit' }}>
                {isCrashed ? (
                  'The assistant stopped unexpectedly. Click Restart to recover.'
                ) : isStuckTransient ? (
                  "The assistant isn't responding. Click Restart to recover."
                ) : entry.nextRetryAt ? (
                  (() => {
                    const secondsUntilRetry = Math.max(0, Math.ceil((entry.nextRetryAt - Date.now()) / 1000))
                    if (isTransientFailure) {
                      return secondsUntilRetry > 0
                        ? `Waiting for the assistant — retrying in ${secondsUntilRetry}s`
                        : 'Waiting for the assistant — retrying now...'
                    }
                    return secondsUntilRetry > 0
                      ? `Failed - retrying in ${secondsUntilRetry}s`
                      : 'Failed - retrying now...'
                  })()
                ) : (
                  isTransientFailure ? 'Waiting for the assistant' : 'Failed - will retry'
                )}
              </Typography>
              {showRestart && (
                <Box sx={{ mt: 0.5 }}>
                  <Box
                    component="button"
                    onClick={(e: React.MouseEvent) => {
                      e.stopPropagation()
                      handleRestartAgent()
                    }}
                    disabled={isRestarting}
                    sx={{
                      display: 'inline-flex',
                      alignItems: 'center',
                      gap: 0.5,
                      px: 1,
                      py: 0.25,
                      border: '1px solid',
                      borderColor: 'error.main',
                      borderRadius: 1,
                      bgcolor: 'transparent',
                      color: 'error.main',
                      cursor: isRestarting ? 'wait' : 'pointer',
                      fontSize: '0.75rem',
                      fontFamily: 'inherit',
                      '&:hover': !isRestarting ? {
                        bgcolor: (theme) => alpha(theme.palette.error.main, 0.08),
                      } : undefined,
                      '&:disabled': { opacity: 0.6 },
                    }}
                  >
                    {isRestarting ? (
                      <>
                        <CircularProgress size={10} sx={{ color: 'error.main' }} />
                        Restarting...
                      </>
                    ) : (
                      'Restart'
                    )}
                  </Box>
                </Box>
              )}
              {entry.errorMessage && !isCrashed && (
                <Typography
                  variant="caption"
                  sx={{
                    color: failColor,
                    opacity: 0.8,
                    display: 'block',
                    whiteSpace: 'normal',
                    wordBreak: 'break-word',
                    mt: 0.25,
                  }}
                  title={entry.errorMessage}
                >
                  {entry.errorMessage}
                </Typography>
              )}
            </Box>
          )}
        </Box>
      )}

      {/* Actions - only show when not editing.
          flexShrink: 0 keeps the delete button visible when the queue panel
          is narrow (e.g. mobile). Without it the Typography content above
          could squeeze the actions past the queue container's overflow:hidden
          clip and leave queue items unrecoverable. */}
      {!isEditing && !isSending && (
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.25, flexShrink: 0 }}>
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
                color: entry.interrupt !== false ? 'warning.main' : 'text.secondary',
                opacity: 0.62,
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
  fill = false,
  hasAttachedHeader = false,
  specTaskId,
  projectId,
  apiClient,
  onHeightChange,
  appendText,
  onFileUpload,
  onImagePaste,
  onCancel,
  isAgentBusy = false,
  isCancelling = false,
  onWillSend,
  sendMode = 'queued',
  inlineImageAttachments = false,
  deferredFileAttachments = false,
  attachmentAccept,
  attachmentMaxBytes,
  attachmentMaxCount,
  validateAttachment,
  leadingActions,
  trailingActions,
  showContextUsage = false,
  contextMenuAppId,
  formatContextMenuInsert,
  autoFocus = false,
  enableSandboxCompletions = false,
  reviewComments = NO_REVIEW_COMMENTS,
  onRemoveReviewComment,
  onReviewCommentsSent,
}) => {
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const sandboxEditorRef = useRef<HTMLDivElement>(null)
  const pendingComposerCursorRef = useRef<number | null>(null)
  const editTextareaRef = useRef<HTMLTextAreaElement>(null)
  const fileInputRef = useRef<HTMLInputElement>(null)
  const [sendingId, setSendingId] = useState<string | null>(null)
  const [isDirectSending, setIsDirectSending] = useState(false)
  const [isRestartingAgent, setIsRestartingAgent] = useState(false)
  const [composerCursor, setComposerCursor] = useState(0)
  const [composerFocused, setComposerFocused] = useState(false)
  const [composerSelectedIndex, setComposerSelectedIndex] = useState(0)
  // Pending attachments that will be sent with the message
  const [attachments, setAttachments] = useState<PendingChatAttachment[]>([])
  const [attachmentError, setAttachmentError] = useState<string | null>(null)
  const attachmentsRef = useRef<PendingChatAttachment[]>([])
  const attachmentUploadsInFlightRef = useRef(new Set<string>())
  const reviewCommentsRef = useRef(reviewComments)
  const onReviewCommentsSentRef = useRef(onReviewCommentsSent)
  reviewCommentsRef.current = reviewComments
  onReviewCommentsSentRef.current = onReviewCommentsSent

  // Use onFileUpload if provided, otherwise fall back to onImagePaste for backwards compat
  const handleFileUploadCallback = onFileUpload || onImagePaste
  const attachmentsEnabled = inlineImageAttachments || deferredFileAttachments || !!handleFileUploadCallback
  const inputDisabled = disabled || isDirectSending

  // Check if we're on a mobile device for camera support
  const isMobile = /Android|webOS|iPhone|iPad|iPod|BlackBerry|IEMobile|Opera Mini/i.test(navigator.userAgent)
  const interruptShortcut = /Mac|iPhone|iPad|iPod/i.test(navigator.platform) ? '⌘Enter' : 'Ctrl+Enter'
  const [isOnline, setIsOnline] = useState(navigator.onLine)
  const [showQueue, setShowQueue] = useState(true)
  const [interruptMode, setInterruptMode] = useState(false) // false = queue after (default), true = interrupt

  // Editing state for queued messages
  const [editingId, setEditingId] = useState<string | null>(null)
  const [editingContent, setEditingContent] = useState('')
  const [editingOriginalContent, setEditingOriginalContent] = useState('')
  const [editingInterruptMode, setEditingInterruptMode] = useState(false)

  const {
    draft,
    setDraft,
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
  } = usePromptHistory({ sessionId, specTaskId, projectId, apiClient })

  const focusPromptEditor = useCallback(() => {
    if (enableSandboxCompletions) sandboxEditorRef.current?.focus()
    else textareaRef.current?.focus()
  }, [enableSandboxCompletions])

  const composerTrigger = useMemo(
    () => enableSandboxCompletions && composerFocused
      ? detectSandboxComposerTrigger(draft, composerCursor)
      : null,
    [composerCursor, composerFocused, draft, enableSandboxCompletions],
  )
  const composerSuggestions = useSandboxComposerSuggestions(
    sessionId,
    composerTrigger,
    enableSandboxCompletions,
  )

  useEffect(() => {
    setComposerSelectedIndex(0)
  }, [composerTrigger?.kind, composerTrigger?.query])

  const selectComposerSuggestion = useCallback((suggestion: SandboxComposerSuggestion) => {
    if (!composerTrigger) return
    const next = applySandboxComposerSuggestion(draft, composerTrigger, suggestion)
    pendingComposerCursorRef.current = next.cursor
    setDraft(next.text)
    setComposerCursor(next.cursor)
  }, [composerTrigger, draft, setDraft])

  useLayoutEffect(() => {
    const cursor = pendingComposerCursorRef.current
    if (cursor === null) return
    pendingComposerCursorRef.current = null
    setComposerCursor(cursor)
    if (enableSandboxCompletions && sandboxEditorRef.current) {
      const frame = requestAnimationFrame(() => {
        if (!sandboxEditorRef.current) return
        sandboxEditorRef.current.focus()
        setSandboxPromptEditorCursor(sandboxEditorRef.current, cursor)
      })
      return () => cancelAnimationFrame(frame)
    } else {
      textareaRef.current?.focus()
      textareaRef.current?.setSelectionRange(cursor, cursor)
    }
  }, [draft, enableSandboxCompletions])

  // Canonical "still actionable in the queue" list, failed-first. Computed in ONE
  // place so every consumer — queue display, the interrupt toggle, the empty-Enter
  // interrupt-promotion, the client-side pump — operates on the SAME set.
  // Previously each site recomputed [...failedPrompts, ...pendingPrompts]
  // independently, and the promotion path diverged to pendingPrompts-only — so it
  // silently skipped a prompt the instant the backend deferred it to 'failed'
  // (which a long current turn does almost immediately).
  //
  // We exclude 'sending' (backend has dispatched to Zed, awaiting first
  // message_added): once a message is in flight it can't be promoted/toggled, and
  // showing it in the queue until the *next* sync flips it to 'sent' is the lag
  // that makes a just-sent prompt linger. Dropping 'sending' hides it optimistically
  // the moment dispatch is confirmed; if it later bounces it returns via 'failed'.
  // See design/2026-06-19-incident-interrupt-during-boot-context-loss.md.
  const queuedPrompts = [...failedPrompts, ...pendingPrompts].filter(p => p.status !== 'sending')

  // Track previous appendText to detect changes
  const prevAppendTextRef = useRef<string | undefined>(undefined)

  // Re-entrancy guard for the client-side queue pump below.
  const processingRef = useRef(false)

  // Backend queue processing only kicks in for spec-task composers: that path
  // persists prompts via usePromptHistory and the server dispatches them once
  // synced (keyed on spec_task_id). Plain external-agent sessions — the
  // project "Project Desktop" (TeamDesktopPage) and org-worker chat — have no
  // spec_task_id, so the composer must pump the queue itself by calling
  // onSend, which the parent routes to streaming.NewInference → the running
  // Zed agent. Without this fallback every queued message just sits forever as
  // "Message queue (saved locally)" and the agent never sees it. (Regression
  // from 53b336e01, which deleted the client-side pump on the assumption that
  // the backend queue was always enabled.)
  const backendQueueEnabled = !!(specTaskId && projectId && apiClient)

  useEffect(() => {
    attachmentsRef.current = attachments
  }, [attachments])

  useEffect(() => () => {
    attachmentsRef.current.forEach((attachment) => {
      if (attachment.previewUrl) URL.revokeObjectURL(attachment.previewUrl)
    })
  }, [])

  const processQueue = useCallback(async () => {
    if (sendMode === 'direct') return
    // Spec-task sessions: the backend owns dispatch after sync.
    if (backendQueueEnabled) return
    // Prevent concurrent processing.
    if (processingRef.current || !isOnline || disabled) return

    // Interrupt-mode messages first, then queue-mode, each oldest-first.
    const sortedQueue = [...queuedPrompts].sort((a, b) => {
      const aInterrupt = a.interrupt !== false
      const bInterrupt = b.interrupt !== false
      if (aInterrupt && !bInterrupt) return -1
      if (!aInterrupt && bInterrupt) return 1
      return a.timestamp - b.timestamp
    })

    // If editing, block the edited message and everything after it so ordering
    // is preserved while the user revises.
    let editingIndex = -1
    if (editingId) {
      editingIndex = sortedQueue.findIndex(m => m.id === editingId)
    }

    const nextToSend = sortedQueue.find((m, index) => {
      if (m.id === sendingId) return false
      if (m.status === 'sent') return false
      if (editingIndex !== -1 && index >= editingIndex) return false
      return true
    })

    if (!nextToSend) return

    processingRef.current = true
    setSendingId(nextToSend.id)

    try {
      // interrupt=true interrupts the agent's current turn; false queues after.
      const sent = await onSend(nextToSend.content, nextToSend.interrupt !== false)
      if (sent === false) throw new Error('Prompt was not sent')
      markAsSent(nextToSend.id)
    } catch (error) {
      console.error('Failed to send message:', error)
      markAsFailed(nextToSend.id)
    } finally {
      setSendingId(null)
      processingRef.current = false
    }
  }, [sendMode, backendQueueEnabled, isOnline, disabled, queuedPrompts, sendingId, editingId, onSend, markAsSent, markAsFailed])

  // Pump the queue when messages are pending and we're online.
  useEffect(() => {
    if (isOnline && (pendingPrompts.length > 0 || failedPrompts.length > 0) && !processingRef.current) {
      const timer = setTimeout(processQueue, 500)
      return () => clearTimeout(timer)
    }
  }, [isOnline, pendingPrompts.length, failedPrompts.length, processQueue])

  // Continue pumping after each send completes.
  useEffect(() => {
    if (!sendingId && isOnline && (pendingPrompts.length > 0 || failedPrompts.length > 0)) {
      const timer = setTimeout(processQueue, 300)
      return () => clearTimeout(timer)
    }
  }, [sendingId, isOnline, pendingPrompts.length, failedPrompts.length, processQueue])

  // Handle text appended by a parent surface (e.g., uploaded file paths).
  useEffect(() => {
    if (appendText && appendText !== prevAppendTextRef.current) {
      // Strip any unique key suffix (format: "text#123")
      const textToAppend = appendText.replace(/#\d+$/, '')
      const needsSpace = draft.length > 0 && !/\s$/.test(draft)
      setDraft(draft + (needsSpace ? ' ' : '') + textToAppend)
      prevAppendTextRef.current = appendText
      // Focus the textarea
      focusPromptEditor()
    }
  }, [appendText, setDraft, draft, focusPromptEditor])

  useEffect(() => {
    if (autoFocus && !inputDisabled) focusPromptEditor()
  }, [autoFocus, inputDisabled, sessionId, focusPromptEditor])

  const handleContextMenuInsert = useCallback((text: string) => {
    const insertedText = formatContextMenuInsert ? formatContextMenuInsert(text) : text
    const lastAtIndex = draft.lastIndexOf('@')
    setDraft(
      lastAtIndex >= 0
        ? draft.substring(0, lastAtIndex) + insertedText
        : draft + insertedText,
    )
    focusPromptEditor()
  }, [draft, formatContextMenuInsert, setDraft, focusPromptEditor])

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
    const editor = enableSandboxCompletions ? sandboxEditorRef.current : textareaRef.current
    if (!editor) return
    // In fill mode the height comes from the flex layout, and writing an inline
    // height here would collapse it back to the content on every keystroke.
    if (fill) return

    const oldHeight = editor.offsetHeight
    editor.style.height = 'auto'
    const newHeight = Math.min(Math.max(editor.scrollHeight, 40), maxHeight)
    editor.style.height = `${newHeight}px`

    // Notify parent if height changed
    if (oldHeight !== newHeight && onHeightChange) {
      onHeightChange()
    }
  }, [enableSandboxCompletions, fill, maxHeight, onHeightChange])

  useEffect(() => {
    adjustHeight()
  }, [draft, adjustHeight])

  // Notify parent when queue changes (affects overall height)
  const queueLength = queuedPrompts.length
  useEffect(() => {
    if (onHeightChange) {
      // Small delay to allow Collapse animation to start
      const timer = setTimeout(onHeightChange, 50)
      return () => clearTimeout(timer)
    }
  }, [queueLength, onHeightChange])

  const clearCurrentAttachments = useCallback(() => {
    setAttachments((current) => {
      current.forEach((attachment) => {
        if (attachment.previewUrl) URL.revokeObjectURL(attachment.previewUrl)
      })
      return []
    })
  }, [])

  const submitDraft = useCallback(async (interrupt: boolean) => {
    const content = draft.trim()
    if (
      (!content && attachments.length === 0 && reviewComments.length === 0) ||
      inputDisabled ||
      (sendMode === 'direct' && isAgentBusy)
    ) return

    const uploadingAttachments = attachments.filter(a => a.uploadStatus === 'uploading' || a.uploadStatus === 'pending')
    if (uploadingAttachments.length > 0) return
    if (attachments.some((attachment) => attachment.uploadStatus === 'failed')) return

    const fullContent = appendWorkspaceReviewComments(
      buildMessageWithAttachments(content, attachments),
      reviewCommentsRef.current,
    )

    // Optimistic UI hook: fires synchronously before the queue persist /
    // backend sync POST so the parent can flip a paused desktop's cached
    // status to "starting" and render the spinner without waiting for the
    // 3s session poll. Errors here must not block the send.
    if (onWillSend) {
      try {
        onWillSend()
      } catch (e) {
        console.warn('[RobustPromptInput] onWillSend threw:', e)
      }
    }

    if (sendMode === 'direct') {
      const inlineImages = attachments.flatMap((attachment) => attachment.file ? [attachment.file] : [])
      setIsDirectSending(true)
      try {
        const sent = await onSend(
          appendWorkspaceReviewComments(content, reviewCommentsRef.current),
          true,
          inlineImages,
        )
        if (sent === false) return
        clearDraft()
        clearCurrentAttachments()
        onReviewCommentsSentRef.current?.()
      } catch (error) {
        console.error('Failed to send prompt:', error)
      } finally {
        setIsDirectSending(false)
      }
      return
    }

    saveToHistory(fullContent, interrupt)
    clearDraft()
    clearCurrentAttachments()
    onReviewCommentsSentRef.current?.()
  }, [draft, attachments, inputDisabled, sendMode, isAgentBusy, onSend, clearDraft, clearCurrentAttachments, saveToHistory, onWillSend, reviewComments.length])

  const handleSend = useCallback(async () => {
    await submitDraft(interruptMode)
  }, [interruptMode, submitDraft])

  // Remove from queue: tombstone locally first (instant UI update, prevents
  // re-import on sync), then fire backend DELETE (best effort).
  const handleRemoveFromQueue = useCallback((entryId: string) => {
    removeFromQueue(entryId) // Marks as deleted in localStorage (tombstone)
    if (apiClient) {
      apiClient.v1PromptHistoryDelete(entryId).catch((err: unknown) => {
        console.warn('Failed to delete prompt from backend:', err)
      })
    }
  }, [removeFromQueue, apiClient])

  // Toggle interrupt mode for a queued message
  const handleToggleInterrupt = useCallback((entryId: string) => {
    const entry = queuedPrompts.find(e => e.id === entryId)
    if (entry) {
      updateInterrupt(entryId, entry.interrupt === false)
    }
  }, [queuedPrompts, updateInterrupt])

  // Restart Zed thread after a Claude Agent crash. Calls the backend endpoint
  // which clears the dead acp_thread_id and resets crashed prompts back to
  // pending — the queue then re-dispatches them, Zed creates a fresh thread,
  // and a new Claude Agent ACP wrapper is spawned. The local queue UI catches
  // up via the existing prompt-history poll (~1-2s) so we just disable the
  // button while the request is in flight; we don't optimistically mutate
  // local state because the backend is the source of truth for prompt status.
  const handleRestartAgent = useCallback(() => {
    if (!apiClient || !sessionId || isRestartingAgent) return
    setIsRestartingAgent(true)
    apiClient.v1SessionsRestartAgentCreate(sessionId)
      .catch((err: unknown) => {
        console.error('Failed to restart agent thread:', err)
      })
      .finally(() => {
        setIsRestartingAgent(false)
      })
  }, [apiClient, sessionId, isRestartingAgent])

  // Start editing a queued message.
  // To genuinely pause sending, we remove the entry from the backend queue
  // (DELETE on backend only — keep it locally for the edit UI). On save/cancel,
  // we delete the old entry locally and re-queue as a new entry.
  const handleStartEdit = useCallback((entry: PromptHistoryEntry) => {
    // Don't allow editing a message that's currently being sent
    if (entry.id === sendingId) return

    // Store original content and interrupt mode so we can restore on cancel
    setEditingOriginalContent(entry.content)
    setEditingInterruptMode(entry.interrupt !== false)
    setEditingId(entry.id)
    setEditingContent(entry.content)

    // Remove from backend queue so it can't be sent while editing.
    // Keep the entry locally so the edit UI can render on it.
    if (apiClient) {
      apiClient.v1PromptHistoryDelete(entry.id).catch((err: unknown) => {
        console.warn('Failed to delete prompt from backend during edit:', err)
      })
    }

    // Focus the edit textarea after render
    setTimeout(() => {
      editTextareaRef.current?.focus()
      editTextareaRef.current?.select()
    }, 50)
  }, [sendingId, apiClient])

  // Save edited message — remove old entry, re-queue with new content.
  // The old entry was already deleted from the backend in handleStartEdit.
  const handleSaveEdit = useCallback(() => {
    if (!editingId) return

    const trimmedContent = editingContent.trim()

    // Remove the old local entry (tombstone it)
    removeFromQueue(editingId)

    if (trimmedContent) {
      // Re-queue as a new pending entry with the edited content
      saveToHistory(trimmedContent, editingInterruptMode)
    }
    // If content is empty, don't re-queue (effectively deletes it)

    setEditingId(null)
    setEditingContent('')
    setEditingOriginalContent('')
  }, [editingId, editingContent, editingInterruptMode, removeFromQueue, saveToHistory])

  // Cancel editing — remove old entry, re-queue original content unchanged.
  // The old entry was already deleted from the backend in handleStartEdit.
  const handleCancelEdit = useCallback(() => {
    if (editingId) {
      // Remove old local entry (tombstone it)
      removeFromQueue(editingId)

      // Re-queue the original content as a new pending entry
      if (editingOriginalContent.trim()) {
        saveToHistory(editingOriginalContent, editingInterruptMode)
      }
    }
    setEditingId(null)
    setEditingContent('')
    setEditingOriginalContent('')
  }, [editingId, editingOriginalContent, editingInterruptMode, removeFromQueue, saveToHistory])

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
  // Empty Enter (no draft, no attachments) = promote the OLDEST queued entry to
  // interrupt mode, which dispatches it immediately via the existing sync loop.
  // Equivalent to clicking the lightning icon on that queue item.
  // Oldest-first (not most-recent) so repeated empty-Enter escalates the queue in
  // FIFO order — promoting the newest would dispatch it ahead of older queued
  // messages, reordering the conversation.
  // See design/2026-06-23-queue-drain-out-of-order-dispatch.md.
  const handleKeyDown = useCallback((e: React.KeyboardEvent<HTMLElement>) => {
    if (composerTrigger) {
      if (e.key === 'Escape') {
        e.preventDefault()
        setComposerFocused(false)
        return
      }
      if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
        if (composerSuggestions.items.length === 0) return
        e.preventDefault()
        const direction = e.key === 'ArrowDown' ? 1 : -1
        setComposerSelectedIndex((current) =>
          (current + direction + composerSuggestions.items.length) % composerSuggestions.items.length,
        )
        return
      }
      if ((e.key === 'Enter' || e.key === 'Tab') && composerSuggestions.items.length > 0) {
        e.preventDefault()
        selectComposerSuggestion(
          composerSuggestions.items[Math.min(composerSelectedIndex, composerSuggestions.items.length - 1)],
        )
        return
      }
    }
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      // Ctrl+Enter = interrupt mode, Enter = queue mode
      const useInterrupt = e.ctrlKey || e.metaKey // metaKey for Mac Cmd key
      const content = draft.trim()

      // Empty field: promote most-recent queued entry to interrupt instead of sending nothing.
      if (!content && attachments.length === 0 && reviewComments.length === 0) {
        if (inputDisabled || sendMode === 'direct') return
        // Promote the OLDEST NON-interrupt queued message to interrupt.
        // Scans queuedPrompts (failed + pending) so a deferred message — the one
        // the user is actually trying to escalate — is still a candidate.
        const candidates = queuedPrompts.filter(p =>
          p.interrupt === false &&
          !p.deleted &&
          p.id !== sendingId &&
          p.id !== editingId
        )
        if (candidates.length === 0) return
        const target = candidates.reduce((a, b) => (b.timestamp < a.timestamp ? b : a))
        updateInterrupt(target.id, true)
        return
      }

      void submitDraft(useInterrupt)
      return
    }

  }, [composerSelectedIndex, composerSuggestions.items, composerTrigger, draft, attachments.length, reviewComments.length, inputDisabled, sendMode, submitDraft, queuedPrompts, updateInterrupt, sendingId, editingId, selectComposerSuggestion])

  const addFilesAsAttachments = useCallback((files: File[]) => {
    if (!attachmentsEnabled || files.length === 0) return
    const unsupported = files.flatMap((file) => {
      if (inlineImageAttachments && !file.type.startsWith('image/')) {
        return [{ name: file.name, reason: 'only images can be attached to model chats' }]
      }
      const customReason = validateAttachment?.(file)
      return customReason ? [{ name: file.name, reason: customReason }] : []
    })
    const rejectedNames = new Set(unsupported.map(({ name }) => name))
    const eligibleFiles = files.filter((file) => !rejectedNames.has(file.name))
    const { accepted, rejected } = validateChatAttachmentFiles(
      eligibleFiles,
      attachmentsRef.current.length,
      { maxBytes: attachmentMaxBytes, maxCount: attachmentMaxCount },
    )
    const allRejected = [...unsupported, ...rejected]
    setAttachmentError(
      allRejected.length > 0
        ? allRejected.map(({ name, reason }) => `${name}: ${reason}`).join('. ')
        : null,
    )
    if (accepted.length === 0) return
    setAttachments((current) => [
      ...current,
      ...accepted.map((file) => {
        const attachment = createPendingChatAttachment(file)
        return inlineImageAttachments || deferredFileAttachments
          ? { ...attachment, uploadStatus: 'uploaded' as const }
          : attachment
      }),
    ])
  }, [attachmentsEnabled, attachmentMaxBytes, attachmentMaxCount, deferredFileAttachments, inlineImageAttachments, validateAttachment])

  const uploadAttachment = useCallback(async (attachmentId: string) => {
    if (!handleFileUploadCallback || attachmentUploadsInFlightRef.current.has(attachmentId)) return

    const attachment = attachmentsRef.current.find((candidate) => candidate.id === attachmentId)
    if (!attachment?.file) return

    attachmentUploadsInFlightRef.current.add(attachmentId)

    setAttachments(prev => prev.map(a =>
      a.id === attachmentId ? { ...a, uploadStatus: 'uploading' as const, error: undefined } : a
    ))

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
    } finally {
      attachmentUploadsInFlightRef.current.delete(attachmentId)
    }
  }, [handleFileUploadCallback])

  // Auto-upload when file is added or when coming back online
  useEffect(() => {
    if (!isOnline || !handleFileUploadCallback || inlineImageAttachments || deferredFileAttachments) return
    attachments
      .filter((attachment) => attachment.uploadStatus === 'pending' && attachment.file)
      .forEach((attachment) => void uploadAttachment(attachment.id))
  }, [attachments, deferredFileAttachments, handleFileUploadCallback, inlineImageAttachments, isOnline, uploadAttachment])

  // Remove an attachment
  const removeAttachment = useCallback((id: string) => {
    setAttachments(prev => {
      const toRemove = prev.find(a => a.id === id)
      if (toRemove?.previewUrl) {
        URL.revokeObjectURL(toRemove.previewUrl)
      }
      return prev.filter(a => a.id !== id)
    })
    setAttachmentError(null)
  }, [])

  const retryAttachment = useCallback((id: string) => {
    setAttachments((current) => current.map((attachment) =>
      attachment.id === id && attachment.file
        ? { ...attachment, uploadStatus: 'pending' as const, error: undefined }
        : attachment,
    ))
    setAttachmentError(null)
  }, [])

  // Handle file input change (from browse button)
  const handleFileInputChange = useCallback((e: React.ChangeEvent<HTMLInputElement>) => {
    const files = Array.from(e.target.files || [])
    addFilesAsAttachments(files)
    // Reset the input so the same file can be selected again
    if (fileInputRef.current) {
      fileInputRef.current.value = ''
    }
  }, [addFilesAsAttachments])

  // Open file browser
  const handleBrowseClick = useCallback(() => {
    fileInputRef.current?.click()
  }, [])

  // Clipboard files include screenshots as well as files copied from Finder /
  // Explorer. Read the DataTransfer file list first so PDFs and other binary
  // assets follow the same upload path as images.
  const handlePaste = useCallback((e: React.ClipboardEvent<HTMLElement>) => {
    if (!attachmentsEnabled) return

    const clipboardFiles = filesFromClipboard(e.clipboardData)
    if (clipboardFiles.length > 0) {
      e.preventDefault()
      addFilesAsAttachments(clipboardFiles)
      return
    }

    // Check for large text paste - convert to text file attachment
    const pastedText = e.clipboardData?.getData('text/plain')
    if (handleFileUploadCallback && pastedText && pastedText.length > LARGE_TEXT_THRESHOLD) {
      e.preventDefault()
      const timestamp = new Date().toISOString().replace(/[:.]/g, '-').slice(0, 19)
      const file = new File([pastedText], `pasted-text-${timestamp}.txt`, { type: 'text/plain' })
      addFilesAsAttachments([file])
      // Add a note to the draft about the attached file
      const note = '[Large text pasted as attachment]'
      if (!draft.includes(note)) {
        const needsSpace = draft.length > 0 && !draft.startsWith(' ') && !draft.startsWith('\n')
        setDraft(note + (needsSpace ? ' ' : '') + draft)
      }
    }
  }, [addFilesAsAttachments, attachmentsEnabled, handleFileUploadCallback, setDraft, draft])

  // Track drag state for visual feedback
  const [isDraggingOver, setIsDraggingOver] = useState(false)

  // Handle drag enter - show visual feedback
  const handleDragEnter = useCallback((e: React.DragEvent<HTMLElement>) => {
    e.preventDefault()
    e.stopPropagation()
    if (attachmentsEnabled) {
      setIsDraggingOver(true)
    }
  }, [attachmentsEnabled])

  // Handle drag leave - hide visual feedback
  const handleDragLeave = useCallback((e: React.DragEvent<HTMLElement>) => {
    e.preventDefault()
    e.stopPropagation()
    const nextTarget = e.relatedTarget
    if (nextTarget instanceof Node && e.currentTarget.contains(nextTarget)) return
    setIsDraggingOver(false)
  }, [])

  // Handle drag over - prevent default to allow drop
  const handleDragOver = useCallback((e: React.DragEvent<HTMLElement>) => {
    e.preventDefault()
    e.stopPropagation()
    if (attachmentsEnabled) {
      e.dataTransfer.dropEffect = 'copy'
    }
  }, [attachmentsEnabled])

  // Handle drop events for files
  const handleDrop = useCallback((e: React.DragEvent<HTMLElement>) => {
    e.preventDefault()
    e.stopPropagation()
    setIsDraggingOver(false)

    if (!attachmentsEnabled) return

    const files = Array.from(e.dataTransfer.files)
    addFilesAsAttachments(files)
  }, [addFilesAsAttachments, attachmentsEnabled])

  // Format timestamp
  const truncateContent = (content: string, maxLen: number = 60): string => {
    const firstLine = content.split('\n')[0]
    if (firstLine.length <= maxLen) return firstLine
    return firstLine.substring(0, maxLen - 3) + '...'
  }

  // All queued messages (pending + failed), sorted: interrupt mode first, then queue mode
  const queuedMessages = [...queuedPrompts].sort((a, b) => {
    // Interrupt mode (true or undefined) comes first
    const aInterrupt = a.interrupt !== false
    const bInterrupt = b.interrupt !== false
    if (aInterrupt && !bInterrupt) return -1
    if (!aInterrupt && bInterrupt) return 1
    // Within same mode, maintain original order by timestamp
    return a.timestamp - b.timestamp
  })
  // Only surface the queue panel for prompts that are genuinely waiting — a
  // lone prompt handed to an idle agent is being delivered, not queued, and
  // showing "1 queued" for it is a lie that clears itself a second later.
  // See utils/promptQueueVisibility.ts.
  const visibleQueuedMessages = selectVisibleQueuedPrompts(queuedMessages, { isAgentBusy })
  const hasVisibleQueue = backendQueueEnabled && showQueue && visibleQueuedMessages.length > 0

  // Keep the last non-empty list on screen while the panel collapses, so the
  // header never flashes "0 queued" mid-animation. Collapse unmounts it after.
  const lastVisibleQueueRef = useRef<PromptHistoryEntry[]>(visibleQueuedMessages)
  if (visibleQueuedMessages.length > 0) lastVisibleQueueRef.current = visibleQueuedMessages
  const renderedQueueMessages = hasVisibleQueue ? visibleQueuedMessages : lastVisibleQueueRef.current

  // A prompt hidden by the grace window must still appear if its dispatch never
  // lands, so schedule the single re-render that reveals it. queueRevealAt is a
  // plain timestamp, so this effect re-arms only when the queue itself changes.
  const queueRevealAt = nextQueueVisibilityDeadline(queuedMessages, { isAgentBusy })
  const [, setQueueRevealTick] = useState(0)
  useEffect(() => {
    if (queueRevealAt === null) return
    const delay = Math.max(0, queueRevealAt - Date.now()) + 50
    const timer = setTimeout(() => setQueueRevealTick(n => n + 1), delay)
    return () => clearTimeout(timer)
  }, [queueRevealAt])
  const promptPlaceholder = isDraggingOver
    ? inlineImageAttachments
      ? 'Drop image to attach...'
      : 'Drop file to upload...'
    : isOnline
      ? placeholder
      : sendMode === 'direct'
        ? 'Offline'
        : 'Offline - messages will queue'

  const input = (
    <Box
      className="prompt-input-container"
      data-prompt-input="true"
      sx={{
        position: 'relative',
        width: '100%',
        minWidth: 0,
        ...(fill && { flex: 1, minHeight: 0, display: 'flex', flexDirection: 'column' }),
      }}
    >
      {composerTrigger && (
        <SandboxComposerSuggestions
          trigger={composerTrigger}
          items={composerSuggestions.items}
          loading={composerSuggestions.loading}
          error={composerSuggestions.error}
          selectedIndex={composerSelectedIndex}
          onSelectedIndexChange={setComposerSelectedIndex}
          onSelect={selectComposerSuggestion}
        />
      )}
      {/* Queued messages display. Only rendered when this is an authoritative
          backend-backed queue (spec-task). For plain sessions (org-chat,
          team desktop) the local queue is a non-authoritative ghost — the
          session-keyed SessionPromptQueue is the single source there. */}
      <Collapse in={hasVisibleQueue} unmountOnExit>
        <Box
          sx={{
            borderRadius: hasAttachedHeader ? 0 : '20px 20px 0 0',
            border: '1px solid',
            borderBottom: 0,
            borderColor: (theme) => getChatColors(theme).border,
            bgcolor: (theme) => getChatColors(theme).composerSurface,
            overflow: 'hidden',
          }}
        >
          {/* Queue header */}
          <Box
            sx={{
              display: 'flex',
              alignItems: 'center',
              gap: 0.75,
              px: 2,
              pt: 1.25,
              pb: 0.75,
              bgcolor: 'transparent',
              color: (theme) => getChatColors(theme).subtle,
              borderBottom: '1px solid',
              borderColor: (theme) => getChatColors(theme).border,
            }}
          >
            <ListStart size={14} />
            <Typography variant="caption" sx={{ flex: 1, fontWeight: 500, letterSpacing: '0.01em' }}>
              {editingId
                ? 'Editing queued message'
                : isOnline
                  ? `${renderedQueueMessages.length} queued`
                  : `${renderedQueueMessages.length} queued · offline`}
            </Typography>
          </Box>

          {/* Queue items with drag and drop */}
          <Box sx={{ maxHeight: 200, overflowY: 'auto' }}>
            <DndContext
              sensors={sensors}
              collisionDetection={closestCenter}
              onDragEnd={handleDragEnd}
            >
              <SortableContext
                items={renderedQueueMessages.map(m => m.id)}
                strategy={verticalListSortingStrategy}
              >
                {renderedQueueMessages.map((entry, index) => (
                    <SortableQueueItem
                      key={entry.id}
                      entry={entry}
                      index={index}
                      totalCount={renderedQueueMessages.length}
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
                      handleRestartAgent={handleRestartAgent}
                      isRestarting={isRestartingAgent}
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
        accept={inlineImageAttachments ? 'image/*' : attachmentAccept}
        onChange={handleFileInputChange}
        style={{ display: 'none' }}
      />

      {/* Input container */}
      <Box
        onDragEnter={handleDragEnter}
        onDragLeave={handleDragLeave}
        onDragOver={handleDragOver}
        onDrop={handleDrop}
        sx={{
          display: 'flex',
          flexDirection: 'column',
          ...(fill && { flex: 1, minHeight: 0 }),
          bgcolor: (theme) => fill ? 'transparent' : getChatColors(theme).composerSurface,
          borderRadius: fill
            ? 0
            : hasVisibleQueue || hasAttachedHeader
              ? '0 0 22px 22px'
              : '22px',
          border: fill ? 'none' : '1px solid',
          borderColor: isDraggingOver
            ? 'primary.main'
            : !isOnline
            ? 'warning.main'
            : (theme) => getChatColors(theme).borderStrong,
          transition: 'border-color 0.15s, box-shadow 0.15s, background-color 0.15s',
          boxShadow: fill
            ? 'none'
            : isDraggingOver
            ? (theme) => `0 0 0 2px ${alpha(theme.palette.primary.main, 0.22)}`
            : !isOnline
            ? (theme) => `0 0 0 2px ${alpha(theme.palette.warning.main, 0.2)}`
            : (theme) => theme.palette.mode === 'light'
                ? '0 12px 28px -18px rgba(0,0,0,0.4)'
                : 'none',
          '&:focus-within': {
            borderColor: (theme) => theme.palette.mode === 'dark'
              ? 'rgba(255,255,255,0.18)'
              : 'rgba(0,0,0,0.2)',
          },
          px: { xs: 1.5, sm: 2 },
          pt: { xs: 1.5, sm: 2 },
          pb: { xs: 1.5, sm: 2 },
        }}
      >
        {reviewComments.length > 0 && (
          <Box data-review-comment-tray sx={{ display: 'flex', flexWrap: 'wrap', gap: 0.75, mb: 1 }}>
            {reviewComments.map((comment) => (
              <Chip
                key={comment.id}
                size="small"
                icon={<MessageCircle size={13} />}
                label={`${comment.filePath} ${comment.rangeLabel}`}
                title={comment.text}
                onDelete={onRemoveReviewComment ? () => onRemoveReviewComment(comment.id) : undefined}
                sx={{
                  maxWidth: '100%',
                  height: 24,
                  bgcolor: (theme) => getChatColors(theme).inlineCodeSurface,
                  border: '1px solid',
                  borderColor: (theme) => getChatColors(theme).inlineCodeBorder,
                  '& .MuiChip-label': { overflow: 'hidden', textOverflow: 'ellipsis' },
                }}
              />
            ))}
          </Box>
        )}
        <ChatAttachmentTray
          attachments={attachments}
          onRemove={removeAttachment}
          onRetry={retryAttachment}
        />

        {attachmentError && (
          <Box role="alert" sx={{ display: 'flex', alignItems: 'flex-start', gap: 0.75, mb: 1, color: 'error.main' }}>
            <CircleAlert size={15} style={{ flexShrink: 0, marginTop: 2 }} />
            <Typography variant="caption" sx={{ lineHeight: 1.4 }}>
              {attachmentError}
            </Typography>
          </Box>
        )}

        {enableSandboxCompletions ? (
          <SandboxPromptEditor
            ref={sandboxEditorRef}
            value={draft}
            placeholder={promptPlaceholder || ''}
            disabled={inputDisabled}
            maxHeight={maxHeight}
            isDraggingOver={isDraggingOver}
            isOnline={isOnline}
            onValueChange={(value, cursor) => {
              setDraft(value)
              setComposerCursor(cursor)
              setComposerFocused(true)
            }}
            onCursorChange={setComposerCursor}
            onKeyDown={handleKeyDown}
            onFocus={(event) => {
              setComposerFocused(true)
              setComposerCursor(getSandboxPromptEditorCursor(event.currentTarget))
            }}
            onBlur={() => setComposerFocused(false)}
            onPaste={handlePaste}
          />
        ) : (
          <Box
            component="textarea"
            ref={textareaRef}
            value={draft}
            onChange={(e) => {
              setDraft(e.target.value)
              setComposerCursor(e.target.selectionStart)
              setComposerFocused(true)
            }}
            onKeyDown={handleKeyDown}
            onKeyUp={(e) => setComposerCursor(e.currentTarget.selectionStart)}
            onClick={(e) => setComposerCursor(e.currentTarget.selectionStart)}
            onFocus={(e) => {
              setComposerFocused(true)
              setComposerCursor(e.currentTarget.selectionStart)
            }}
            onBlur={() => setComposerFocused(false)}
            onPaste={handlePaste}
            placeholder={promptPlaceholder}
            disabled={inputDisabled}
            sx={{
              width: '100%',
              resize: 'none',
              border: 'none',
              borderRadius: 0,
              outline: 'none',
              bgcolor: 'transparent',
              color: (theme) => getChatColors(theme).foreground,
              fontFamily: 'inherit',
              // 1rem on a phone, not 0.9375: under 16px iOS zooms in on focus.
              fontSize: { xs: '1rem', sm: '0.875rem' },
              fontWeight: 450,
              lineHeight: 1.55,
              letterSpacing: '-0.005em',
              p: 0,
              ...(fill
                ? {
                    flex: 1,
                    minHeight: 0,
                    maxHeight: 'none',
                    fontSize: '1.0625rem',
                    overscrollBehavior: 'contain',
                  }
                : { minHeight: 70, maxHeight }),
              overflowY: 'auto',
              '&::placeholder': {
                color: isDraggingOver
                  ? 'primary.main'
                  : !isOnline
                    ? 'warning.main'
                    : (theme) => getChatColors(theme).subtle,
                opacity: isDraggingOver ? 1 : 0.72,
              },
              '&:disabled': {
                opacity: 0.6,
                cursor: 'not-allowed',
              },
            }}
          />
        )}

        {/* Buttons row at bottom */}
        <Box
          sx={{
            display: 'flex',
            alignItems: 'center',
            gap: 0.5,
            mt: 1.25,
            minWidth: 0,
            flexWrap: 'nowrap',
            ...(fill && { flexShrink: 0 }),
          }}
        >
          {/* On a phone the model/sandbox controls are wider than the screen.
              Only they scroll — attach and send stay pinned, because a send
              button you have to scroll to find is worse than a cramped one. */}
          {fill ? (
            <Box
              sx={{
                display: 'flex',
                alignItems: 'center',
                gap: 0.5,
                minWidth: 0,
                overflowX: 'auto',
                scrollbarWidth: 'none',
                '&::-webkit-scrollbar': { display: 'none' },
                '& > *': { flexShrink: 0 },
              }}
            >
              {leadingActions}
            </Box>
          ) : leadingActions}

          {leadingActions && (
            <Box
              aria-hidden="true"
              sx={{
                width: '1px',
                height: 16,
                mx: 0.25,
                flexShrink: 0,
                bgcolor: 'divider',
                opacity: 0.65,
              }}
            />
          )}

          {/* Attach file button */}
          {attachmentsEnabled && (
            <Tooltip title={inlineImageAttachments ? 'Attach image' : 'Attach file'}>
              <IconButton
                size="small"
                onClick={handleBrowseClick}
                disabled={inputDisabled}
                sx={{
                  color: (theme) => getChatColors(theme).subtle,
                  flexShrink: 0,
                  width: 28,
                  height: 28,
                  '&:hover': {
                    color: 'primary.main',
                  },
                }}
              >
                <Paperclip size={17} />
              </IconButton>
            </Tooltip>
          )}

          {/* Camera button (mobile only) */}
          {attachmentsEnabled && isMobile && (
            <Tooltip title="Take photo">
              <IconButton
                size="small"
                onClick={() => {
                  // Create a temporary input with capture for camera
                  const input = document.createElement('input')
                  input.type = 'file'
                  input.accept = 'image/*'
                  input.capture = 'environment' // Use rear camera by default
                  input.onchange = (e) => {
                    const files = (e.target as HTMLInputElement).files
                    if (files && files.length > 0) {
                      addFilesAsAttachments([files[0]])
                    }
                  }
                  input.click()
                }}
                disabled={inputDisabled}
                sx={{
                  color: 'text.secondary',
                  flexShrink: 0,
                  width: 28,
                  height: 28,
                  '&:hover': {
                    color: 'primary.main',
                  },
                }}
              >
                <Camera size={17} />
              </IconButton>
            </Tooltip>
          )}

          {/* Offline indicator */}
          {!isOnline && (
            <Tooltip title={sendMode === 'direct'
              ? "You're offline"
              : "You're offline - messages will queue and send when connected"}
            >
              <CloudOff size={20} style={{ flexShrink: 0 }} />
            </Tooltip>
          )}

          {/* Interrupt mode toggle */}
          {sendMode === 'queued' && <Tooltip
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
                  Keyboard: Enter = queue | {interruptShortcut} = interrupt
                </Typography>
              </Box>
            }
          >
            <IconButton
              size="small"
              onClick={() => setInterruptMode(!interruptMode)}
              aria-label={interruptMode ? 'Switch to queue mode' : 'Switch to interrupt mode'}
              aria-pressed={interruptMode}
              sx={{
                flexShrink: 0,
                width: 28,
                height: 28,
                color: interruptMode
                  ? 'warning.main'
                  : (theme) => getChatColors(theme).subtle,
                bgcolor: interruptMode
                  ? (theme) => alpha(theme.palette.warning.main, 0.1)
                  : 'transparent',
                '&:hover': {
                  color: interruptMode ? 'warning.main' : 'text.primary',
                  bgcolor: (theme) => alpha(theme.palette.text.primary, 0.06),
                },
              }}
            >
              {interruptMode ? (
                <Zap size={17} />
              ) : (
                <ListStart size={17} />
              )}
            </IconButton>
          </Tooltip>}

          <Box sx={{ flex: 1 }} />

          {trailingActions}

          {showContextUsage && <SessionContextUsageIndicator sessionId={sessionId} />}

          {/* Cancel button - visible when agent is busy */}
          {isAgentBusy && onCancel && (
            <Tooltip title={isCancelling ? 'Stopping generation…' : 'Stop generation'}>
              <Box component="span" sx={{ display: 'flex', flexShrink: 0 }}>
                <IconButton
                  onClick={onCancel}
                  aria-label={isCancelling ? 'Stopping generation' : 'Stop generation'}
                  disabled={isCancelling}
                  sx={{
                    flexShrink: 0,
                    width: 32,
                    height: 32,
                    color: '#fff',
                    bgcolor: 'error.main',
                    boxShadow: '0 1px 3px rgba(0,0,0,0.28)',
                    '&:hover': {
                      bgcolor: 'error.dark',
                    },
                    '&.Mui-disabled': {
                      color: '#fff',
                      bgcolor: 'error.main',
                      opacity: 0.72,
                    },
                    '& svg': {
                      pointerEvents: 'none',
                    },
                  }}
                >
                  {isCancelling
                    ? <CircularProgress size={14} thickness={5} color="inherit" />
                    : <Square size={12} fill="currentColor" strokeWidth={0} />}
                </IconButton>
              </Box>
            </Tooltip>
          )}

          {/* Send button */}
          {!isAgentBusy && !isDirectSending && (() => {
            const hasContent = draft.trim().length > 0 || reviewComments.length > 0
            const uploadedAttachments = attachments.filter(a => a.uploadStatus === 'uploaded')
            const pendingUploads = attachments.filter(a => a.uploadStatus === 'uploading' || a.uploadStatus === 'pending')
            const failedUploads = attachments.filter(a => a.uploadStatus === 'failed')
            const canSend = (hasContent || uploadedAttachments.length > 0) && pendingUploads.length === 0 && failedUploads.length === 0 && !inputDisabled && (isOnline || sendMode === 'queued')

            return (
              <Tooltip
                title={
                  pendingUploads.length > 0
                    ? `Uploading ${pendingUploads.length} file${pendingUploads.length > 1 ? 's' : ''}...`
                    : failedUploads.length > 0
                      ? 'Retry or remove failed uploads before sending'
                      : sendMode === 'direct'
                        ? 'Send message'
                        : `Add to queue (Enter = queue, ${interruptShortcut} = interrupt)`
                }
              >
                <span>
                  <IconButton
                    onClick={handleSend}
                    disabled={!canSend}
                    aria-label="Send message"
                    color={canSend ? 'secondary' : 'primary'}
                    sx={{
                      flexShrink: 0,
                      width: 32,
                      height: 32,
                      borderRadius: '50%',
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

    </Box>
  )

  if (!contextMenuAppId) return input

  return (
    <ContextMenuModal
      appId={contextMenuAppId}
      textAreaRef={textareaRef}
      onInsertText={handleContextMenuInsert}
    >
      {input}
    </ContextMenuModal>
  )
}

export default RobustPromptInput
