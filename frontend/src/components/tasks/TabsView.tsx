import React, { useState, useEffect, useCallback, useMemo } from 'react'
import {
  Box,
  Typography,
  IconButton,
  Tooltip,
  TextField,
  keyframes,
  Menu,
  MenuItem,
  ListItemIcon,
  ListItemText,
  Divider,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  CircularProgress,
  Chip,
  Alert,
} from '@mui/material'
import {
  Close as CloseIcon,
  Add as AddIcon,
  Circle as CircleIcon,
  SplitscreenOutlined as SplitHorizontalIcon,
  ViewColumn as SplitVerticalIcon,
  MoreVert as MoreIcon,
  Create as CreateIcon,
  PlayArrow as PlayIcon,
  Description as SpecIcon,
  CheckCircle as ApproveIcon,
  Launch as LaunchIcon,
  Computer as DesktopIcon,
} from '@mui/icons-material'
import { Panel, Group as PanelGroup, Separator as PanelResizeHandle } from 'react-resizable-panels'

import { TypesSpecTask, TypesCreateTaskRequest, TypesSpecTaskPriority, TypesSession, TypesTitleHistoryEntry } from '../../api/api'
import useSnackbar from '../../hooks/useSnackbar'
import useApi from '../../hooks/useApi'
import { useUpdateSpecTask, useSpecTask } from '../../services/specTaskService'
import { useQuery } from '@tanstack/react-query'
import { getBrowserLocale } from '../../hooks/useBrowserLocale'
import SpecTaskDetailContent from './SpecTaskDetailContent'
import ArchiveConfirmDialog from './ArchiveConfirmDialog'
import DesignReviewContent from '../spec-tasks/DesignReviewContent'
import ExternalAgentDesktopViewer from '../external-agent/ExternalAgentDesktopViewer'
import RobustPromptInput from '../common/RobustPromptInput'
import { useStreaming } from '../../contexts/streaming'
import { SESSION_TYPE_TEXT } from '../../types'
import useAccount from '../../hooks/useAccount'

// Pulse animation for active agent indicator
const activePulse = keyframes`
  0%, 100% {
    transform: scale(1);
    opacity: 1;
  }
  50% {
    transform: scale(1.4);
    opacity: 0.7;
  }
`

// Helper to check if agent is active (session updated within last 10 seconds)
const isAgentActive = (sessionUpdatedAt?: string): boolean => {
  if (!sessionUpdatedAt) return false
  const updatedTime = new Date(sessionUpdatedAt).getTime()
  const now = Date.now()
  const diffSeconds = (now - updatedTime) / 1000
  return diffSeconds < 10
}

// Hook to periodically check agent activity status
const useAgentActivityCheck = (
  sessionUpdatedAt?: string,
  enabled: boolean = true
): { isActive: boolean; needsAttention: boolean; markAsSeen: () => void } => {
  const [tick, setTick] = useState(0)
  const [lastSeenTimestamp, setLastSeenTimestamp] = useState<string | null>(null)

  useEffect(() => {
    if (!enabled || !sessionUpdatedAt) return
    const interval = setInterval(() => setTick(t => t + 1), 3000)
    return () => clearInterval(interval)
  }, [enabled, sessionUpdatedAt])

  const isActive = isAgentActive(sessionUpdatedAt)
  const needsAttention = !isActive && sessionUpdatedAt !== lastSeenTimestamp && !!sessionUpdatedAt

  const markAsSeen = () => {
    if (sessionUpdatedAt) setLastSeenTimestamp(sessionUpdatedAt)
  }

  useEffect(() => {
    if (isActive && lastSeenTimestamp) setLastSeenTimestamp(null)
  }, [isActive, lastSeenTimestamp])

  return { isActive, needsAttention, markAsSeen }
}

// Generate unique panel IDs
let panelIdCounter = 0
const generatePanelId = () => `panel-${++panelIdCounter}`

interface TabData {
  id: string
  type: 'task' | 'review' | 'desktop'
  task?: TypesSpecTask
  // For review tabs
  taskId?: string
  reviewId?: string
  reviewTitle?: string
  // For desktop tabs
  sessionId?: string
  desktopTitle?: string
}

interface PanelData {
  id: string
  tabs: TabData[]
  activeTabId: string | null
}

interface PanelTabProps {
  tab: TabData
  isActive: boolean
  onSelect: () => void
  onClose: (e: React.MouseEvent) => void
  onRename: (newTitle: string) => void
  onDragStart: (e: React.DragEvent, tabId: string) => void
  onTouchDragStart: (tabId: string) => void
  onTouchDragEnd: (tabId: string, clientX: number, clientY: number) => void
}

const PanelTab: React.FC<PanelTabProps> = ({
  tab,
  isActive,
  onSelect,
  onClose,
  onRename,
  onDragStart,
  onTouchDragStart,
  onTouchDragEnd,
}) => {
  const api = useApi()
  const [isEditing, setIsEditing] = useState(false)
  const [editValue, setEditValue] = useState('')
  const [isHovered, setIsHovered] = useState(false)
  const [isTouchDragging, setIsTouchDragging] = useState(false)
  const touchStartRef = React.useRef<{ x: number; y: number; time: number } | null>(null)

  // Only fetch task data for task tabs
  const { data: refreshedTask } = useSpecTask(tab.type === 'task' ? tab.id : '', {
    enabled: tab.type === 'task',
    refetchInterval: 3000,
  })
  const displayTask = tab.type === 'task' ? (refreshedTask || tab.task) : null

  const hasSession = !!(displayTask?.planning_session_id)
  const { isActive: isAgentActiveState, needsAttention, markAsSeen } = useAgentActivityCheck(
    displayTask?.session_updated_at,
    hasSession
  )

  // Fetch session data with title history when hovering (only if session exists)
  const { data: sessionData } = useQuery({
    queryKey: ['session-title-history', displayTask?.planning_session_id],
    queryFn: async () => {
      if (!displayTask?.planning_session_id) return null
      const response = await api.get<TypesSession>(`/api/v1/sessions/${displayTask.planning_session_id}`)
      return response
    },
    enabled: isHovered && !!displayTask?.planning_session_id,
    staleTime: 30000, // Cache for 30 seconds
  })

  const titleHistory = sessionData?.config?.title_history || []

  // Display title depends on tab type
  const displayTitle = tab.type === 'review'
    ? (tab.reviewTitle || 'Spec Review')
    : tab.type === 'desktop'
    ? (tab.desktopTitle || 'Team Desktop')
    : (displayTask?.user_short_title
      || displayTask?.short_title
      || displayTask?.name?.substring(0, 20)
      || 'Task')

  // Format title history for tooltip
  const tooltipContent = useMemo(() => {
    // Review tabs have a simple tooltip
    if (tab.type === 'review') {
      return tab.reviewTitle || 'Spec Review'
    }
    // Desktop tabs
    if (tab.type === 'desktop') {
      return tab.desktopTitle || 'Team Desktop'
    }
    if (!hasSession) {
      return displayTask?.name || displayTask?.description || 'Task details'
    }
    if (titleHistory.length === 0) {
      return displayTask?.name || displayTask?.description || 'No title history yet'
    }
    return (
      <Box sx={{ p: 0.5 }}>
        <Typography variant="caption" color="text.secondary" sx={{ fontWeight: 600, mb: 0.5, display: 'block' }}>
          Topic Evolution
        </Typography>
        {titleHistory.slice(0, 5).map((entry: TypesTitleHistoryEntry, idx: number) => (
          <Box key={idx} sx={{ display: 'flex', gap: 1, alignItems: 'baseline', py: 0.25 }}>
            <Typography variant="caption" color="primary.main" sx={{ fontWeight: 500, minWidth: 35 }}>
              Turn {entry.turn}
            </Typography>
            <Typography variant="caption" color="text.primary" sx={{ flex: 1 }}>
              {entry.title}
            </Typography>
          </Box>
        ))}
        {titleHistory.length > 5 && (
          <Typography variant="caption" color="text.secondary" sx={{ fontStyle: 'italic', mt: 0.5, display: 'block' }}>
            +{titleHistory.length - 5} more...
          </Typography>
        )}
      </Box>
    )
  }, [tab.type, tab.reviewTitle, tab.desktopTitle, hasSession, titleHistory, displayTask?.name, displayTask?.description])

  const handleDoubleClick = (e: React.MouseEvent) => {
    // Only allow renaming task tabs
    if (tab.type !== 'task') return
    e.stopPropagation()
    setEditValue(displayTask?.user_short_title || displayTask?.short_title || displayTask?.name || '')
    setIsEditing(true)
  }

  const handleEditSubmit = () => {
    if (editValue.trim()) onRename(editValue.trim())
    setIsEditing(false)
  }

  const handleClick = () => {
    markAsSeen()
    onSelect()
  }

  // Touch drag handlers for iPad/mobile
  const handleTouchStart = (e: React.TouchEvent) => {
    const touch = e.touches[0]
    touchStartRef.current = { x: touch.clientX, y: touch.clientY, time: Date.now() }
  }

  const handleTouchMove = (e: React.TouchEvent) => {
    if (!touchStartRef.current) return
    const touch = e.touches[0]
    const dx = touch.clientX - touchStartRef.current.x
    const dy = touch.clientY - touchStartRef.current.y
    const distance = Math.sqrt(dx * dx + dy * dy)
    // Start drag after moving 10px
    if (distance > 10 && !isTouchDragging) {
      setIsTouchDragging(true)
      onTouchDragStart(tab.id)
    }
  }

  const handleTouchEnd = (e: React.TouchEvent) => {
    if (isTouchDragging) {
      // Use the last known touch position (changedTouches has the release position)
      const touch = e.changedTouches[0]
      onTouchDragEnd(tab.id, touch.clientX, touch.clientY)
      setIsTouchDragging(false)
    } else if (touchStartRef.current) {
      // Short tap - just select the tab
      const elapsed = Date.now() - touchStartRef.current.time
      if (elapsed < 200) {
        handleClick()
      }
    }
    touchStartRef.current = null
  }

  return (
    <Tooltip
      title={tooltipContent}
      placement="bottom"
      enterDelay={500}
      enterNextDelay={300}
      arrow
      slotProps={{
        tooltip: {
          sx: {
            maxWidth: 350,
            backgroundColor: 'background.paper',
            color: 'text.primary',
            border: '1px solid',
            borderColor: 'divider',
            boxShadow: 2,
          },
        },
        arrow: {
          sx: {
            color: 'background.paper',
            '&::before': {
              border: '1px solid',
              borderColor: 'divider',
            },
          },
        },
      }}
    >
      <Box
        draggable
        onDragStart={(e) => onDragStart(e, tab.id)}
        onMouseDown={(e) => {
          // Prevent text selection when starting a drag with trackpad/mouse
          // Don't prevent default on the close button
          if ((e.target as HTMLElement).closest('button')) return
          e.preventDefault()
        }}
        onSelectStart={(e) => e.preventDefault()}
        onClick={handleClick}
        onDoubleClick={handleDoubleClick}
        onMouseEnter={() => setIsHovered(true)}
        onMouseLeave={() => setIsHovered(false)}
        onTouchStart={handleTouchStart}
        onTouchMove={handleTouchMove}
        onTouchEnd={handleTouchEnd}
        sx={{
          display: 'flex',
          alignItems: 'center',
          gap: 0.5,
          px: 1.5,
          py: 0.5,
          minWidth: 100,
          maxWidth: 180,
          cursor: 'grab',
          backgroundColor: isTouchDragging ? 'primary.main' : (isActive ? 'background.paper' : 'transparent'),
          borderBottom: isActive ? '2px solid' : '2px solid transparent',
          borderBottomColor: isActive ? 'primary.main' : 'transparent',
          opacity: isTouchDragging ? 0.7 : 1,
          transition: 'all 0.15s ease',
          userSelect: 'none',
          WebkitUserSelect: 'none',
          WebkitTouchCallout: 'none',
          // Enable dragging on Safari/WebKit
          WebkitUserDrag: 'element',
          // Prevent iOS from triggering its own gestures
          touchAction: 'none',
          '&:hover': {
            backgroundColor: isActive ? 'background.paper' : 'action.hover',
          },
          '&:active': {
            cursor: 'grabbing',
          },
        }}
      >
      {/* Activity indicator */}
      {hasSession && (
        <Box sx={{ display: 'flex', alignItems: 'center', mr: 0.5 }}>
          {isAgentActiveState ? (
            <Tooltip title="Agent is working">
              <Box
                sx={{
                  width: 6,
                  height: 6,
                  borderRadius: '50%',
                  backgroundColor: '#22c55e',
                  animation: `${activePulse} 1.5s ease-in-out infinite`,
                }}
              />
            </Tooltip>
          ) : needsAttention ? (
            <Tooltip title="Agent finished">
              <Box sx={{ width: 6, height: 6, borderRadius: '50%', backgroundColor: '#f59e0b' }} />
            </Tooltip>
          ) : (
            <Box sx={{ width: 6, height: 6, borderRadius: '50%', backgroundColor: 'text.disabled', opacity: 0.3 }} />
          )}
        </Box>
      )}

      {isEditing ? (
        <TextField
          size="small"
          value={editValue}
          onChange={(e) => setEditValue(e.target.value)}
          onBlur={handleEditSubmit}
          onKeyDown={(e) => {
            if (e.key === 'Enter') { e.preventDefault(); handleEditSubmit() }
            else if (e.key === 'Escape') setIsEditing(false)
          }}
          autoFocus
          onClick={(e) => e.stopPropagation()}
          sx={{
            flex: 1,
            '& .MuiInputBase-input': { py: 0, px: 0.5, fontSize: '0.75rem' },
            '& .MuiOutlinedInput-notchedOutline': { border: 'none' },
          }}
        />
      ) : (
        <Typography
          variant="body2"
          noWrap
          sx={{
            flex: 1,
            fontSize: '0.75rem',
            fontWeight: isActive ? 600 : 400,
            color: isActive ? 'text.primary' : 'text.secondary',
          }}
        >
          {displayTitle}
        </Typography>
      )}

      <IconButton
        size="small"
        onClick={onClose}
        sx={{ p: 0.25, opacity: 0.5, '&:hover': { opacity: 1 } }}
      >
        <CloseIcon sx={{ fontSize: 12 }} />
      </IconButton>
      </Box>
    </Tooltip>
  )
}

// Single resizable panel with its own tabs
interface TaskPanelProps {
  panel: PanelData
  tasks: TypesSpecTask[]
  projectId?: string
  onTabSelect: (panelId: string, tabId: string) => void
  onTabClose: (panelId: string, tabId: string) => void
  onTabRename: (tabId: string, newTitle: string) => void
  onAddTab: (panelId: string, task: TypesSpecTask) => void
  onAddDesktop: (panelId: string, sessionId: string, title?: string) => void
  onTaskCreated: (panelId: string, task: TypesSpecTask) => void
  onSplitPanel: (panelId: string, direction: 'horizontal' | 'vertical', taskId?: string) => void
  onDropTab: (panelId: string, tabId: string, fromPanelId: string) => void
  onClosePanel: (panelId: string) => void
  onOpenReview: (taskId: string, reviewId: string, reviewTitle?: string) => void
  onTouchDragStart: (panelId: string, tabId: string) => void
  onTouchDragEnd: (panelId: string, tabId: string, clientX: number, clientY: number) => void
  panelCount: number
  panelRef: (el: HTMLDivElement | null) => void
}

const TaskPanel: React.FC<TaskPanelProps> = ({
  panel,
  tasks,
  projectId,
  onTabSelect,
  onTabClose,
  onTabRename,
  onAddTab,
  onAddDesktop,
  onTaskCreated,
  onSplitPanel,
  onDropTab,
  onClosePanel,
  onOpenReview,
  onTouchDragStart,
  onTouchDragEnd,
  panelCount,
  panelRef,
}) => {
  const api = useApi()
  const snackbar = useSnackbar()
  const account = useAccount()
  const streaming = useStreaming()
  const [menuAnchor, setMenuAnchor] = useState<null | HTMLElement>(null)
  const [createDialogOpen, setCreateDialogOpen] = useState(false)
  const [createPrompt, setCreatePrompt] = useState('')
  const [isCreating, setIsCreating] = useState(false)
  const [isActioning, setIsActioning] = useState(false)
  const [dragOverEdge, setDragOverEdge] = useState<'left' | 'right' | 'top' | 'bottom' | null>(null)
  const [draggedTabId, setDraggedTabId] = useState<string | null>(null)
  const [draggedFromPanelId, setDraggedFromPanelId] = useState<string | null>(null)

  // Archive/reject confirmation dialog state
  const [archiveConfirmOpen, setArchiveConfirmOpen] = useState(false)

  const activeTab = panel.tabs.find(t => t.id === panel.activeTabId)
  const unopenedTasks = tasks.filter(t => !panel.tabs.some(tab => tab.id === t.id))

  // Get refreshed task data for the active tab (from the tasks prop which is periodically refetched)
  const activeTask = activeTab ? tasks.find(t => t.id === activeTab.id) || activeTab.task : null

  // Helper to get status display info
  const getStatusInfo = (task: TypesSpecTask) => {
    const status = task.status || ''
    switch (status) {
      case 'backlog':
        return { label: 'Backlog', color: 'default' as const }
      case 'spec_generation':
        return { label: 'Planning', color: 'warning' as const }
      case 'spec_review':
        return { label: 'Review Spec', color: 'info' as const }
      case 'implementation':
        return { label: 'In Progress', color: 'success' as const }
      case 'implementation_review':
        return { label: 'Review Code', color: 'info' as const }
      case 'pull_request':
        return { label: 'Pull Request', color: 'secondary' as const }
      case 'done':
        return { label: 'Complete', color: 'success' as const }
      default:
        return { label: status, color: 'default' as const }
    }
  }

  // Handle creating a new task
  const handleCreateTask = async () => {
    if (!createPrompt.trim()) {
      snackbar.error('Please describe what you want to get done')
      return
    }

    if (!projectId) {
      snackbar.error('No project selected')
      return
    }

    setIsCreating(true)
    try {
      const createTaskRequest: TypesCreateTaskRequest = {
        prompt: createPrompt.trim(),
        priority: TypesSpecTaskPriority.SpecTaskPriorityMedium,
        project_id: projectId,
      }

      const response = await api.getApiClient().v1SpecTasksFromPromptCreate(createTaskRequest)

      if (response.data) {
        snackbar.success('Task created!')
        setCreateDialogOpen(false)
        setCreatePrompt('')
        // Add the new task to this panel
        onTaskCreated(panel.id, response.data)
      }
    } catch (err: any) {
      console.error('Failed to create task:', err)
      snackbar.error(err?.message || 'Failed to create task')
    } finally {
      setIsCreating(false)
    }
  }

  // Handle starting planning for a task
  const handleStartPlanning = async () => {
    if (!activeTask?.id) return

    setIsActioning(true)
    try {
      const { keyboardLayout, timezone } = getBrowserLocale()
      const queryParams = new URLSearchParams()
      if (keyboardLayout) queryParams.set('keyboard', keyboardLayout)
      if (timezone) queryParams.set('timezone', timezone)
      const queryString = queryParams.toString()
      const url = `/api/v1/spec-tasks/${activeTask.id}/start-planning${queryString ? `?${queryString}` : ''}`

      const response = await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'include',
      })
      if (!response.ok) {
        const errorData = await response.json().catch(() => ({}))
        throw new Error(errorData.error || errorData.message || `Failed to start planning: ${response.statusText}`)
      }

      snackbar.success('Planning started! Agent session will begin shortly.')
    } catch (err: any) {
      console.error('Failed to start planning:', err)
      snackbar.error(err?.message || 'Failed to start planning. Please try again.')
    } finally {
      setIsActioning(false)
    }
  }

  // Handle reviewing spec - opens design review viewer
  const handleReviewSpec = async () => {
    if (!activeTask?.id) return

    setIsActioning(true)
    try {
      const response = await api.getApiClient().v1SpecTasksDesignReviewsDetail(activeTask.id)
      const reviews = response.data?.reviews || []
      if (reviews.length > 0) {
        const latestReview = reviews.find((r: any) => r.status !== 'superseded') || reviews[0]
        // Navigate to the review page
        account.orgNavigate('project-task-review', {
          id: projectId,
          taskId: activeTask.id,
          reviewId: latestReview.id,
        })
      } else {
        snackbar.error('No design review found')
      }
    } catch (error) {
      console.error('Failed to fetch design reviews:', error)
      snackbar.error('Failed to load design review')
    } finally {
      setIsActioning(false)
    }
  }

  // Handle approving implementation
  const handleApproveImplementation = async () => {
    if (!activeTask?.id) return

    setIsActioning(true)
    try {
      const response = await api.getApiClient().v1SpecTasksApproveImplementationCreate(activeTask.id)
      if (response.data?.pull_request_url) {
        snackbar.success(`Pull request opened! View PR: ${response.data.pull_request_url}`)
      } else if (response.data?.pull_request_id) {
        snackbar.success('Pull request #' + response.data.pull_request_id + ' opened - awaiting merge')
      } else {
        snackbar.success('Implementation approved! Agent will merge to your primary branch...')
      }
    } catch (err: any) {
      console.error('Failed to approve implementation:', err)
      snackbar.error(err?.response?.data?.message || 'Failed to approve implementation')
    } finally {
      setIsActioning(false)
    }
  }

  // Handle rejecting task - show confirmation dialog
  const handleRejectTask = () => {
    if (!activeTask?.id) return
    setArchiveConfirmOpen(true)
  }

  // Actually perform the archive operation (called after confirmation)
  const performArchive = async () => {
    if (!activeTask?.id) return

    setArchiveConfirmOpen(false)
    setIsActioning(true)
    try {
      await api.getApiClient().v1SpecTasksArchivePartialUpdate(activeTask.id, { archived: true })
      snackbar.success('Task rejected and archived')
      // Close the tab after archiving
      onTabClose(panel.id, activeTask.id)
    } catch (err: any) {
      console.error('Failed to reject task:', err)
      snackbar.error(err?.response?.data?.message || 'Failed to reject task')
    } finally {
      setIsActioning(false)
    }
  }

  const handleDragStart = (e: React.DragEvent, tabId: string) => {
    e.dataTransfer.setData('tabId', tabId)
    e.dataTransfer.setData('fromPanelId', panel.id)
    setDraggedTabId(tabId)
    setDraggedFromPanelId(panel.id)
  }

  const handleDragOver = (e: React.DragEvent) => {
    e.preventDefault()
    const rect = e.currentTarget.getBoundingClientRect()
    const x = e.clientX - rect.left
    const y = e.clientY - rect.top
    const edgeThreshold = 60

    if (x < edgeThreshold) setDragOverEdge('left')
    else if (x > rect.width - edgeThreshold) setDragOverEdge('right')
    else if (y < edgeThreshold) setDragOverEdge('top')
    else if (y > rect.height - edgeThreshold) setDragOverEdge('bottom')
    else setDragOverEdge(null)
  }

  const handleDragLeave = () => setDragOverEdge(null)

  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault()
    const tabId = e.dataTransfer.getData('tabId')
    const fromPanelId = e.dataTransfer.getData('fromPanelId')

    if (dragOverEdge) {
      // Split the panel
      const direction = (dragOverEdge === 'left' || dragOverEdge === 'right') ? 'horizontal' : 'vertical'
      onSplitPanel(panel.id, direction, tabId)
    } else if (fromPanelId !== panel.id) {
      // Move tab to this panel
      onDropTab(panel.id, tabId, fromPanelId)
    }

    setDragOverEdge(null)
    setDraggedTabId(null)
    setDraggedFromPanelId(null)
  }

  return (
    <Box
      ref={panelRef}
      sx={{
        display: 'flex',
        flexDirection: 'column',
        height: '100%',
        position: 'relative',
        backgroundColor: 'background.default',
      }}
      onDragOver={handleDragOver}
      onDragLeave={handleDragLeave}
      onDrop={handleDrop}
    >
      {/* Drop zone indicators */}
      {dragOverEdge && (
        <Box
          sx={{
            position: 'absolute',
            ...(dragOverEdge === 'left' && { left: 0, top: 0, bottom: 0, width: '50%' }),
            ...(dragOverEdge === 'right' && { right: 0, top: 0, bottom: 0, width: '50%' }),
            ...(dragOverEdge === 'top' && { top: 0, left: 0, right: 0, height: '50%' }),
            ...(dragOverEdge === 'bottom' && { bottom: 0, left: 0, right: 0, height: '50%' }),
            backgroundColor: 'primary.main',
            opacity: 0.15,
            zIndex: 10,
            pointerEvents: 'none',
          }}
        />
      )}

      {/* Tab bar */}
      <Box
        sx={{
          display: 'flex',
          alignItems: 'center',
          borderBottom: '1px solid',
          borderColor: 'divider',
          backgroundColor: 'background.paper',
          minHeight: 32,
        }}
      >
        <Box sx={{
          display: 'flex',
          flex: 1,
          overflowX: 'auto',
          '&::-webkit-scrollbar': { height: 2 },
          // Prevent text selection when dragging tabs on iPad with trackpad
          userSelect: 'none',
          WebkitUserSelect: 'none',
        }}>
          {panel.tabs.map(tab => (
            <PanelTab
              key={tab.id}
              tab={tab}
              isActive={tab.id === panel.activeTabId}
              onSelect={() => onTabSelect(panel.id, tab.id)}
              onClose={(e) => { e.stopPropagation(); onTabClose(panel.id, tab.id) }}
              onRename={(title) => onTabRename(tab.id, title)}
              onDragStart={handleDragStart}
              onTouchDragStart={(tabId) => onTouchDragStart(panel.id, tabId)}
              onTouchDragEnd={(tabId, clientX, clientY) => onTouchDragEnd(panel.id, tabId, clientX, clientY)}
            />
          ))}
        </Box>

        {/* Panel actions */}
        <Box sx={{ display: 'flex', alignItems: 'center', px: 0.5 }}>
          <Tooltip title="Add task or desktop">
            <IconButton
              size="small"
              onClick={(e) => setMenuAnchor(e.currentTarget)}
              sx={{ opacity: 0.6, '&:hover': { opacity: 1 } }}
            >
              <AddIcon sx={{ fontSize: 16 }} />
            </IconButton>
          </Tooltip>
          <Tooltip title="Split horizontally">
            <IconButton
              size="small"
              onClick={() => onSplitPanel(panel.id, 'horizontal')}
              sx={{ opacity: 0.6, '&:hover': { opacity: 1 } }}
            >
              <SplitVerticalIcon sx={{ fontSize: 16 }} />
            </IconButton>
          </Tooltip>
          <Tooltip title="Split vertically">
            <IconButton
              size="small"
              onClick={() => onSplitPanel(panel.id, 'vertical')}
              sx={{ opacity: 0.6, '&:hover': { opacity: 1 } }}
            >
              <SplitHorizontalIcon sx={{ fontSize: 16 }} />
            </IconButton>
          </Tooltip>
          {panelCount > 1 && (
            <Tooltip title="Close panel">
              <IconButton
                size="small"
                onClick={() => onClosePanel(panel.id)}
                sx={{ opacity: 0.6, '&:hover': { opacity: 1 } }}
              >
                <CloseIcon sx={{ fontSize: 16 }} />
              </IconButton>
            </Tooltip>
          )}
        </Box>

        {/* Task picker menu */}
        <Menu
          anchorEl={menuAnchor}
          open={Boolean(menuAnchor)}
          onClose={() => setMenuAnchor(null)}
          slotProps={{ paper: { sx: { maxHeight: 400, width: 280 } } }}
        >
          {/* Create new task option */}
          {projectId && (
            <>
              <MenuItem
                onClick={() => {
                  setMenuAnchor(null)
                  setCreateDialogOpen(true)
                }}
                sx={{ color: 'primary.main' }}
              >
                <ListItemIcon>
                  <CreateIcon sx={{ fontSize: 16, color: 'primary.main' }} />
                </ListItemIcon>
                <ListItemText
                  primary="Create New Task"
                  primaryTypographyProps={{ fontWeight: 500, fontSize: '0.875rem' }}
                />
              </MenuItem>
              <Divider />
            </>
          )}

          {/* Team Desktop sessions from active tasks */}
          {(() => {
            const tasksWithSessions = tasks.filter(t => t.planning_session_id)
            if (tasksWithSessions.length > 0) {
              return (
                <>
                  {tasksWithSessions.slice(0, 5).map(task => {
                    const desktopTabId = `desktop-${task.planning_session_id}`
                    const alreadyOpen = panel.tabs.some(t => t.id === desktopTabId)
                    return (
                      <MenuItem
                        key={`desktop-${task.id}`}
                        onClick={() => {
                          if (!alreadyOpen) {
                            onAddDesktop(panel.id, task.planning_session_id!, 'Team Desktop')
                          }
                          setMenuAnchor(null)
                        }}
                        disabled={alreadyOpen}
                      >
                        <ListItemIcon>
                          <DesktopIcon sx={{ fontSize: 16, color: alreadyOpen ? 'text.disabled' : 'success.main' }} />
                        </ListItemIcon>
                        <ListItemText
                          primary="Team Desktop"
                          secondary={alreadyOpen ? 'Already open' : undefined}
                          primaryTypographyProps={{ fontSize: '0.875rem' }}
                          secondaryTypographyProps={{ fontSize: '0.7rem' }}
                        />
                      </MenuItem>
                    )
                  })}
                  <Divider sx={{ my: 0.5 }} />
                </>
              )
            }
            return null
          })()}

          {/* Tasks section */}
          <Typography variant="caption" sx={{ px: 2, py: 0.5, color: 'text.secondary', display: 'block' }}>
            Tasks
          </Typography>
          {unopenedTasks.length === 0 ? (
            <MenuItem disabled>
              <ListItemText primary="All tasks are open" primaryTypographyProps={{ fontSize: '0.875rem' }} />
            </MenuItem>
          ) : (
            unopenedTasks.slice(0, 15).map(task => (
              <MenuItem
                key={task.id}
                onClick={() => {
                  onAddTab(panel.id, task)
                  setMenuAnchor(null)
                }}
              >
                <ListItemIcon>
                  <CircleIcon
                    sx={{
                      fontSize: 8,
                      color:
                        task.status === 'implementation' || task.status === 'spec_generation'
                          ? '#22c55e'
                          : task.status === 'spec_review'
                          ? '#3b82f6'
                          : '#9ca3af',
                    }}
                  />
                </ListItemIcon>
                <ListItemText
                  primary={task.user_short_title || task.short_title || task.name?.substring(0, 30) || 'Task'}
                  primaryTypographyProps={{ noWrap: true, fontSize: '0.875rem' }}
                />
              </MenuItem>
            ))
          )}
        </Menu>
      </Box>

      {/* Content area */}
      <Box sx={{ flex: 1, overflow: 'hidden' }}>
        {activeTab ? (
          activeTab.type === 'desktop' && activeTab.sessionId ? (
            <Box sx={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
              <ExternalAgentDesktopViewer
                key={activeTab.id}
                sessionId={activeTab.sessionId}
                sandboxId={activeTab.sessionId}
                mode="stream"
              />
              <Box sx={{ p: 2, borderTop: 1, borderColor: 'divider', flexShrink: 0 }}>
                <RobustPromptInput
                  sessionId={activeTab.sessionId}
                  projectId={projectId}
                  apiClient={api.getApiClient()}
                  onSend={async (message: string, interrupt?: boolean) => {
                    await streaming.NewInference({
                      type: SESSION_TYPE_TEXT,
                      message,
                      sessionId: activeTab.sessionId!,
                      interrupt: interrupt ?? true,
                    })
                  }}
                  placeholder="Send message to agent..."
                />
              </Box>
            </Box>
          ) : activeTab.type === 'review' && activeTab.taskId && activeTab.reviewId ? (
            <DesignReviewContent
              key={activeTab.id}
              specTaskId={activeTab.taskId}
              reviewId={activeTab.reviewId}
              onClose={() => onTabClose(panel.id, activeTab.id)}
              hideTitle={true}
            />
          ) : (
            <SpecTaskDetailContent
              key={activeTab.id}
              taskId={activeTab.id}
              onOpenReview={onOpenReview}
            />
          )
        ) : (
          <Box
            sx={{
              display: 'flex',
              flexDirection: 'column',
              alignItems: 'center',
              justifyContent: 'center',
              height: '100%',
              gap: 1,
            }}
          >
            <Typography variant="body2" color="text.secondary">
              No task selected
            </Typography>
            <Typography variant="caption" color="text.disabled">
              Drag a tab here or click + to add
            </Typography>
          </Box>
        )}
      </Box>

      {/* Create task dialog */}
      <Dialog
        open={createDialogOpen}
        onClose={() => !isCreating && setCreateDialogOpen(false)}
        maxWidth="sm"
        fullWidth
      >
        <DialogTitle>Create New Task</DialogTitle>
        <DialogContent>
          <TextField
            autoFocus
            fullWidth
            multiline
            rows={3}
            placeholder="Describe what you want to get done..."
            value={createPrompt}
            onChange={(e) => setCreatePrompt(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter' && e.metaKey) {
                e.preventDefault()
                handleCreateTask()
              }
            }}
            disabled={isCreating}
            sx={{ mt: 1 }}
          />
          <Typography variant="caption" color="text.secondary" sx={{ mt: 1, display: 'block' }}>
            Press ⌘+Enter to create
          </Typography>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setCreateDialogOpen(false)} disabled={isCreating}>
            Cancel
          </Button>
          <Button
            onClick={handleCreateTask}
            variant="contained"
            disabled={isCreating || !createPrompt.trim()}
            startIcon={isCreating ? <CircularProgress size={16} /> : undefined}
          >
            {isCreating ? 'Creating...' : 'Create Task'}
          </Button>
        </DialogActions>
      </Dialog>

      {/* Archive Confirmation Dialog */}
      <ArchiveConfirmDialog
        open={archiveConfirmOpen}
        onClose={() => setArchiveConfirmOpen(false)}
        onConfirm={performArchive}
        taskName={activeTask?.name || activeTask?.description}
        isArchiving={isActioning}
      />
    </Box>
  )
}

// Resize handle component
const ResizeHandle: React.FC<{ direction: 'horizontal' | 'vertical' }> = ({ direction }) => (
  <PanelResizeHandle
    style={{
      width: direction === 'horizontal' ? 4 : '100%',
      height: direction === 'horizontal' ? '100%' : 4,
      backgroundColor: 'transparent',
      cursor: direction === 'horizontal' ? 'col-resize' : 'row-resize',
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
      transition: 'background-color 0.15s',
    }}
  >
    <Box
      sx={{
        width: direction === 'horizontal' ? 2 : '30%',
        height: direction === 'horizontal' ? '30%' : 2,
        backgroundColor: 'divider',
        borderRadius: 1,
        transition: 'all 0.15s',
        '&:hover': {
          backgroundColor: 'primary.main',
          width: direction === 'horizontal' ? 3 : '40%',
          height: direction === 'horizontal' ? '40%' : 3,
        },
      }}
    />
  </PanelResizeHandle>
)

interface TabsViewProps {
  projectId?: string
  tasks: TypesSpecTask[]
  onCreateTask?: () => void
  onRefresh?: () => void
  initialTaskId?: string // Task ID to open initially (from "Split Screen" button)
  initialDesktopId?: string // Desktop session ID to open initially (from "Split Screen" button)
}

// localStorage key for workspace state
const WORKSPACE_STATE_KEY = 'helix_workspace_state'

interface SavedWorkspaceState {
  projectId: string
  panels: {
    id: string
    tabIds: string[] // Just task IDs, not full task objects
    activeTabId: string | null
  }[]
  layoutDirection: 'horizontal' | 'vertical'
}

const TabsView: React.FC<TabsViewProps> = ({
  projectId,
  tasks,
  onCreateTask,
  onRefresh,
  initialTaskId,
  initialDesktopId,
}) => {
  const snackbar = useSnackbar()
  const updateSpecTask = useUpdateSpecTask()

  // Track if we've initialized from saved state
  const [initialized, setInitialized] = useState(false)

  // Layout state: array of panel rows, each row has panels
  const [panels, setPanels] = useState<PanelData[]>([])
  const [layoutDirection, setLayoutDirection] = useState<'horizontal' | 'vertical'>('horizontal')

  // Touch drag state - store refs to panel elements
  const panelRefsMap = React.useRef<Map<string, HTMLDivElement>>(new Map())
  const [touchDragInfo, setTouchDragInfo] = useState<{ panelId: string; tabId: string } | null>(null)

  // Save workspace state to localStorage whenever panels change
  useEffect(() => {
    if (!projectId || panels.length === 0) return

    const savedState: SavedWorkspaceState = {
      projectId,
      panels: panels.map(p => ({
        id: p.id,
        tabIds: p.tabs.map(t => t.id),
        activeTabId: p.activeTabId,
      })),
      layoutDirection,
    }
    localStorage.setItem(WORKSPACE_STATE_KEY, JSON.stringify(savedState))
  }, [panels, layoutDirection, projectId])

  // Initialize workspace: restore from localStorage or start fresh
  useEffect(() => {
    if (initialized || tasks.length === 0) return

    // Try to restore from localStorage
    const savedJson = localStorage.getItem(WORKSPACE_STATE_KEY)
    let restored = false

    if (savedJson && projectId) {
      try {
        const saved: SavedWorkspaceState = JSON.parse(savedJson)

        // Only restore if it's for the same project
        if (saved.projectId === projectId && saved.panels.length > 0) {
          // Rebuild panels with current task data (filter out tasks that no longer exist)
          const restoredPanels: PanelData[] = []

          for (const savedPanel of saved.panels) {
            const tabs: TabData[] = []
            for (const taskId of savedPanel.tabIds) {
              const task = tasks.find(t => t.id === taskId)
              if (task) {
                tabs.push({ id: taskId, type: 'task', task })
              }
            }

            if (tabs.length > 0) {
              // Ensure activeTabId is valid
              const activeTabId = tabs.some(t => t.id === savedPanel.activeTabId)
                ? savedPanel.activeTabId
                : tabs[0].id

              restoredPanels.push({
                id: savedPanel.id,
                tabs,
                activeTabId,
              })
            }
          }

          if (restoredPanels.length > 0) {
            setPanels(restoredPanels)
            setLayoutDirection(saved.layoutDirection)
            restored = true
          }
        }
      } catch (e) {
        console.warn('Failed to restore workspace state:', e)
      }
    }

    // If initialTaskId is provided, ensure it's open (even if we restored state)
    if (initialTaskId) {
      const taskToOpen = tasks.find(t => t.id === initialTaskId)
      if (taskToOpen) {
        if (restored) {
          // Add to first panel if not already open
          setPanels(prev => {
            const alreadyOpen = prev.some(p => p.tabs.some(t => t.id === initialTaskId))
            if (alreadyOpen) {
              // Just activate it
              return prev.map(p => {
                if (p.tabs.some(t => t.id === initialTaskId)) {
                  return { ...p, activeTabId: initialTaskId }
                }
                return p
              })
            }
            // Add to first panel
            if (prev.length > 0) {
              return prev.map((p, i) => i === 0 ? {
                ...p,
                tabs: [...p.tabs, { id: initialTaskId, type: 'task', task: taskToOpen }],
                activeTabId: initialTaskId,
              } : p)
            }
            return prev
          })
        } else {
          // Start fresh with this task - create two panels for obvious split-screen
          // Second panel shows another task or stays empty for user to add
          const otherTasks = tasks.filter(t => t.id !== initialTaskId)
          const secondTask = otherTasks.length > 0 ? otherTasks[0] : null

          const firstPanel: PanelData = {
            id: generatePanelId(),
            tabs: [{ id: taskToOpen.id, type: 'task', task: taskToOpen }],
            activeTabId: taskToOpen.id,
          }

          if (secondTask) {
            // Create split with two tasks
            setPanels([
              firstPanel,
              {
                id: generatePanelId(),
                tabs: [{ id: secondTask.id, type: 'task', task: secondTask }],
                activeTabId: secondTask.id,
              }
            ])
          } else {
            // Only one task exists, just show single panel
            setPanels([firstPanel])
          }
          restored = true
        }
      }
    }

    // If initialDesktopId is provided, ensure the desktop tab is open
    if (initialDesktopId) {
      const desktopTabId = `desktop-${initialDesktopId}`
      if (restored) {
        // Add to first panel if not already open
        setPanels(prev => {
          const alreadyOpen = prev.some(p => p.tabs.some(t => t.id === desktopTabId))
          if (alreadyOpen) {
            // Just activate it
            return prev.map(p => {
              if (p.tabs.some(t => t.id === desktopTabId)) {
                return { ...p, activeTabId: desktopTabId }
              }
              return p
            })
          }
          // Add to first panel
          if (prev.length > 0) {
            return prev.map((p, i) => i === 0 ? {
              ...p,
              tabs: [...p.tabs, {
                id: desktopTabId,
                type: 'desktop',
                sessionId: initialDesktopId,
                desktopTitle: 'Team Desktop',
              }],
              activeTabId: desktopTabId,
            } : p)
          }
          return prev
        })
      } else {
        // Start fresh with this desktop
        setPanels([{
          id: generatePanelId(),
          tabs: [{
            id: desktopTabId,
            type: 'desktop',
            sessionId: initialDesktopId,
            desktopTitle: 'Team Desktop',
          }],
          activeTabId: desktopTabId,
        }])
        restored = true
      }
    }

    // If nothing restored and no initialTaskId/initialDesktopId, open most recently updated task
    if (!restored) {
      const sortedTasks = [...tasks].sort((a, b) => {
        const aDate = new Date(a.updated_at || a.created_at || 0).getTime()
        const bDate = new Date(b.updated_at || b.created_at || 0).getTime()
        return bDate - aDate
      })
      const taskToOpen = sortedTasks[0]

      if (taskToOpen?.id) {
        setPanels([{
          id: generatePanelId(),
          tabs: [{ id: taskToOpen.id, type: 'task', task: taskToOpen }],
          activeTabId: taskToOpen.id,
        }])
      }
    }

    setInitialized(true)
  }, [tasks, initialized, initialTaskId, initialDesktopId, projectId])

  const handleTabSelect = useCallback((panelId: string, tabId: string) => {
    setPanels(prev => prev.map(p =>
      p.id === panelId ? { ...p, activeTabId: tabId } : p
    ))
  }, [])

  const handleTabClose = useCallback((panelId: string, tabId: string) => {
    setPanels(prev => {
      const panel = prev.find(p => p.id === panelId)
      if (!panel) return prev

      const newTabs = panel.tabs.filter(t => t.id !== tabId)

      // If panel has no tabs left, remove it (unless it's the only panel)
      if (newTabs.length === 0 && prev.length > 1) {
        return prev.filter(p => p.id !== panelId)
      }

      let newActiveTabId = panel.activeTabId
      if (panel.activeTabId === tabId && newTabs.length > 0) {
        const closedIndex = panel.tabs.findIndex(t => t.id === tabId)
        const newActiveIndex = Math.min(closedIndex, newTabs.length - 1)
        newActiveTabId = newTabs[newActiveIndex]?.id || null
      } else if (newTabs.length === 0) {
        newActiveTabId = null
      }

      return prev.map(p =>
        p.id === panelId ? { ...p, tabs: newTabs, activeTabId: newActiveTabId } : p
      )
    })
  }, [])

  const handleTabRename = useCallback(async (tabId: string, newTitle: string) => {
    try {
      await updateSpecTask.mutateAsync({
        taskId: tabId,
        updates: { user_short_title: newTitle },
      })
      snackbar.success('Tab renamed')
    } catch (err) {
      console.error('Failed to rename tab:', err)
      snackbar.error('Failed to rename tab')
    }
  }, [updateSpecTask, snackbar])

  const handleAddTab = useCallback((panelId: string, task: TypesSpecTask) => {
    if (!task.id) return
    setPanels(prev => prev.map(p => {
      if (p.id !== panelId) return p
      // Check if tab already exists
      if (p.tabs.some(t => t.id === task.id)) {
        return { ...p, activeTabId: task.id }
      }
      return {
        ...p,
        tabs: [...p.tabs, { id: task.id, type: 'task', task }],
        activeTabId: task.id,
      }
    }))
  }, [])

  const handleSplitPanel = useCallback((panelId: string, direction: 'horizontal' | 'vertical', taskId?: string) => {
    setPanels(prev => {
      const panelIndex = prev.findIndex(p => p.id === panelId)
      if (panelIndex === -1) return prev

      const sourcePanel = prev[panelIndex]
      let tabToMove: TabData | undefined
      let newSourceTabs = sourcePanel.tabs

      if (taskId) {
        tabToMove = sourcePanel.tabs.find(t => t.id === taskId)
        if (tabToMove) {
          newSourceTabs = sourcePanel.tabs.filter(t => t.id !== taskId)
        }
      }

      const newPanel: PanelData = {
        id: generatePanelId(),
        tabs: tabToMove ? [tabToMove] : [],
        activeTabId: tabToMove?.id || null,
      }

      // Update layout direction if needed
      setLayoutDirection(direction)

      // Update source panel and add new panel
      const updatedPanels = [...prev]
      updatedPanels[panelIndex] = {
        ...sourcePanel,
        tabs: newSourceTabs,
        activeTabId: newSourceTabs.length > 0
          ? (newSourceTabs.some(t => t.id === sourcePanel.activeTabId)
              ? sourcePanel.activeTabId
              : newSourceTabs[0].id)
          : null,
      }
      updatedPanels.splice(panelIndex + 1, 0, newPanel)

      return updatedPanels
    })
  }, [])

  const handleDropTab = useCallback((targetPanelId: string, tabId: string, fromPanelId: string) => {
    setPanels(prev => {
      const sourcePanel = prev.find(p => p.id === fromPanelId)
      const targetPanel = prev.find(p => p.id === targetPanelId)
      if (!sourcePanel || !targetPanel) return prev

      const tabToMove = sourcePanel.tabs.find(t => t.id === tabId)
      if (!tabToMove) return prev

      // Check if already in target
      if (targetPanel.tabs.some(t => t.id === tabId)) return prev

      return prev.map(p => {
        if (p.id === fromPanelId) {
          const newTabs = p.tabs.filter(t => t.id !== tabId)
          return {
            ...p,
            tabs: newTabs,
            activeTabId: newTabs.length > 0
              ? (newTabs.some(t => t.id === p.activeTabId) ? p.activeTabId : newTabs[0].id)
              : null,
          }
        }
        if (p.id === targetPanelId) {
          return {
            ...p,
            tabs: [...p.tabs, tabToMove],
            activeTabId: tabId,
          }
        }
        return p
      }).filter(p => p.tabs.length > 0 || prev.length <= 1)
    })
  }, [])

  const handleClosePanel = useCallback((panelId: string) => {
    setPanels(prev => {
      if (prev.length <= 1) return prev
      return prev.filter(p => p.id !== panelId)
    })
  }, [])

  // Handle task created - add it to the specified panel
  const handleTaskCreated = useCallback((panelId: string, task: TypesSpecTask) => {
    if (!task.id) return
    setPanels(prev => prev.map(p => {
      if (p.id !== panelId) return p
      // Add new task as a tab and make it active
      return {
        ...p,
        tabs: [...p.tabs, { id: task.id!, type: 'task', task }],
        activeTabId: task.id!,
      }
    }))
  }, [])

  // Handle adding a Team Desktop tab to a panel
  const handleAddDesktop = useCallback((panelId: string, sessionId: string, title?: string) => {
    const desktopTabId = `desktop-${sessionId}`
    setPanels(prev => prev.map(p => {
      if (p.id !== panelId) return p
      // Check if already open
      if (p.tabs.some(t => t.id === desktopTabId)) {
        return { ...p, activeTabId: desktopTabId }
      }
      // Add new desktop tab
      return {
        ...p,
        tabs: [...p.tabs, {
          id: desktopTabId,
          type: 'desktop' as const,
          sessionId,
          desktopTitle: title || 'Team Desktop',
        }],
        activeTabId: desktopTabId,
      }
    }))
  }, [])

  // Handle opening a review in a new tab (called from SpecTaskDetailContent)
  const handleOpenReview = useCallback((taskId: string, reviewId: string, reviewTitle?: string) => {
    const tabId = `review-${taskId}-${reviewId}`

    setPanels(prev => {
      // Check if this review is already open in any panel
      for (const panel of prev) {
        if (panel.tabs.some(t => t.id === tabId)) {
          // Activate it
          return prev.map(p =>
            p.tabs.some(t => t.id === tabId)
              ? { ...p, activeTabId: tabId }
              : p
          )
        }
      }

      // Add to the first panel (or create a new panel if we want split behavior)
      if (prev.length > 0) {
        return prev.map((p, i) => i === 0 ? {
          ...p,
          tabs: [...p.tabs, {
            id: tabId,
            type: 'review' as const,
            taskId,
            reviewId,
            reviewTitle: reviewTitle || 'Spec Review',
          }],
          activeTabId: tabId,
        } : p)
      }

      return prev
    })
  }, [])

  // Touch drag handlers for iPad/mobile
  const handleTouchDragStart = useCallback((panelId: string, tabId: string) => {
    setTouchDragInfo({ panelId, tabId })
  }, [])

  const handleTouchDragEnd = useCallback((fromPanelId: string, tabId: string, clientX: number, clientY: number) => {
    if (!touchDragInfo) return
    setTouchDragInfo(null)

    // Find which panel the touch ended on
    let targetPanelId: string | null = null
    panelRefsMap.current.forEach((el, panelId) => {
      const rect = el.getBoundingClientRect()
      if (clientX >= rect.left && clientX <= rect.right &&
          clientY >= rect.top && clientY <= rect.bottom) {
        targetPanelId = panelId
      }
    })

    // If dropped on a different panel, move the tab
    if (targetPanelId && targetPanelId !== fromPanelId) {
      handleDropTab(targetPanelId, tabId, fromPanelId)
    }
  }, [touchDragInfo, handleDropTab])

  // Create a stable ref callback for each panel
  const getPanelRef = useCallback((panelId: string) => {
    return (el: HTMLDivElement | null) => {
      if (el) {
        panelRefsMap.current.set(panelId, el)
      } else {
        panelRefsMap.current.delete(panelId)
      }
    }
  }, [])

  // When no panels exist, show an empty panel with just a + button
  if (panels.length === 0) {
    return (
      <Box sx={{ height: '100%', display: 'flex', flexDirection: 'column' }}>
        {/* Tab bar with just the + button */}
        <Box
          sx={{
            display: 'flex',
            alignItems: 'center',
            borderBottom: '1px solid',
            borderColor: 'divider',
            backgroundColor: 'background.paper',
            minHeight: 32,
          }}
        >
          <Box sx={{ flex: 1 }} />
          <Box sx={{ display: 'flex', alignItems: 'center', px: 0.5 }}>
            <Tooltip title="Create new task">
              <IconButton
                size="small"
                onClick={onCreateTask}
                sx={{ opacity: 0.8, '&:hover': { opacity: 1 } }}
              >
                <AddIcon sx={{ fontSize: 18 }} />
              </IconButton>
            </Tooltip>
          </Box>
        </Box>
        {/* Empty state content */}
        <Box
          sx={{
            flex: 1,
            display: 'flex',
            flexDirection: 'column',
            alignItems: 'center',
            justifyContent: 'center',
            gap: 2,
          }}
        >
          <Typography variant="h6" color="text.secondary">
            No tasks to display
          </Typography>
          <Typography variant="body2" color="text.disabled">
            Click + above to create a task
          </Typography>
        </Box>
      </Box>
    )
  }

  return (
    <Box sx={{ height: '100%', overflow: 'hidden' }}>
      <PanelGroup orientation={layoutDirection} style={{ height: '100%' }}>
        {panels.map((panel, index) => (
          <React.Fragment key={panel.id}>
            {index > 0 && <ResizeHandle direction={layoutDirection} />}
            <Panel defaultSize={100 / panels.length} minSize={15}>
              <TaskPanel
                panel={panel}
                tasks={tasks}
                projectId={projectId}
                onTabSelect={handleTabSelect}
                onTabClose={handleTabClose}
                onTabRename={handleTabRename}
                onAddTab={handleAddTab}
                onAddDesktop={handleAddDesktop}
                onTaskCreated={handleTaskCreated}
                onSplitPanel={handleSplitPanel}
                onDropTab={handleDropTab}
                onClosePanel={handleClosePanel}
                onOpenReview={handleOpenReview}
                onTouchDragStart={handleTouchDragStart}
                onTouchDragEnd={handleTouchDragEnd}
                panelCount={panels.length}
                panelRef={getPanelRef(panel.id)}
              />
            </Panel>
          </React.Fragment>
        ))}
      </PanelGroup>
    </Box>
  )
}

export default TabsView
