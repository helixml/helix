import React, { FC, useState, useRef, useEffect, useCallback } from 'react'
import {
  Paper,
  Box,
  Typography,
  Chip,
  Divider,
  IconButton,
  Tabs,
  Tab,
  Menu,
  MenuItem,
  ListItemText,
  TextField,
  Button,
  Checkbox,
  FormControlLabel,
  Tooltip,
} from '@mui/material'
import CloseIcon from '@mui/icons-material/Close'
import EditIcon from '@mui/icons-material/Edit'
import DragIndicatorIcon from '@mui/icons-material/DragIndicator'
import GridViewOutlined from '@mui/icons-material/GridViewOutlined'
import PlayArrow from '@mui/icons-material/PlayArrow'
import Description from '@mui/icons-material/Description'
import Send from '@mui/icons-material/Send'
import { TypesSpecTask } from '../../services'
import ExternalAgentDesktopViewer from '../external-agent/ExternalAgentDesktopViewer'
import DesignDocViewer from './DesignDocViewer'
import DesignReviewViewer from '../spec-tasks/DesignReviewViewer'
import useSnackbar from '../../hooks/useSnackbar'
import useApi from '../../hooks/useApi'
import { useStreaming } from '../../contexts/streaming'
import { useGetSession } from '../../services/sessionService'
import { SESSION_TYPE_TEXT } from '../../types'
import { useResize } from '../../hooks/useResize'
import { getSmartInitialPosition, getSmartInitialSize } from '../../utils/windowPositioning'

type WindowPosition = 'center' | 'full' | 'half-left' | 'half-right' | 'corner-tl' | 'corner-tr' | 'corner-bl' | 'corner-br'

interface SpecTaskDetailDialogProps {
  task: TypesSpecTask | null
  open: boolean
  onClose: () => void
  onEdit?: (task: TypesSpecTask) => void
}

const SpecTaskDetailDialog: FC<SpecTaskDetailDialogProps> = ({
  task,
  open,
  onClose,
  onEdit,
}) => {
  const api = useApi()
  const snackbar = useSnackbar()
  const streaming = useStreaming()

  const [position, setPosition] = useState<WindowPosition>('center')
  const [isSnapped, setIsSnapped] = useState(false)
  const [currentTab, setCurrentTab] = useState(0)
  const [tileMenuAnchor, setTileMenuAnchor] = useState<null | HTMLElement>(null)
  const [snapPreview, setSnapPreview] = useState<string | null>(null)
  const [isDragging, setIsDragging] = useState(false)
  const [dragStart, setDragStart] = useState<{ x: number; y: number } | null>(null)
  const [dragOffset, setDragOffset] = useState({ x: 0, y: 0 })
  const [message, setMessage] = useState('')
  const [clientUniqueId, setClientUniqueId] = useState<string>('')
  const [refreshedTask, setRefreshedTask] = useState<TypesSpecTask | null>(task)
  const nodeRef = useRef(null)

  // Resize hook for window resizing
  // Calculate size based on 4:3 aspect ratio for session viewer
  // Total chrome: title bar (~60px) + tabs (~48px) + message box (~110px) = ~220px
  const chromeHeight = 220
  const preferredHeight = window.innerHeight * 0.75 // Smaller default - 75% instead of 85%
  const sessionHeight = preferredHeight - chromeHeight
  const sessionWidth = sessionHeight * (4 / 3) // 4:3 aspect ratio
  const preferredWidth = Math.min(sessionWidth + 60, window.innerWidth * 0.6) // Smaller - 60% instead of 70%

  const smartSize = getSmartInitialSize(preferredWidth, preferredHeight, 700, 500)
  const smartPos = getSmartInitialPosition(smartSize.width, smartSize.height)

  const [windowPos, setWindowPos] = useState(smartPos)

  const { size, setSize, isResizing, getResizeHandles } = useResize({
    initialSize: smartSize,
    minSize: { width: 600, height: 400 },
    maxSize: { width: window.innerWidth, height: window.innerHeight },
    onResize: (newSize, direction) => {
      // Adjust position when resizing from top or left edges
      if (direction.includes('w') || direction.includes('n')) {
        setWindowPos(prev => ({
          x: direction.includes('w') ? prev.x + (size.width - newSize.width) : prev.x,
          y: direction.includes('n') ? prev.y + (size.height - newSize.height) : prev.y
        }))
      }
    }
  })

  // Design review state
  const [docViewerOpen, setDocViewerOpen] = useState(false)
  const [designReviewViewerOpen, setDesignReviewViewerOpen] = useState(false)
  const [activeReviewId, setActiveReviewId] = useState<string | null>(null)
  const [implementationReviewMessageSent, setImplementationReviewMessageSent] = useState(false)

  // Just Do It mode state (initialized from task, synced via API)
  const [justDoItMode, setJustDoItMode] = useState(task?.just_do_it_mode || false)
  const [updatingJustDoIt, setUpdatingJustDoIt] = useState(false)

  // Poll for task updates to detect when spec_session_id is populated
  useEffect(() => {
    if (!task?.id) return

    const pollTask = async () => {
      try {
        const response = await api.getApiClient().v1SpecTasksDetail(task.id!)
        if (response.data) {
          console.log('[SpecTaskDetailDialog] Polled task:', {
            id: response.data.id,
            status: response.data.status,
            planning_session_id: response.data.planning_session_id,
            has_session: !!response.data.planning_session_id
          })
          setRefreshedTask(response.data)
        }
      } catch (err) {
        console.error('Failed to poll task:', err)
      }
    }

    // Initial fetch
    pollTask()

    // Poll every 2 seconds
    const interval = setInterval(pollTask, 2000)
    return () => clearInterval(interval)
  }, [task?.id])

  // Use refreshed task data for rendering
  const displayTask = refreshedTask || task

  // Get the active session ID (single session used for entire workflow)
  const activeSessionId = displayTask?.planning_session_id

  // Fetch session data to get sway_version, gpu_vendor, and render_node for debug panel
  const { data: sessionResponse } = useGetSession(activeSessionId || '', { enabled: !!activeSessionId })
  const sessionData = sessionResponse?.data
  const swayVersion = sessionData?.config?.sway_version
  const gpuVendor = sessionData?.config?.gpu_vendor
  const renderNode = sessionData?.config?.render_node

  // Debug logging
  useEffect(() => {
    console.log('[SpecTaskDetailDialog] Active session state:', {
      taskId: displayTask?.id,
      status: displayTask?.status,
      planning_session_id: displayTask?.planning_session_id,
      activeSessionId,
      hasActiveSession: !!activeSessionId
    })
  }, [displayTask?.id, displayTask?.status, displayTask?.planning_session_id, activeSessionId])

  // Auto-send review request message when dialog opens for implementation_review
  useEffect(() => {
    if (
      open &&
      !implementationReviewMessageSent &&
      displayTask?.status === 'implementation_review' &&
      activeSessionId
    ) {
      const reviewMessage = `I'm here to review your implementation.

If this is a web application, please start the development server and provide the URL where I can test it.

I'll give you feedback and we can iterate on any changes needed.`

      streaming.NewInference({
        type: SESSION_TYPE_TEXT,
        message: reviewMessage,
        sessionId: activeSessionId,
      }).then(() => {
        setImplementationReviewMessageSent(true)
      }).catch((err) => {
        console.error('Failed to send implementation review message:', err)
      })
    }

    // Reset when dialog closes
    if (!open) {
      setImplementationReviewMessageSent(false)
    }
  }, [open, implementationReviewMessageSent, displayTask?.status, activeSessionId, streaming])

  const getPriorityColor = (priority: string) => {
    switch (priority?.toLowerCase()) {
      case 'critical':
        return 'error'
      case 'high':
        return 'warning'
      case 'medium':
        return 'info'
      case 'low':
        return 'success'
      default:
        return 'default'
    }
  }

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'spec_approved':
      case 'implementation_complete':
      case 'completed':
        return 'success'
      case 'in_progress':
      case 'spec_generation':
      case 'implementation_in_progress':
        return 'primary'
      case 'spec_review':
        return 'warning'
      case 'backlog':
        return 'default'
      default:
        return 'default'
    }
  }

  const formatStatus = (status: string) => {
    return status
      ?.split('_')
      .map((word) => word.charAt(0).toUpperCase() + word.slice(1))
      .join(' ')
  }

  const handleTile = (tilePosition: string) => {
    setTileMenuAnchor(null)
    setPosition(tilePosition as WindowPosition)
    setIsSnapped(true)
  }

  const getPositionStyle = () => {
    const w = window.innerWidth
    const h = window.innerHeight

    switch (position) {
      case 'full':
        return { top: 0, left: 0, width: w, height: h }
      case 'half-left':
        return { top: 0, left: 0, width: w / 2, height: h }
      case 'half-right':
        return { top: 0, left: w / 2, width: w / 2, height: h }
      case 'corner-tl':
        return { top: 0, left: 0, width: w / 2, height: h / 2 }
      case 'corner-tr':
        return { top: 0, left: w / 2, width: w / 2, height: h / 2 }
      case 'corner-bl':
        return { top: h / 2, left: 0, width: w / 2, height: h / 2 }
      case 'corner-br':
        return { top: h / 2, left: w / 2, width: w / 2, height: h / 2 }
      case 'center':
      default:
        return { top: windowPos.y, left: windowPos.x, width: size.width, height: size.height }
    }
  }

  const getSnapPreviewStyle = () => {
    if (!snapPreview) return null

    const w = window.innerWidth
    const h = window.innerHeight

    switch (snapPreview) {
      case 'full':
        return { top: 0, left: 0, width: w, height: h }
      case 'half-left':
        return { top: 0, left: 0, width: w / 2, height: h }
      case 'half-right':
        return { top: 0, left: w / 2, width: w / 2, height: h }
      case 'corner-tl':
        return { top: 0, left: 0, width: w / 2, height: h / 2 }
      case 'corner-tr':
        return { top: 0, left: w / 2, width: w / 2, height: h / 2 }
      case 'corner-bl':
        return { top: h / 2, left: 0, width: w / 2, height: h / 2 }
      case 'corner-br':
        return { top: h / 2, left: w / 2, width: w / 2, height: h / 2 }
      default:
        return null
    }
  }

  const handleStartPlanning = async () => {
    if (!task.id) return

    try {
      await api.getApiClient().v1SpecTasksStartPlanningCreate(task.id)
      snackbar.success('Planning started! Agent session will begin shortly.')
      // Switch to Active Session tab once it starts
      setCurrentTab(0)
    } catch (err: any) {
      console.error('Failed to start planning:', err)
      const errorMessage = err?.response?.data?.error
        || err?.response?.data?.message
        || err?.message
        || 'Failed to start planning. Please try again.'
      snackbar.error(errorMessage)
    }
  }

  const handleSendMessage = async () => {
    if (!message.trim() || !activeSessionId) return

    try {
      await streaming.NewInference({
        type: SESSION_TYPE_TEXT,
        message: message.trim(),
        sessionId: activeSessionId,
      })
      setMessage('')
    } catch (err) {
      console.error('Failed to send message:', err)
      snackbar.error('Failed to send message')
    }
  }

  // Toggle Just Do It mode and persist to backend
  const handleToggleJustDoIt = useCallback(async () => {
    if (!task?.id || updatingJustDoIt) return

    const newValue = !justDoItMode
    setUpdatingJustDoIt(true)

    try {
      await api.getApiClient().v1SpecTasksUpdate(task.id, {
        just_do_it_mode: newValue,
      })
      setJustDoItMode(newValue)
      snackbar.success(newValue ? 'Just Do It mode enabled' : 'Just Do It mode disabled')
    } catch (err) {
      console.error('Failed to update Just Do It mode:', err)
      snackbar.error('Failed to update Just Do It mode')
    } finally {
      setUpdatingJustDoIt(false)
    }
  }, [task?.id, justDoItMode, updatingJustDoIt, api, snackbar])

  // Keyboard shortcuts for task actions (with Ctrl/Cmd modifiers to work while typing)
  useEffect(() => {
    if (!open || !displayTask) return

    const handleKeyDown = (e: KeyboardEvent) => {
      const isMod = e.ctrlKey || e.metaKey

      // Ctrl/Cmd + J - Toggle Just Do It mode (only for backlog tasks)
      if (isMod && e.key === 'j' && displayTask.status === 'backlog') {
        e.preventDefault()
        handleToggleJustDoIt()
      }

      // Ctrl/Cmd + Enter - Start Planning (only for backlog tasks)
      if (isMod && e.key === 'Enter' && displayTask.status === 'backlog') {
        e.preventDefault()
        handleStartPlanning()
      }

      // Escape - Close dialog (no modifier needed)
      if (e.key === 'Escape' && !isMod) {
        e.preventDefault()
        onClose()
      }
    }

    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [open, displayTask, handleToggleJustDoIt, handleStartPlanning, onClose])

  // Sync justDoItMode when task changes
  useEffect(() => {
    if (displayTask?.just_do_it_mode !== undefined) {
      setJustDoItMode(displayTask.just_do_it_mode)
    }
  }, [displayTask?.just_do_it_mode])

  const handleMouseDown = (e: React.MouseEvent) => {
    if (isResizing) return // Don't allow dragging when resizing
    // Record mouse down position but don't start dragging yet (wait for movement threshold)
    setDragStart({ x: e.clientX, y: e.clientY })
    setDragOffset({
      x: e.clientX - windowPos.x,
      y: e.clientY - windowPos.y
    })
  }

  // Prevent text selection globally while dragging
  useEffect(() => {
    if (isDragging) {
      document.body.style.userSelect = 'none'
      return () => {
        document.body.style.userSelect = ''
      }
    }
  }, [isDragging])

  useEffect(() => {
    const handleMouseMove = (e: MouseEvent) => {
      // Check if we should start dragging (higher threshold when snapped to prevent accidental unsnapping)
      if (dragStart && !isDragging && !isResizing) {
        const dx = Math.abs(e.clientX - dragStart.x)
        const dy = Math.abs(e.clientY - dragStart.y)
        const dragThreshold = isSnapped ? 15 : 5

        if (dx > dragThreshold || dy > dragThreshold) {
          setIsDragging(true)
          setIsSnapped(false) // Unsnap when starting to drag
          if (position !== 'center') {
            setPosition('center')
          }
        }
        return // Don't move window until threshold is crossed
      }

      if (isDragging && position === 'center' && !isResizing) {
        const newX = e.clientX - dragOffset.x
        const newY = e.clientY - dragOffset.y

        // Keep within bounds
        const boundedX = Math.max(0, Math.min(newX, window.innerWidth - size.width))
        const boundedY = Math.max(0, Math.min(newY, window.innerHeight - size.height))

        setWindowPos({ x: boundedX, y: boundedY })

        // Detect snap zones
        const snapThreshold = 50
        const mouseX = e.clientX
        const mouseY = e.clientY
        const w = window.innerWidth
        const h = window.innerHeight

        let preview: string | null = null

        if (mouseX < snapThreshold) {
          if (mouseY < h / 3) {
            preview = 'corner-tl'
          } else if (mouseY > (2 * h) / 3) {
            preview = 'corner-bl'
          } else {
            preview = 'half-left'
          }
        } else if (mouseX > w - snapThreshold) {
          if (mouseY < h / 3) {
            preview = 'corner-tr'
          } else if (mouseY > (2 * h) / 3) {
            preview = 'corner-br'
          } else {
            preview = 'half-right'
          }
        } else if (mouseY < snapThreshold && mouseX > w / 3 && mouseX < (2 * w) / 3) {
          preview = 'full'
        }

        setSnapPreview(preview)
      }
    }

    const handleMouseUp = () => {
      if (snapPreview) {
        handleTile(snapPreview)
        setSnapPreview(null)
      }
      setIsDragging(false)
      setDragStart(null)
    }

    if (isDragging || dragStart) {
      document.addEventListener('mousemove', handleMouseMove)
      document.addEventListener('mouseup', handleMouseUp)
    }

    return () => {
      document.removeEventListener('mousemove', handleMouseMove)
      document.removeEventListener('mouseup', handleMouseUp)
    }
  }, [isDragging, dragStart, dragOffset, isResizing, position, isSnapped, size, snapPreview])

  const posStyle = getPositionStyle()

  if (!displayTask) return null

  return (
    <>
      {/* Snap Preview Overlay */}
      {snapPreview && (
        <Box
          sx={{
            position: 'fixed',
            ...getSnapPreviewStyle(),
            zIndex: 100000,
            backgroundColor: 'rgba(33, 150, 243, 0.3)',
            border: '2px solid rgba(33, 150, 243, 0.8)',
            pointerEvents: 'none',
            transition: 'all 0.1s ease',
          }}
        />
      )}

      {/* Floating Window */}
      <Paper
        ref={nodeRef}
        sx={{
          position: 'fixed',
          ...posStyle,
          zIndex: 9999,
          display: open ? 'flex' : 'none',
          flexDirection: 'column',
          backgroundColor: 'background.paper',
          boxShadow: 24,
          border: '1px solid',
          borderColor: 'divider',
          transition: position !== 'center' ? 'all 0.15s ease' : 'none',
        }}
      >
        {/* Resize Handles */}
        {position === 'center' && getResizeHandles().map((handle) => (
          <Box
            key={handle.direction}
            onMouseDown={(e) => {
              setIsSnapped(false) // Unsnap when resizing
              handle.onMouseDown(e)
            }}
            sx={{
              ...handle.style,
              // Make corner handles larger and more visible
              ...(handle.direction.length === 2 && {
                width: 16,
                height: 16,
              }),
              '&:hover': {
                backgroundColor: 'rgba(33, 150, 243, 0.3)',
              },
            }}
          />
        ))}
        {/* Title Bar */}
        <Box
          onMouseDown={handleMouseDown}
          sx={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            p: 0.75,
            cursor: position === 'center' && !isSnapped ? 'move' : 'default',
            borderBottom: '1px solid',
            borderColor: 'divider',
            backgroundColor: 'background.default',
            minHeight: 32,
            userSelect: 'none', // Prevent text selection when dragging
          }}
        >
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
            <DragIndicatorIcon sx={{ color: 'text.secondary', fontSize: 16 }} />
            <Box>
              <Typography variant="subtitle2" sx={{ fontSize: '0.875rem', fontWeight: 500 }}>
                {displayTask.name}
              </Typography>
              <Box sx={{ display: 'flex', gap: 0.5, mt: 0.25 }}>
                <Chip
                  label={formatStatus(displayTask.status)}
                  color={getStatusColor(displayTask.status)}
                  size="small"
                  sx={{ height: 20, fontSize: '0.7rem' }}
                />
                <Chip
                  label={displayTask.priority || 'Medium'}
                  color={getPriorityColor(displayTask.priority)}
                  size="small"
                  sx={{ height: 20, fontSize: '0.7rem' }}
                />
                {displayTask.type && (
                  <Chip label={displayTask.type} size="small" variant="outlined" sx={{ height: 20, fontSize: '0.7rem' }} />
                )}
              </Box>
            </Box>
          </Box>
          <Box sx={{ display: 'flex', gap: 0.25 }}>
            <IconButton
              size="small"
              onClick={(e) => {
                e.stopPropagation()
                setTileMenuAnchor(e.currentTarget)
              }}
              title="Tile Window"
              sx={{ padding: '4px' }}
            >
              <GridViewOutlined sx={{ fontSize: 16 }} />
            </IconButton>
            {onEdit && (
              <IconButton size="small" onClick={() => onEdit(displayTask)} sx={{ padding: '4px' }}>
                <EditIcon sx={{ fontSize: 16 }} />
              </IconButton>
            )}
            <IconButton size="small" onClick={onClose} sx={{ padding: '4px' }}>
              <CloseIcon sx={{ fontSize: 16 }} />
            </IconButton>
          </Box>
        </Box>

        {/* Tabs */}
        <Box sx={{ borderBottom: 1, borderColor: 'divider' }}>
          <Tabs value={currentTab} onChange={(_, newValue) => setCurrentTab(newValue)}>
            {activeSessionId && <Tab label="Active Session" />}
            <Tab label="Details" />
          </Tabs>
        </Box>

        {/* Tab Content */}
        <Box sx={{ flex: 1, overflow: 'hidden', display: 'flex', flexDirection: 'column' }}>
          {/* Tab 0: Active Session (only if session exists) */}
          {activeSessionId && currentTab === 0 && (
            <>
              {/* ExternalAgentDesktopViewer */}
              <Box sx={{ flex: 1, overflow: 'hidden', minHeight: 0 }}>
                <ExternalAgentDesktopViewer
                  sessionId={activeSessionId}
                  height="100%"
                  mode="stream"
                  onClientIdCalculated={setClientUniqueId}
                />
              </Box>

              {/* Message input box */}
              <Box sx={{ p: 2, borderTop: 1, borderColor: 'divider', flexShrink: 0 }}>
                <Box sx={{ display: 'flex', gap: 1 }}>
                  <TextField
                    fullWidth
                    size="small"
                    placeholder="Send message to agent..."
                    value={message}
                    onChange={(e) => setMessage(e.target.value)}
                    onKeyPress={(e) => {
                      if (e.key === 'Enter' && !e.shiftKey) {
                        e.preventDefault()
                        handleSendMessage()
                      }
                    }}
                  />
                  <Button
                    variant="contained"
                    size="small"
                    endIcon={<Send />}
                    onClick={handleSendMessage}
                    disabled={!message.trim()}
                  >
                    Send
                  </Button>
                </Box>
              </Box>
            </>
          )}

          {/* Details Tab */}
          {((activeSessionId && currentTab === 1) || (!activeSessionId && currentTab === 0)) && (
            <Box sx={{ flex: 1, overflow: 'auto', p: 3 }}>
              {/* Action Buttons */}
              <Box sx={{ mb: 3, display: 'flex', gap: 1, flexWrap: 'wrap', alignItems: 'center' }}>
                {displayTask.status === 'backlog' && (
                  <>
                    <Button
                      variant="contained"
                      color={justDoItMode ? 'success' : 'warning'}
                      startIcon={<PlayArrow />}
                      onClick={handleStartPlanning}
                      endIcon={
                        <Box component="span" sx={{ fontSize: '0.65rem', opacity: 0.7, fontFamily: 'monospace', ml: 0.5 }}>
                          {navigator.platform.includes('Mac') ? '⌘↵' : 'Ctrl+↵'}
                        </Box>
                      }
                    >
                      {justDoItMode ? 'Just Do It' : 'Start Planning'}
                    </Button>
                    <Tooltip title={`Skip spec planning and start implementation immediately (${navigator.platform.includes('Mac') ? '⌘J' : 'Ctrl+J'})`}>
                      <FormControlLabel
                        control={
                          <Checkbox
                            checked={justDoItMode}
                            onChange={handleToggleJustDoIt}
                            disabled={updatingJustDoIt}
                            color="warning"
                            size="small"
                          />
                        }
                        label={
                          <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
                            <Typography variant="body2">Just Do It</Typography>
                            <Box component="span" sx={{ fontSize: '0.6rem', opacity: 0.6, fontFamily: 'monospace', border: '1px solid', borderColor: 'divider', borderRadius: '3px', px: 0.5 }}>
                              {navigator.platform.includes('Mac') ? '⌘J' : 'Ctrl+J'}
                            </Box>
                          </Box>
                        }
                        sx={{ ml: 1 }}
                      />
                    </Tooltip>
                  </>
                )}
                {displayTask.status === 'spec_review' && (
                  <Button
                    variant="contained"
                    color="info"
                    startIcon={<Description />}
                    onClick={async () => {
                      if (!task) return

                      // Fetch design reviews for this task
                      try {
                        const response = await api.getApiClient().v1SpecTasksDesignReviewsDetail(task.id!)
                        console.log('Design reviews response:', response)
                        const reviews = response.data?.reviews || []
                        console.log('Reviews array:', reviews)

                        if (reviews.length > 0) {
                          // Get the latest non-superseded review
                          const latestReview = reviews.find((r: any) => r.status !== 'superseded') || reviews[0]
                          console.log('Opening review:', latestReview.id)
                          setActiveReviewId(latestReview.id)
                          setDesignReviewViewerOpen(true)
                        } else {
                          console.error('No design reviews found for task:', task.id)
                          setSnackbar({ message: 'No design review found', severity: 'error' })
                        }
                      } catch (error) {
                        console.error('Failed to fetch design reviews:', error)
                        setSnackbar({ message: 'Failed to load design review', severity: 'error' })
                      }
                    }}
                  >
                    Review Spec
                  </Button>
                )}
              </Box>

              <Divider sx={{ mb: 3 }} />

              {/* Full Description/Prompt */}
              <Box sx={{ mb: 3 }}>
                <Typography variant="subtitle2" color="text.secondary" gutterBottom>
                  Description
                </Typography>
                <Typography variant="body1" sx={{ whiteSpace: 'pre-wrap' }}>
                  {displayTask.description || displayTask.original_prompt || 'No description provided'}
                </Typography>
              </Box>

              {/* Context (from metadata) */}
              {displayTask.metadata?.context && (
                <Box sx={{ mb: 3 }}>
                  <Typography variant="subtitle2" color="text.secondary" gutterBottom>
                    Context
                  </Typography>
                  <Typography variant="body2" sx={{ whiteSpace: 'pre-wrap' }}>
                    {displayTask.metadata.context}
                  </Typography>
                </Box>
              )}

              {/* Constraints (from metadata) */}
              {displayTask.metadata?.constraints && (
                <Box sx={{ mb: 3 }}>
                  <Typography variant="subtitle2" color="text.secondary" gutterBottom>
                    Constraints
                  </Typography>
                  <Typography variant="body2" sx={{ whiteSpace: 'pre-wrap' }}>
                    {displayTask.metadata.constraints}
                  </Typography>
                </Box>
              )}

              <Divider sx={{ my: 2 }} />

              {/* Labels */}
              {displayTask.labels && displayTask.labels.length > 0 && (
                <Box sx={{ mb: 2 }}>
                  <Typography variant="subtitle2" color="text.secondary" gutterBottom>
                    Labels
                  </Typography>
                  <Box sx={{ display: 'flex', gap: 0.5, flexWrap: 'wrap' }}>
                    {displayTask.labels.map((label, idx) => (
                      <Chip key={idx} label={label} size="small" variant="outlined" />
                    ))}
                  </Box>
                </Box>
              )}

              {/* Estimated Hours */}
              {displayTask.estimated_hours && (
                <Box sx={{ mb: 2 }}>
                  <Typography variant="subtitle2" color="text.secondary">
                    Estimated Hours: <strong>{displayTask.estimated_hours}</strong>
                  </Typography>
                </Box>
              )}

              {/* Assigned Agent */}
              {displayTask.helix_app_id && (
                <Box sx={{ mb: 2 }}>
                  <Typography variant="subtitle2" color="text.secondary">
                    Assigned Agent: <strong>{displayTask.helix_app_id}</strong>
                  </Typography>
                </Box>
              )}

              {/* Timestamps */}
              <Box sx={{ mt: 3 }}>
                <Typography variant="caption" color="text.secondary" display="block">
                  Created: {displayTask.created_at ? new Date(displayTask.created_at).toLocaleString() : 'N/A'}
                </Typography>
                <Typography variant="caption" color="text.secondary" display="block">
                  Updated: {displayTask.updated_at ? new Date(displayTask.updated_at).toLocaleString() : 'N/A'}
                </Typography>
              </Box>

              {/* Debug Information */}
              <Divider sx={{ my: 2 }} />
              <Box sx={{ mt: 2, p: 2, bgcolor: 'grey.900', borderRadius: 1 }}>
                <Typography variant="caption" color="grey.400" display="block" gutterBottom sx={{ fontWeight: 600 }}>
                  Debug Information
                </Typography>
                <Typography variant="caption" color="grey.300" sx={{ fontFamily: 'monospace', fontSize: '0.7rem', display: 'block' }}>
                  Task ID: {displayTask.id || 'N/A'}
                </Typography>
                {activeSessionId && (
                  <>
                    <Typography variant="caption" color="grey.300" sx={{ fontFamily: 'monospace', fontSize: '0.7rem', display: 'block' }}>
                      Active Session ID: {activeSessionId}
                    </Typography>
                    {displayTask.planning_session_id && displayTask.spec_session_id && (
                      <Typography variant="caption" color="grey.400" sx={{ fontFamily: 'monospace', fontSize: '0.65rem', display: 'block', fontStyle: 'italic' }}>
                        (using planning_session_id, spec_session_id also available)
                      </Typography>
                    )}
                    <Typography variant="caption" color="grey.300" sx={{ fontFamily: 'monospace', fontSize: '0.7rem', display: 'block' }}>
                      Moonlight Client ID: {clientUniqueId || 'calculating...'}
                    </Typography>
                    {swayVersion && (
                      <Typography variant="caption" color="grey.300" sx={{ fontFamily: 'monospace', fontSize: '0.7rem', display: 'block' }}>
                        Sway Version:{' '}
                        <a
                          href={`https://github.com/helixml/helix/commit/${swayVersion}`}
                          target="_blank"
                          rel="noopener noreferrer"
                          style={{ color: '#90caf9', textDecoration: 'underline' }}
                        >
                          {swayVersion}
                        </a>
                      </Typography>
                    )}
                    {gpuVendor && (
                      <Typography variant="caption" color="grey.300" sx={{ fontFamily: 'monospace', fontSize: '0.7rem', display: 'block' }}>
                        GPU Vendor: {gpuVendor}
                      </Typography>
                    )}
                    {renderNode && (
                      <Typography variant="caption" color="grey.300" sx={{ fontFamily: 'monospace', fontSize: '0.7rem', display: 'block' }}>
                        Render Node: {renderNode}
                      </Typography>
                    )}
                  </>
                )}
              </Box>
            </Box>
          )}
        </Box>
      </Paper>

      {/* Tiling Menu */}
      <Menu
        anchorEl={tileMenuAnchor}
        open={Boolean(tileMenuAnchor)}
        onClose={() => setTileMenuAnchor(null)}
        sx={{ zIndex: 100001 }}
      >
        <MenuItem onClick={() => handleTile('full')}>
          <ListItemText primary="Full Screen" secondary="Fill entire window" />
        </MenuItem>
        <MenuItem onClick={() => handleTile('half-left')}>
          <ListItemText primary="Half Left" secondary="Left half of screen" />
        </MenuItem>
        <MenuItem onClick={() => handleTile('half-right')}>
          <ListItemText primary="Half Right" secondary="Right half of screen" />
        </MenuItem>
        <MenuItem onClick={() => handleTile('corner-tl')}>
          <ListItemText primary="Top Left" secondary="Upper left quarter" />
        </MenuItem>
        <MenuItem onClick={() => handleTile('corner-tr')}>
          <ListItemText primary="Top Right" secondary="Upper right quarter" />
        </MenuItem>
        <MenuItem onClick={() => handleTile('corner-bl')}>
          <ListItemText primary="Bottom Left" secondary="Lower left quarter" />
        </MenuItem>
        <MenuItem onClick={() => handleTile('corner-br')}>
          <ListItemText primary="Bottom Right" secondary="Lower right quarter" />
        </MenuItem>
      </Menu>

      {/* Design Document Viewer */}
      <DesignDocViewer
        open={docViewerOpen}
        onClose={() => {
          setDocViewerOpen(false)
        }}
        taskId={task?.id || ''}
        taskName={task?.name || ''}
        sessionId={task?.spec_session_id}
        onApprove={async () => {
          if (!task) return
          await api.getApiClient().v1SpecTasksApproveSpecsCreate(task.id!, {
            approved: true,
            comments: 'Specs approved',
          })
          snackbar.success('Specs approved')
          setDocViewerOpen(false)
        }}
        onReject={async (comment: string) => {
          if (!task) return
          await api.getApiClient().v1SpecTasksApproveSpecsCreate(task.id!, {
            approved: false,
            comments: comment,
          })
          snackbar.info('Requested changes to specs')
          setDocViewerOpen(false)
        }}
        onRejectCompletely={async (comment: string) => {
          if (!task) return
          await api.getApiClient().v1SpecTasksArchivePartialUpdate(task.id!, {
            archived: true,
          })
          snackbar.info('Task archived')
          setDocViewerOpen(false)
          onClose()
        }}
      />

      {/* Design Review Viewer - New beautiful review UI */}
      {designReviewViewerOpen && task && activeReviewId && (
        <DesignReviewViewer
          open={designReviewViewerOpen}
          onClose={() => {
            setDesignReviewViewerOpen(false)
            setActiveReviewId(null)
          }}
          specTaskId={task.id!}
          reviewId={activeReviewId}
        />
      )}
    </>
  )
}

export default SpecTaskDetailDialog
