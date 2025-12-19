import React, { FC, useState, useRef, useEffect, useCallback, useMemo } from 'react'
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
  Select,
  FormControl,
  InputLabel,
  CircularProgress,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogContentText,
  DialogActions,
} from '@mui/material'
import CloseIcon from '@mui/icons-material/Close'
import EditIcon from '@mui/icons-material/Edit'
import DragIndicatorIcon from '@mui/icons-material/DragIndicator'
import GridViewOutlined from '@mui/icons-material/GridViewOutlined'
import PlayArrow from '@mui/icons-material/PlayArrow'
import Description from '@mui/icons-material/Description'
import Send from '@mui/icons-material/Send'
import SaveIcon from '@mui/icons-material/Save'
import CancelIcon from '@mui/icons-material/Cancel'
import RestartAltIcon from '@mui/icons-material/RestartAlt'
import LaunchIcon from '@mui/icons-material/Launch'
import { TypesSpecTask, TypesSpecTaskPriority, TypesSpecTaskStatus } from '../../api/api'
import ExternalAgentDesktopViewer from '../external-agent/ExternalAgentDesktopViewer'
import DesignDocViewer from './DesignDocViewer'
import DesignReviewViewer from '../spec-tasks/DesignReviewViewer'
import useSnackbar from '../../hooks/useSnackbar'
import useApi from '../../hooks/useApi'
import { getBrowserLocale } from '../../hooks/useBrowserLocale'
import useApps from '../../hooks/useApps'
import { useStreaming } from '../../contexts/streaming'
import { useQueryClient } from '@tanstack/react-query'
import { useGetSession, GET_SESSION_QUERY_KEY } from '../../services/sessionService'
import { SESSION_TYPE_TEXT, AGENT_TYPE_ZED_EXTERNAL } from '../../types'
import { useResize } from '../../hooks/useResize'
import { getSmartInitialPosition, getSmartInitialSize } from '../../utils/windowPositioning'
import { useUpdateSpecTask, useSpecTask } from '../../services/specTaskService'
import RobustPromptInput from '../common/RobustPromptInput'

type WindowPosition = 'center' | 'full' | 'half-left' | 'half-right' | 'corner-tl' | 'corner-tr' | 'corner-bl' | 'corner-br'

interface SpecTaskDetailDialogProps {
  task: TypesSpecTask | null
  open: boolean
  onClose: () => void  
}

const SpecTaskDetailDialog: FC<SpecTaskDetailDialogProps> = ({
  task,
  open,
  onClose,  
}) => {
  const api = useApi()
  const snackbar = useSnackbar()
  const streaming = useStreaming()
  const apps = useApps()
  const updateSpecTask = useUpdateSpecTask()
  const queryClient = useQueryClient()

  // Edit mode state
  const [isEditMode, setIsEditMode] = useState(false)
  const [editFormData, setEditFormData] = useState({
    name: '',
    description: '',
    priority: '',
  })

  // Agent selection state
  const [selectedAgent, setSelectedAgent] = useState(task?.helix_app_id || '')
  const [updatingAgent, setUpdatingAgent] = useState(false)

  // Sort apps: zed_external agents first, then others
  const sortedApps = useMemo(() => {
    if (!apps.apps) return []
    const zedExternalApps = apps.apps.filter(app =>
      app.config?.helix?.assistants?.some(a => a.agent_type === AGENT_TYPE_ZED_EXTERNAL) ||
      app.config?.helix?.default_agent_type === AGENT_TYPE_ZED_EXTERNAL
    )
    const otherApps = apps.apps.filter(app =>
      !app.config?.helix?.assistants?.some(a => a.agent_type === AGENT_TYPE_ZED_EXTERNAL) &&
      app.config?.helix?.default_agent_type !== AGENT_TYPE_ZED_EXTERNAL
    )
    return [...zedExternalApps, ...otherApps]
  }, [apps.apps])

  // Get display settings from the task's app configuration
  const displaySettings = useMemo(() => {
    if (!task?.helix_app_id || !apps.apps) {
      return { width: 1920, height: 1080, fps: 60 } // Default values
    }
    const taskApp = apps.apps.find(a => a.id === task.helix_app_id)
    const config = taskApp?.config?.helix?.external_agent_config
    if (!config) {
      return { width: 1920, height: 1080, fps: 60 }
    }

    // Get dimensions from resolution preset or explicit values
    let width = config.display_width || 1920
    let height = config.display_height || 1080
    if (config.resolution === '5k') {
      width = 5120
      height = 2880
    } else if (config.resolution === '4k') {
      width = 3840
      height = 2160
    } else if (config.resolution === '1080p') {
      width = 1920
      height = 1080
    }

    return {
      width,
      height,
      fps: config.display_refresh_rate || 60,
    }
  }, [task?.helix_app_id, apps.apps])

  // Sync selected agent when task changes
  useEffect(() => {
    if (task?.helix_app_id) {
      setSelectedAgent(task.helix_app_id)
    }
  }, [task?.helix_app_id])

  // Load apps on mount
  useEffect(() => {
    if (open) {
      apps.loadApps()
    }
  }, [open])

  const [position, setPosition] = useState<WindowPosition>('center')
  const [isSnapped, setIsSnapped] = useState(false)
  const [currentTab, setCurrentTab] = useState(0)
  const [tileMenuAnchor, setTileMenuAnchor] = useState<null | HTMLElement>(null)
  const [snapPreview, setSnapPreview] = useState<string | null>(null)
  const [isDragging, setIsDragging] = useState(false)
  const [dragStart, setDragStart] = useState<{ x: number; y: number } | null>(null)
  const [dragOffset, setDragOffset] = useState({ x: 0, y: 0 })
  const [clientUniqueId, setClientUniqueId] = useState<string>('')
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

  // Session restart state
  const [restartConfirmOpen, setRestartConfirmOpen] = useState(false)
  const [isRestarting, setIsRestarting] = useState(false)

  // Just Do It mode state (initialized from task, synced via API)
  const [justDoItMode, setJustDoItMode] = useState(task?.just_do_it_mode || false)
  const [updatingJustDoIt, setUpdatingJustDoIt] = useState(false)

  // Use useSpecTask hook with auto-refresh, but disable when in edit mode
  const { data: refreshedTask } = useSpecTask(task?.id || '', {
    enabled: !!task?.id && open,
    refetchInterval: isEditMode ? false : 2000,
  })

  // Use refreshed task data for rendering
  const displayTask = refreshedTask || task

  // Initialize edit form data when task changes or edit mode is enabled
  useEffect(() => {
    if (displayTask && isEditMode) {
      setEditFormData({
        name: displayTask.name || '',
        description: displayTask.description || displayTask.original_prompt || '',
        priority: displayTask.priority || 'medium',
      })
    }
  }, [displayTask, isEditMode])

  // Get the active session ID (single session used for entire workflow)
  const activeSessionId = displayTask?.planning_session_id

  // Fetch session data to get sway_version, gpu_vendor, and render_node for debug panel
  const { data: sessionResponse } = useGetSession(activeSessionId || '', { enabled: !!activeSessionId })
  const sessionData = sessionResponse?.data
  const swayVersion = sessionData?.config?.sway_version
  const gpuVendor = sessionData?.config?.gpu_vendor
  const renderNode = sessionData?.config?.render_node
  const wolfLobbyId = sessionData?.config?.wolf_lobby_id

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
      // Include keyboard layout from browser locale detection (or ?keyboard= override)
      const { keyboardLayout, timezone, isOverridden } = getBrowserLocale();
      const queryParams = new URLSearchParams();
      if (keyboardLayout) queryParams.set('keyboard', keyboardLayout);
      if (timezone) queryParams.set('timezone', timezone);
      const queryString = queryParams.toString();
      const url = `/api/v1/spec-tasks/${task.id}/start-planning${queryString ? `?${queryString}` : ''}`;

      // Log keyboard layout being sent to API
      console.log(`%c[Start Planning] Keyboard: ${keyboardLayout}${isOverridden ? ' (from URL override)' : ' (from browser)'}`, 'color: #2196F3; font-weight: bold;');
      console.log(`[Start Planning] API URL: ${url}`);

      const response = await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'include',
      });
      if (!response.ok) {
        const errorData = await response.json().catch(() => ({}));
        throw new Error(errorData.error || errorData.message || `Failed to start planning: ${response.statusText}`);
      }

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

  // Handle session restart (stop container, then resume)
  const handleRestartSession = useCallback(async () => {
    if (!activeSessionId || isRestarting) return

    setIsRestarting(true)
    setRestartConfirmOpen(false)

    try {
      // Step 1: Stop the external agent container
      snackbar.info('Stopping agent session...')
      await api.getApiClient().v1SessionsStopExternalAgentDelete(activeSessionId)

      // Small delay to ensure container is fully stopped
      await new Promise(resolve => setTimeout(resolve, 1000))

      // Step 2: Resume the session (starts a new container)
      snackbar.info('Starting new agent session...')
      await api.getApiClient().v1SessionsResumeCreate(activeSessionId)

      // Step 3: Invalidate session query to refetch wolf_lobby_id
      // This triggers MoonlightStreamViewer to detect the lobby change and reconnect
      queryClient.invalidateQueries({ queryKey: GET_SESSION_QUERY_KEY(activeSessionId) })

      snackbar.success('Session restarted successfully')
    } catch (err: any) {
      console.error('Failed to restart session:', err)
      const errorMessage = err?.response?.data?.error
        || err?.response?.data?.message
        || err?.message
        || 'Failed to restart session'
      snackbar.error(errorMessage)
    } finally {
      setIsRestarting(false)
    }
  }, [activeSessionId, isRestarting, api, snackbar])

  // Toggle Just Do It mode and persist to backend
  const handleToggleJustDoIt = useCallback(async () => {
    if (!task?.id || updatingJustDoIt) return

    const newValue = !justDoItMode
    setUpdatingJustDoIt(true)

    try {
      await updateSpecTask.mutateAsync({
        taskId: task.id,
        updates: {
          just_do_it_mode: newValue,
        },
      })
      setJustDoItMode(newValue)
      snackbar.success(newValue ? 'Just Do It mode enabled' : 'Just Do It mode disabled')
    } catch (err) {
      console.error('Failed to update Just Do It mode:', err)
      snackbar.error('Failed to update Just Do It mode')
    } finally {
      setUpdatingJustDoIt(false)
    }
  }, [task?.id, justDoItMode, updatingJustDoIt, updateSpecTask, snackbar])

  // Handle agent change and persist to backend
  const handleAgentChange = useCallback(async (newAgentId: string) => {
    if (!task?.id || updatingAgent || newAgentId === selectedAgent) return

    setUpdatingAgent(true)
    const previousAgent = selectedAgent
    setSelectedAgent(newAgentId) // Optimistic update

    try {
      await updateSpecTask.mutateAsync({
        taskId: task.id,
        updates: {
          helix_app_id: newAgentId,
        },
      })
      snackbar.success('Agent updated')
    } catch (err) {
      console.error('Failed to update agent:', err)
      snackbar.error('Failed to update agent')
      setSelectedAgent(previousAgent) // Revert on error
    } finally {
      setUpdatingAgent(false)
    }
  }, [task?.id, selectedAgent, updatingAgent, updateSpecTask, snackbar])

  // Handle edit mode toggle
  const handleEditToggle = useCallback(() => {
    setIsEditMode(true)
  }, [])

  // Handle cancel edit
  const handleCancelEdit = useCallback(() => {
    setIsEditMode(false)
    if (displayTask) {
      setEditFormData({
        name: displayTask.name || '',
        description: displayTask.description || displayTask.original_prompt || '',
        priority: displayTask.priority || 'medium',
      })
    }
  }, [displayTask])

  // Handle save edit
  const handleSaveEdit = useCallback(async () => {
    if (!task?.id) return

    try {
      await updateSpecTask.mutateAsync({
        taskId: task.id,
        updates: {
          name: editFormData.name,
          description: editFormData.description,
          priority: editFormData.priority as TypesSpecTaskPriority,
        },
      })
      setIsEditMode(false)
      snackbar.success('Task updated successfully')
    } catch (err) {
      console.error('Failed to update task:', err)
      snackbar.error('Failed to update task')
    }
  }, [task?.id, editFormData, updateSpecTask, snackbar])

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

  // Double-click on title bar toggles maximize
  const handleTitleBarDoubleClick = useCallback(() => {
    if (position === 'full') {
      // Restore to center/floating
      setPosition('center')
      setIsSnapped(false)
    } else {
      // Maximize to full screen
      setPosition('full')
      setIsSnapped(true)
    }
  }, [position])

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
          onDoubleClick={handleTitleBarDoubleClick}
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
              <Typography variant="subtitle1" noWrap sx={{ maxWidth: 400 }}>
                {displayTask.description || displayTask.name || 'Unnamed task'}
              </Typography>
              <Box sx={{ display: 'flex', gap: 0.5, mt: 0.25 }}>
                <Chip
                  label={formatStatus(displayTask.status)}
                  color={getStatusColor(displayTask.status)}
                  size="small"
                  sx={{ height: 20 }}
                />
                <Chip
                  label={displayTask.priority || 'Medium'}
                  color={getPriorityColor(displayTask.priority)}
                  size="small"
                  sx={{ height: 20 }}
                />
                {displayTask.type && (
                  <Chip label={displayTask.type} size="small" variant="outlined" sx={{ height: 20 }} />
                )}
              </Box>
            </Box>
          </Box>
          <Box sx={{ display: 'flex', gap: 0.25, alignItems: 'center' }}>
            {isEditMode ? (
              <>
                <Button
                  variant="outlined"
                  size="small"
                  startIcon={<CancelIcon />}
                  onClick={handleCancelEdit}
                  sx={{ minWidth: 'auto', px: 1, mr: 1 }}
                >
                  Cancel
                </Button>
                <Button
                  variant="outlined"
                  size="small"
                  color="secondary"
                  startIcon={<SaveIcon />}
                  onClick={handleSaveEdit}
                  disabled={updateSpecTask.isPending}
                  sx={{ minWidth: 'auto', px: 1, mr: 1 }}
                >
                  {updateSpecTask.isPending ? 'Saving...' : 'Save'}
                </Button>
              </>
            ) : (
              <>
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
                {displayTask.status === TypesSpecTaskStatus.TaskStatusBacklog && (
                  <IconButton
                    size="small"
                    onClick={handleEditToggle}
                    sx={{ padding: '4px' }}
                    title="Edit task"
                  >
                    <EditIcon sx={{ fontSize: 16 }} />
                  </IconButton>
                )}
                {activeSessionId && (
                  <Tooltip
                    title="Restart agent session (stops container, starts fresh)"
                    slotProps={{ popper: { sx: { zIndex: 100001 } } }}
                  >
                    <IconButton
                      size="small"
                      onClick={() => setRestartConfirmOpen(true)}
                      disabled={isRestarting}
                      sx={{ padding: '4px' }}
                      color={isRestarting ? 'default' : 'warning'}
                    >
                      {isRestarting ? (
                        <CircularProgress size={16} />
                      ) : (
                        <RestartAltIcon sx={{ fontSize: 16 }} />
                      )}
                    </IconButton>
                  </Tooltip>
                )}
              </>
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
              {/* ExternalAgentDesktopViewer - flex: 1 fills available space */}
              <ExternalAgentDesktopViewer
                sessionId={activeSessionId}
                wolfLobbyId={wolfLobbyId}
                mode="stream"
                onClientIdCalculated={setClientUniqueId}
                displayWidth={displaySettings.width}
                displayHeight={displaySettings.height}
                displayFps={displaySettings.fps}
              />

              {/* Message input box */}
              <Box sx={{ p: 2, borderTop: 1, borderColor: 'divider', flexShrink: 0 }}>
                <RobustPromptInput
                  sessionId={activeSessionId}
                  specTaskId={displayTask.id}
                  projectId={displayTask.project_id}
                  apiClient={api.getApiClient()}
                  onSend={async (message: string) => {
                    await streaming.NewInference({
                      type: SESSION_TYPE_TEXT,
                      message,
                      sessionId: activeSessionId,
                    })
                  }}
                  placeholder="Send message to agent..."
                />
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
                        <Box component="span" sx={{ opacity: 0.7, fontFamily: 'monospace', ml: 0.5 }}>
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
                            <Box component="span" sx={{ opacity: 0.6, fontFamily: 'monospace', border: '1px solid', borderColor: 'divider', borderRadius: '3px', px: 0.5 }}>
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
                          snackbar.error('No design review found')
                        }
                      } catch (error) {
                        console.error('Failed to fetch design reviews:', error)
                        snackbar.error('Failed to load design review')
                      }
                    }}
                  >
                    Review Spec
                  </Button>
                )}
                {displayTask.pull_request_url && (
                  <Button
                    variant="outlined"
                    color="primary"
                    startIcon={<LaunchIcon />}
                    onClick={() => window.open(displayTask.pull_request_url, '_blank')}
                  >
                    View Pull Request
                  </Button>
                )}
              </Box>

              <Divider sx={{ mb: 3 }} />

              {/* Description - always show (name is derived from description) */}
              <Box sx={{ mb: 3 }}>
                <Typography variant="subtitle2" color="text.secondary" gutterBottom>
                  Description
                </Typography>
                {isEditMode ? (
                  <TextField
                    fullWidth
                    multiline
                    rows={4}
                    value={editFormData.description}
                    onChange={(e) => setEditFormData(prev => ({ ...prev, description: e.target.value }))}
                    placeholder="Task description"
                  />
                ) : (
                  <Typography variant="body1" sx={{ whiteSpace: 'pre-wrap' }}>
                    {displayTask.description || displayTask.original_prompt || 'No description provided'}
                  </Typography>
                )}
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

              {/* Priority - Editable */}
              <Box sx={{ mb: 2 }}>
                {isEditMode ? (
                  <FormControl fullWidth size="small">
                    <InputLabel>Priority</InputLabel>
                    <Select
                      value={editFormData.priority}
                      onChange={(e) => setEditFormData(prev => ({ ...prev, priority: e.target.value }))}
                      label="Priority"
                      MenuProps={{ sx: { zIndex: 100001 } }}
                    >
                      <MenuItem value={TypesSpecTaskPriority.SpecTaskPriorityCritical}>Critical</MenuItem>
                      <MenuItem value={TypesSpecTaskPriority.SpecTaskPriorityHigh}>High</MenuItem>
                      <MenuItem value={TypesSpecTaskPriority.SpecTaskPriorityMedium}>Medium</MenuItem>
                      <MenuItem value={TypesSpecTaskPriority.SpecTaskPriorityLow}>Low</MenuItem>
                    </Select>
                  </FormControl>
                ) : (
                  <>
                    <Typography variant="subtitle2" color="text.secondary" gutterBottom>
                      Priority
                    </Typography>
                    <Chip
                      label={displayTask.priority || 'Medium'}
                      color={getPriorityColor(displayTask.priority)}
                      size="small"
                    />
                  </>
                )}
              </Box>

              {/* Labels 
              TODO: show once we can create/update them
              */}
              {/* {displayTask.labels && displayTask.labels.length > 0 && (
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
              )} */}

              {/* Estimated Hours */}
              {displayTask.estimated_hours && (
                <Box sx={{ mb: 2 }}>
                  <Typography variant="subtitle2" color="text.secondary">
                    Estimated Hours: <strong>{displayTask.estimated_hours}</strong>
                  </Typography>
                </Box>
              )}

              {/* Assigned Agent - editable dropdown */}
              <Box sx={{ mb: 2 }}>
                <FormControl fullWidth size="small">
                  <InputLabel>Agent</InputLabel>
                  <Select
                    value={selectedAgent}
                    onChange={(e) => handleAgentChange(e.target.value)}
                    label="Agent"
                    disabled={updatingAgent}
                    endAdornment={updatingAgent ? <CircularProgress size={16} sx={{ mr: 2 }} /> : null}
                    MenuProps={{ sx: { zIndex: 100001 } }}
                  >
                    {sortedApps.map((app) => (
                      <MenuItem key={app.id} value={app.id}>
                        {app.config?.helix?.name || 'Unnamed Agent'}
                      </MenuItem>
                    ))}
                  </Select>
                </FormControl>
              </Box>

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
                <Typography variant="caption" color="grey.400" display="block" gutterBottom>
                  Debug Information
                </Typography>
                <Typography variant="caption" color="grey.300" sx={{ fontFamily: 'monospace', display: 'block' }}>
                  Task ID: {displayTask.id || 'N/A'}
                </Typography>
                {displayTask.task_number && (
                  <Typography variant="caption" color="grey.300" sx={{ fontFamily: 'monospace', fontSize: '0.7rem', display: 'block' }}>
                    Task Number: {displayTask.task_number}
                  </Typography>
                )}
                {displayTask.design_doc_path && (
                  <Typography variant="caption" color="grey.300" sx={{ fontFamily: 'monospace', fontSize: '0.7rem', display: 'block' }}>
                    Design Doc Path: {displayTask.design_doc_path}
                  </Typography>
                )}
                {displayTask.branch_name && (
                  <Typography variant="caption" color="grey.300" sx={{ fontFamily: 'monospace', fontSize: '0.7rem', display: 'block' }}>
                    Branch: {displayTask.branch_name}
                  </Typography>
                )}
                {displayTask.pull_request_url && (
                  <Typography variant="caption" color="grey.300" sx={{ fontFamily: 'monospace', fontSize: '0.7rem', display: 'block' }}>
                    Pull Request:{' '}
                    <a
                      href={displayTask.pull_request_url}
                      target="_blank"
                      rel="noopener noreferrer"
                      style={{ color: '#4caf50', textDecoration: 'underline', fontWeight: 600 }}
                      onClick={(e) => e.stopPropagation()}
                    >
                      #{displayTask.pull_request_id}
                    </a>
                  </Typography>
                )}
                {activeSessionId && (
                  <>
                    <Typography variant="caption" color="grey.300" sx={{ fontFamily: 'monospace', display: 'block' }}>
                      Active Session ID: {activeSessionId}
                    </Typography>
                    {displayTask.planning_session_id && (
                      <Typography variant="caption" color="grey.400" sx={{ fontFamily: 'monospace', display: 'block', fontStyle: 'italic' }}>
                        (using planning_session_id)
                      </Typography>
                    )}
                    <Typography variant="caption" color="grey.300" sx={{ fontFamily: 'monospace', display: 'block' }}>
                      Moonlight Client ID: {clientUniqueId || 'calculating...'}
                    </Typography>
                    {swayVersion && (
                      <Typography variant="caption" color="grey.300" sx={{ fontFamily: 'monospace', display: 'block' }}>
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
                      <Typography variant="caption" color="grey.300" sx={{ fontFamily: 'monospace', display: 'block' }}>
                        GPU Vendor: {gpuVendor}
                      </Typography>
                    )}
                    {renderNode && (
                      <Typography variant="caption" color="grey.300" sx={{ fontFamily: 'monospace', display: 'block' }}>
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
        sessionId={task?.planning_session_id}
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

      {/* Restart Session Confirmation Dialog */}
      <Dialog
        open={restartConfirmOpen}
        onClose={() => setRestartConfirmOpen(false)}
        sx={{ zIndex: 100002 }}
      >
        <DialogTitle>Restart Agent Session?</DialogTitle>
        <DialogContent>
          <DialogContentText>
            This will stop the current agent container and start a fresh one.
            <br /><br />
            <strong>Note:</strong> Any unsaved files in the sandbox may be lost. Please make sure
            you save all your files before restarting. Everything in the work folder will survive the restart.
          </DialogContentText>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setRestartConfirmOpen(false)}>
            Cancel
          </Button>
          <Button
            onClick={handleRestartSession}
            color="warning"
            variant="contained"
            disabled={isRestarting}
            startIcon={isRestarting ? <CircularProgress size={16} /> : <RestartAltIcon />}
          >
            {isRestarting ? 'Restarting...' : 'Restart Session'}
          </Button>
        </DialogActions>
      </Dialog>
    </>
  )
}

export default SpecTaskDetailDialog
