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
  RateReview as ReviewIcon,
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
import NewSpecTaskForm from './NewSpecTaskForm'
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
  type: 'task' | 'review' | 'desktop' | 'create'
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

// Tree-based panel node for nested layouts
// A node is either a "leaf" (a panel with tabs) or a "split" (a container with children)
interface PanelNode {
  id: string
  type: 'leaf' | 'split'
  // For leaf nodes (panels with tabs):
  tabs?: TabData[]
  activeTabId?: string | null
  // For split nodes (containers):
  direction?: 'horizontal' | 'vertical'
  children?: PanelNode[]
}

// Helper to create a leaf node
const createLeafNode = (tabs: TabData[] = [], activeTabId: string | null = null): PanelNode => ({
  id: generatePanelId(),
  type: 'leaf',
  tabs,
  activeTabId,
})

// Helper to create a split node
const createSplitNode = (direction: 'horizontal' | 'vertical', children: PanelNode[]): PanelNode => ({
  id: generatePanelId(),
  type: 'split',
  direction,
  children,
})

// Find a node by ID in the tree
const findNode = (root: PanelNode | null, nodeId: string): PanelNode | null => {
  if (!root) return null
  if (root.id === nodeId) return root
  if (root.type === 'split' && root.children) {
    for (const child of root.children) {
      const found = findNode(child, nodeId)
      if (found) return found
    }
  }
  return null
}

// Update a node in the tree immutably
const updateNodeInTree = (
  root: PanelNode,
  nodeId: string,
  updater: (node: PanelNode) => PanelNode
): PanelNode => {
  if (root.id === nodeId) {
    return updater(root)
  }
  if (root.type === 'split' && root.children) {
    return {
      ...root,
      children: root.children.map(child => updateNodeInTree(child, nodeId, updater)),
    }
  }
  return root
}

// Replace a node in the tree immutably
const replaceNodeInTree = (root: PanelNode, nodeId: string, newNode: PanelNode): PanelNode => {
  if (root.id === nodeId) {
    return newNode
  }
  if (root.type === 'split' && root.children) {
    return {
      ...root,
      children: root.children.map(child => replaceNodeInTree(child, nodeId, newNode)),
    }
  }
  return root
}

// Remove a node from the tree and collapse parent if needed
const removeNodeFromTree = (root: PanelNode, nodeId: string): PanelNode | null => {
  if (root.id === nodeId) {
    return null // Root itself is being removed
  }
  if (root.type === 'split' && root.children) {
    const newChildren = root.children
      .map(child => {
        if (child.id === nodeId) return null
        if (child.type === 'split') {
          return removeNodeFromTree(child, nodeId)
        }
        return child
      })
      .filter((c): c is PanelNode => c !== null)

    // If only one child left, collapse the split and return that child
    if (newChildren.length === 1) {
      return newChildren[0]
    }
    // If no children left, this split should be removed
    if (newChildren.length === 0) {
      return null
    }
    return { ...root, children: newChildren }
  }
  return root
}

// Count leaf nodes in tree
const countLeafNodes = (root: PanelNode | null): number => {
  if (!root) return 0
  if (root.type === 'leaf') return 1
  return root.children?.reduce((sum, child) => sum + countLeafNodes(child), 0) || 0
}

// Get all leaf nodes as flat array (for iteration)
const getAllLeafNodes = (root: PanelNode | null): PanelNode[] => {
  if (!root) return []
  if (root.type === 'leaf') return [root]
  return root.children?.flatMap(getAllLeafNodes) || []
}

// Helper to check if a task has a spec review available
const taskHasSpecReview = (task: TypesSpecTask): boolean => {
  const status = task.status || ''
  const statusesWithSpec = [
    'spec_review',
    'spec_revision',
    'spec_approved',
    'implementation_queued',
    'implementation',
    'implementation_review',
    'pull_request',
    'done',
    'spec_failed',
  ]
  return statusesWithSpec.includes(status) ||
    !!(task.requirements_spec) ||
    !!(task.design_doc_path)
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
  // Review tabs always get a "Review:" prefix for distinguishability
  // Don't truncate here - let CSS handle ellipsis so full text is available for editing
  const displayTitle = tab.type === 'create'
    ? 'New Task'
    : tab.type === 'review'
    ? (tab.reviewTitle?.startsWith('Review:')
        ? tab.reviewTitle
        : `Review: ${tab.reviewTitle || 'Spec'}`)
    : tab.type === 'desktop'
    ? (tab.desktopTitle || 'Team Desktop')
    : (displayTask?.user_short_title
      || displayTask?.short_title
      || displayTask?.name
      || 'Task')

  // Format title history for tooltip
  const tooltipContent = useMemo(() => {
    // Review tabs have a simple tooltip with full title
    if (tab.type === 'review') {
      const title = tab.reviewTitle?.startsWith('Review:')
        ? tab.reviewTitle
        : `Review: ${tab.reviewTitle || 'Spec'}`
      return title
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
          minWidth: 80,
          maxWidth: 280,
          flexShrink: 1,
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
  exploratorySessionId?: string
  onTabSelect: (panelId: string, tabId: string) => void
  onTabClose: (panelId: string, tabId: string) => void
  onTabRename: (tabId: string, newTitle: string) => void
  onAddTab: (panelId: string, task: TypesSpecTask) => void
  onAddDesktop: (panelId: string, sessionId: string, title?: string) => void
  onAddCreateTab: (panelId: string) => void
  onTaskCreated: (panelId: string, task: TypesSpecTask) => void
  onSplitPanel: (panelId: string, direction: 'horizontal' | 'vertical', taskId?: string) => void
  onDropTab: (panelId: string, tabId: string, fromPanelId: string) => void
  onClosePanel: (panelId: string) => void
  onOpenReview: (taskId: string, reviewId: string, reviewTitle?: string, sourcePanelId?: string) => void
  onTouchDragStart: (panelId: string, tabId: string) => void
  onTouchDragEnd: (panelId: string, tabId: string, clientX: number, clientY: number) => void
  panelCount: number
  panelRef: (el: HTMLDivElement | null) => void
}

const TaskPanel: React.FC<TaskPanelProps> = ({
  panel,
  tasks,
  projectId,
  exploratorySessionId,
  onTabSelect,
  onTabClose,
  onTabRename,
  onAddTab,
  onAddDesktop,
  onAddCreateTab,
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
  const [isActioning, setIsActioning] = useState(false)
  const [dragOverEdge, setDragOverEdge] = useState<'left' | 'right' | 'top' | 'bottom' | null>(null)
  const [draggedTabId, setDraggedTabId] = useState<string | null>(null)
  const [draggedFromPanelId, setDraggedFromPanelId] = useState<string | null>(null)

  // Archive/reject confirmation dialog state
  const [archiveConfirmOpen, setArchiveConfirmOpen] = useState(false)

  // Track when dragging over the tab bar specifically (for move, not split)
  const [dragOverTabBar, setDragOverTabBar] = useState(false)

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

  // Handle opening the latest spec review for any task (used by dropdown menu)
  const handleOpenTaskReview = async (task: TypesSpecTask) => {
    if (!task.id) return

    try {
      const response = await api.getApiClient().v1SpecTasksDesignReviewsDetail(task.id)
      const reviews = response.data?.reviews || []
      if (reviews.length > 0) {
        const latestReview = reviews.find((r: any) => r.status !== 'superseded') || reviews[0]
        const taskTitle = task.user_short_title || task.short_title || task.name || 'Task'
        onOpenReview(task.id, latestReview.id, `Review: ${taskTitle}`, panel.id)
      } else {
        snackbar.error('No design review found for this task')
      }
    } catch (error) {
      console.error('Failed to fetch design reviews:', error)
      snackbar.error('Failed to load design review')
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

  // Tab bar specific drag handlers - dropping on tab bar should move tab, not split
  const handleTabBarDragOver = (e: React.DragEvent) => {
    e.preventDefault()
    e.stopPropagation() // Prevent panel's handleDragOver from detecting "top" edge
    setDragOverTabBar(true)
    setDragOverEdge(null) // Clear any edge detection
  }

  const handleTabBarDragLeave = (e: React.DragEvent) => {
    // Only clear if we're actually leaving the tab bar, not just moving to a child
    const relatedTarget = e.relatedTarget as HTMLElement
    const currentTarget = e.currentTarget as HTMLElement
    if (!currentTarget.contains(relatedTarget)) {
      setDragOverTabBar(false)
    }
  }

  const handleTabBarDrop = (e: React.DragEvent) => {
    e.preventDefault()
    e.stopPropagation() // Prevent panel's handleDrop from triggering a split
    const tabId = e.dataTransfer.getData('tabId')
    const fromPanelId = e.dataTransfer.getData('fromPanelId')

    // Move tab to this panel (not split)
    if (tabId && fromPanelId && fromPanelId !== panel.id) {
      onDropTab(panel.id, tabId, fromPanelId)
    }

    setDragOverTabBar(false)
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
        onDragOver={handleTabBarDragOver}
        onDragLeave={handleTabBarDragLeave}
        onDrop={handleTabBarDrop}
        sx={{
          display: 'flex',
          alignItems: 'center',
          borderBottom: '1px solid',
          borderColor: dragOverTabBar ? 'primary.main' : 'divider',
          backgroundColor: dragOverTabBar ? 'action.hover' : 'background.paper',
          minHeight: 32,
          transition: 'background-color 0.15s, border-color 0.15s',
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
                  onAddCreateTab(panel.id)
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

          {/* Team Desktop - the exploratory session for the project */}
          {exploratorySessionId && (() => {
            const desktopTabId = `desktop-${exploratorySessionId}`
            const alreadyOpen = panel.tabs.some(t => t.id === desktopTabId)
            return (
              <>
                <MenuItem
                  onClick={() => {
                    if (!alreadyOpen) {
                      onAddDesktop(panel.id, exploratorySessionId, 'Team Desktop')
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
                <Divider sx={{ my: 0.5 }} />
              </>
            )
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
              <React.Fragment key={task.id}>
                <MenuItem
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
                    primary={task.user_short_title || task.short_title || task.name || 'Task'}
                    primaryTypographyProps={{
                      noWrap: true,
                      fontSize: '0.875rem',
                      sx: {
                        overflow: 'hidden',
                        textOverflow: 'ellipsis',
                        maxWidth: 200,
                      },
                    }}
                  />
                </MenuItem>
                {/* Spec review sub-item for tasks that have specs */}
                {taskHasSpecReview(task) && (
                  <MenuItem
                    onClick={() => {
                      handleOpenTaskReview(task)
                      setMenuAnchor(null)
                    }}
                    sx={{ pl: 4 }}
                  >
                    <ListItemIcon>
                      <ReviewIcon sx={{ fontSize: 14, color: 'info.main' }} />
                    </ListItemIcon>
                    <ListItemText
                      primary={`Review: ${task.user_short_title || task.short_title || task.name || 'Spec'}`}
                      primaryTypographyProps={{
                        noWrap: true,
                        fontSize: '0.75rem',
                        sx: {
                          overflow: 'hidden',
                          textOverflow: 'ellipsis',
                          maxWidth: 180,
                        },
                      }}
                    />
                  </MenuItem>
                )}
              </React.Fragment>
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
          ) : activeTab.type === 'create' ? (
            <NewSpecTaskForm
              key={activeTab.id}
              projectId={projectId}
              onTaskCreated={(task) => {
                // Replace create tab with the new task tab
                onTaskCreated(panel.id, task)
                // Close the create tab
                onTabClose(panel.id, activeTab.id)
              }}
              onClose={() => onTabClose(panel.id, activeTab.id)}
              showHeader={false}
              embedded={true}
            />
          ) : (
            <SpecTaskDetailContent
              key={activeTab.id}
              taskId={activeTab.id}
              onOpenReview={(taskId, reviewId, reviewTitle) => onOpenReview(taskId, reviewId, reviewTitle, panel.id)}
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
  exploratorySessionId?: string // Team Desktop session ID (one per project)
}

// localStorage key for workspace state
const WORKSPACE_STATE_KEY = 'helix_workspace_state'

// Serialized node for persistence (stores tab IDs instead of full objects)
interface SerializedNode {
  id: string
  type: 'leaf' | 'split'
  // For leaf nodes:
  tabIds?: string[] // Tab IDs - will be rehydrated with full data
  activeTabId?: string | null
  // For split nodes:
  direction?: 'horizontal' | 'vertical'
  children?: SerializedNode[]
}

interface SavedWorkspaceState {
  version: 2 // Bump version for tree structure
  projectId: string
  rootNode: SerializedNode | null
}

// Legacy v1 format for migration
interface SavedWorkspaceStateV1 {
  projectId: string
  panels: {
    id: string
    tabIds: string[]
    activeTabId: string | null
  }[]
  layoutDirection: 'horizontal' | 'vertical'
}

// Serialize a PanelNode tree to SerializedNode (for localStorage)
// Uses :: delimiter for review tabs: review::taskId::reviewId
const serializeNode = (node: PanelNode): SerializedNode => {
  if (node.type === 'leaf') {
    // Filter out 'create' tabs - they shouldn't be persisted
    const persistableTabs = node.tabs?.filter(t => t.type !== 'create') || []
    return {
      id: node.id,
      type: 'leaf',
      tabIds: persistableTabs.map(t => {
        // Encode review tabs with all needed info
        if (t.type === 'review' && t.taskId && t.reviewId) {
          return `review::${t.taskId}::${t.reviewId}::${t.reviewTitle || 'Spec'}`
        }
        // Encode desktop tabs
        if (t.type === 'desktop' && t.sessionId) {
          return `desktop::${t.sessionId}::${t.desktopTitle || 'Desktop'}`
        }
        // Task tabs just use task ID
        return t.id
      }),
      activeTabId: node.activeTabId,
    }
  }
  return {
    id: node.id,
    type: 'split',
    direction: node.direction,
    children: node.children?.map(serializeNode) || [],
  }
}

// Deserialize SerializedNode to PanelNode (from localStorage)
// Rehydrates tabs with task data from the tasks array
const deserializeNode = (
  serialized: SerializedNode,
  tasks: TypesSpecTask[],
): PanelNode | null => {
  if (serialized.type === 'leaf') {
    const tabs: TabData[] = []
    for (const tabId of serialized.tabIds || []) {
      // Parse review tabs (format: review::taskId::reviewId::title)
      if (tabId.startsWith('review::')) {
        const parts = tabId.split('::')
        if (parts.length >= 3) {
          tabs.push({
            id: tabId,
            type: 'review',
            taskId: parts[1],
            reviewId: parts[2],
            reviewTitle: parts[3] || 'Spec',
          })
        }
        continue
      }
      // Parse desktop tabs (format: desktop::sessionId::title)
      if (tabId.startsWith('desktop::')) {
        const parts = tabId.split('::')
        if (parts.length >= 2) {
          tabs.push({
            id: tabId.replace('desktop::', 'desktop-'), // Keep desktop- prefix for display
            type: 'desktop',
            sessionId: parts[1],
            desktopTitle: parts[2] || 'Desktop',
          })
        }
        continue
      }
      // Regular task tabs
      const task = tasks.find(t => t.id === tabId)
      if (task) {
        tabs.push({ id: tabId, type: 'task', task })
      }
    }
    // Skip empty leaf nodes
    if (tabs.length === 0) return null
    return {
      id: serialized.id,
      type: 'leaf',
      tabs,
      activeTabId: tabs.some(t => t.id === serialized.activeTabId)
        ? serialized.activeTabId
        : tabs[0]?.id || null,
    }
  }

  // Split node
  const children = (serialized.children || [])
    .map(c => deserializeNode(c, tasks))
    .filter((c): c is PanelNode => c !== null)

  if (children.length === 0) return null
  if (children.length === 1) return children[0] // Collapse single-child splits

  return {
    id: serialized.id,
    type: 'split',
    direction: serialized.direction,
    children,
  }
}

// Migrate v1 state to v2 tree structure
const migrateV1ToV2 = (v1: SavedWorkspaceStateV1, tasks: TypesSpecTask[]): PanelNode | null => {
  if (v1.panels.length === 0) return null

  // Convert flat panels to leaf nodes
  const leafNodes: PanelNode[] = v1.panels
    .map(p => {
      const tabs: TabData[] = p.tabIds
        .map(id => {
          const task = tasks.find(t => t.id === id)
          if (task) return { id, type: 'task' as const, task }
          return null
        })
        .filter((t): t is TabData => t !== null)

      if (tabs.length === 0) return null
      return {
        id: p.id,
        type: 'leaf' as const,
        tabs,
        activeTabId: tabs.some(t => t.id === p.activeTabId) ? p.activeTabId : tabs[0]?.id,
      }
    })
    .filter((n): n is PanelNode => n !== null)

  if (leafNodes.length === 0) return null
  if (leafNodes.length === 1) return leafNodes[0]

  // Wrap in a split node using the saved layout direction
  return createSplitNode(v1.layoutDirection, leafNodes)
}

const TabsView: React.FC<TabsViewProps> = ({
  projectId,
  tasks,
  onCreateTask,
  onRefresh,
  initialTaskId,
  initialDesktopId,
  exploratorySessionId,
}) => {
  const snackbar = useSnackbar()
  const updateSpecTask = useUpdateSpecTask()

  // Track if we've initialized from saved state
  const [initialized, setInitialized] = useState(false)

  // Tree-based layout state - rootNode can be a leaf (single panel) or split (nested panels)
  const [rootNode, setRootNode] = useState<PanelNode | null>(null)

  // Touch drag state - store refs to panel elements
  const panelRefsMap = React.useRef<Map<string, HTMLDivElement>>(new Map())
  const [touchDragInfo, setTouchDragInfo] = useState<{ panelId: string; tabId: string } | null>(null)

  // Save workspace state to localStorage whenever rootNode changes
  useEffect(() => {
    if (!projectId || !rootNode) return

    const savedState: SavedWorkspaceState = {
      version: 2,
      projectId,
      rootNode: serializeNode(rootNode),
    }
    localStorage.setItem(WORKSPACE_STATE_KEY, JSON.stringify(savedState))
  }, [rootNode, projectId])

  // Initialize workspace: restore from localStorage or start fresh
  useEffect(() => {
    if (initialized || tasks.length === 0) return

    // Try to restore from localStorage
    const savedJson = localStorage.getItem(WORKSPACE_STATE_KEY)
    let restoredRoot: PanelNode | null = null

    if (savedJson && projectId) {
      try {
        const saved = JSON.parse(savedJson)

        // Only restore if it's for the same project
        if (saved.projectId === projectId) {
          // Check version - migrate v1 or deserialize v2
          if (saved.version === 2 && saved.rootNode) {
            restoredRoot = deserializeNode(saved.rootNode, tasks)
          } else if (saved.panels && saved.panels.length > 0) {
            // Migrate v1 format
            restoredRoot = migrateV1ToV2(saved as SavedWorkspaceStateV1, tasks)
          }
        }
      } catch (e) {
        console.warn('Failed to restore workspace state:', e)
      }
    }

    // Helper to add a tab to the first leaf node
    const addTabToFirstLeaf = (root: PanelNode, tab: TabData): PanelNode => {
      if (root.type === 'leaf') {
        // Check if already open
        if (root.tabs?.some(t => t.id === tab.id)) {
          return { ...root, activeTabId: tab.id }
        }
        return {
          ...root,
          tabs: [...(root.tabs || []), tab],
          activeTabId: tab.id,
        }
      }
      // For split nodes, recurse into first child
      if (root.children && root.children.length > 0) {
        return {
          ...root,
          children: [
            addTabToFirstLeaf(root.children[0], tab),
            ...root.children.slice(1),
          ],
        }
      }
      return root
    }

    // If initialTaskId is provided, ensure it's open
    if (initialTaskId) {
      const taskToOpen = tasks.find(t => t.id === initialTaskId)
      if (taskToOpen) {
        if (restoredRoot) {
          // Add to first leaf if not already open
          const allLeaves = getAllLeafNodes(restoredRoot)
          const alreadyOpen = allLeaves.some(leaf => leaf.tabs?.some(t => t.id === initialTaskId))
          if (!alreadyOpen) {
            restoredRoot = addTabToFirstLeaf(restoredRoot, {
              id: initialTaskId,
              type: 'task',
              task: taskToOpen,
            })
          } else {
            // Just activate it in the panel that has it
            restoredRoot = updateNodeInTree(restoredRoot,
              allLeaves.find(l => l.tabs?.some(t => t.id === initialTaskId))?.id || '',
              node => ({ ...node, activeTabId: initialTaskId })
            )
          }
        } else {
          // Start fresh with this task - create two panels for obvious split-screen
          const otherTasks = tasks.filter(t => t.id !== initialTaskId)
          const secondTask = otherTasks.length > 0 ? otherTasks[0] : null

          const firstLeaf = createLeafNode(
            [{ id: taskToOpen.id!, type: 'task', task: taskToOpen }],
            taskToOpen.id!
          )

          if (secondTask) {
            const secondLeaf = createLeafNode(
              [{ id: secondTask.id!, type: 'task', task: secondTask }],
              secondTask.id!
            )
            restoredRoot = createSplitNode('horizontal', [firstLeaf, secondLeaf])
          } else {
            restoredRoot = firstLeaf
          }
        }
      }
    }

    // If initialDesktopId is provided, ensure the desktop tab is open
    if (initialDesktopId) {
      const desktopTabId = `desktop-${initialDesktopId}`
      const isTeamDesktop = initialDesktopId === exploratorySessionId
      const ownerTask = !isTeamDesktop ? tasks.find(t => t.planning_session_id === initialDesktopId) : null
      const desktopTitle = isTeamDesktop
        ? 'Team Desktop'
        : ownerTask
          ? (ownerTask.user_short_title || ownerTask.short_title || ownerTask.name || 'Task')
          : 'Desktop'

      const desktopTab: TabData = {
        id: desktopTabId,
        type: 'desktop',
        sessionId: initialDesktopId,
        desktopTitle,
      }

      if (restoredRoot) {
        const allLeaves = getAllLeafNodes(restoredRoot)
        const alreadyOpen = allLeaves.some(leaf => leaf.tabs?.some(t => t.id === desktopTabId))
        if (!alreadyOpen) {
          restoredRoot = addTabToFirstLeaf(restoredRoot, desktopTab)
        } else {
          restoredRoot = updateNodeInTree(restoredRoot,
            allLeaves.find(l => l.tabs?.some(t => t.id === desktopTabId))?.id || '',
            node => ({ ...node, activeTabId: desktopTabId })
          )
        }
      } else {
        restoredRoot = createLeafNode([desktopTab], desktopTabId)
      }
    }

    // If nothing restored and no initialTaskId/initialDesktopId, open most recently updated task
    if (!restoredRoot) {
      const sortedTasks = [...tasks].sort((a, b) => {
        const aDate = new Date(a.updated_at || a.created_at || 0).getTime()
        const bDate = new Date(b.updated_at || b.created_at || 0).getTime()
        return bDate - aDate
      })
      const taskToOpen = sortedTasks[0]

      if (taskToOpen?.id) {
        restoredRoot = createLeafNode(
          [{ id: taskToOpen.id, type: 'task', task: taskToOpen }],
          taskToOpen.id
        )
      }
    }

    if (restoredRoot) {
      setRootNode(restoredRoot)
    }
    setInitialized(true)
  }, [tasks, initialized, initialTaskId, initialDesktopId, exploratorySessionId, projectId])

  const handleTabSelect = useCallback((panelId: string, tabId: string) => {
    setRootNode(prev => {
      if (!prev) return prev
      return updateNodeInTree(prev, panelId, node => ({ ...node, activeTabId: tabId }))
    })
  }, [])

  const handleTabClose = useCallback((panelId: string, tabId: string) => {
    setRootNode(prev => {
      if (!prev) return prev

      const panel = findNode(prev, panelId)
      if (!panel || panel.type !== 'leaf') return prev

      const tabs = panel.tabs || []
      const newTabs = tabs.filter(t => t.id !== tabId)

      // If panel has no tabs left and there are other panels, remove this panel
      if (newTabs.length === 0 && countLeafNodes(prev) > 1) {
        return removeNodeFromTree(prev, panelId)
      }

      // Calculate new active tab
      let newActiveTabId = panel.activeTabId
      if (panel.activeTabId === tabId && newTabs.length > 0) {
        const closedIndex = tabs.findIndex(t => t.id === tabId)
        const newActiveIndex = Math.min(closedIndex, newTabs.length - 1)
        newActiveTabId = newTabs[newActiveIndex]?.id || null
      } else if (newTabs.length === 0) {
        newActiveTabId = null
      }

      return updateNodeInTree(prev, panelId, node => ({
        ...node,
        tabs: newTabs,
        activeTabId: newActiveTabId,
      }))
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
    setRootNode(prev => {
      if (!prev) return prev
      return updateNodeInTree(prev, panelId, node => {
        const tabs = node.tabs || []
        // Check if tab already exists
        if (tabs.some(t => t.id === task.id)) {
          return { ...node, activeTabId: task.id }
        }
        return {
          ...node,
          tabs: [...tabs, { id: task.id!, type: 'task', task }],
          activeTabId: task.id,
        }
      })
    })
  }, [])

  // Split a panel into two - this creates proper nested layouts
  // The panel is replaced with a split node containing the original + new panel
  const handleSplitPanel = useCallback((panelId: string, direction: 'horizontal' | 'vertical', taskId?: string) => {
    setRootNode(prev => {
      if (!prev) return prev

      const panel = findNode(prev, panelId)
      if (!panel || panel.type !== 'leaf') return prev

      const tabs = panel.tabs || []
      let tabToMove: TabData | undefined
      let newSourceTabs = tabs

      if (taskId) {
        tabToMove = tabs.find(t => t.id === taskId)
        if (tabToMove) {
          newSourceTabs = tabs.filter(t => t.id !== taskId)
        }
      }

      // Create the new leaf panel
      const newLeaf = createLeafNode(
        tabToMove ? [tabToMove] : [],
        tabToMove?.id || null
      )

      // Update the source panel's tabs
      const updatedSourceLeaf: PanelNode = {
        ...panel,
        tabs: newSourceTabs,
        activeTabId: newSourceTabs.length > 0
          ? (newSourceTabs.some(t => t.id === panel.activeTabId)
              ? panel.activeTabId
              : newSourceTabs[0].id)
          : null,
      }

      // Create a new split node containing both panels
      const newSplit = createSplitNode(direction, [updatedSourceLeaf, newLeaf])

      // Replace the original panel with the new split
      return replaceNodeInTree(prev, panelId, newSplit)
    })
  }, [])

  const handleDropTab = useCallback((targetPanelId: string, tabId: string, fromPanelId: string) => {
    setRootNode(prev => {
      if (!prev) return prev

      const sourcePanel = findNode(prev, fromPanelId)
      const targetPanel = findNode(prev, targetPanelId)
      if (!sourcePanel || !targetPanel) return prev
      if (sourcePanel.type !== 'leaf' || targetPanel.type !== 'leaf') return prev

      const sourceTabs = sourcePanel.tabs || []
      const targetTabs = targetPanel.tabs || []

      const tabToMove = sourceTabs.find(t => t.id === tabId)
      if (!tabToMove) return prev

      // Check if already in target
      if (targetTabs.some(t => t.id === tabId)) return prev

      // First, update source panel (remove tab)
      const newSourceTabs = sourceTabs.filter(t => t.id !== tabId)
      let updated = updateNodeInTree(prev, fromPanelId, node => ({
        ...node,
        tabs: newSourceTabs,
        activeTabId: newSourceTabs.length > 0
          ? (newSourceTabs.some(t => t.id === node.activeTabId) ? node.activeTabId : newSourceTabs[0].id)
          : null,
      }))

      // Then, update target panel (add tab)
      updated = updateNodeInTree(updated, targetPanelId, node => ({
        ...node,
        tabs: [...(node.tabs || []), tabToMove],
        activeTabId: tabId,
      }))

      // If source panel is now empty and there are other panels, remove it
      if (newSourceTabs.length === 0 && countLeafNodes(updated) > 1) {
        updated = removeNodeFromTree(updated, fromPanelId) || updated
      }

      return updated
    })
  }, [])

  const handleClosePanel = useCallback((panelId: string) => {
    setRootNode(prev => {
      if (!prev) return prev
      // Don't remove the last panel
      if (countLeafNodes(prev) <= 1) return prev
      return removeNodeFromTree(prev, panelId)
    })
  }, [])

  // Handle task created - add it to the specified panel
  const handleTaskCreated = useCallback((panelId: string, task: TypesSpecTask) => {
    if (!task.id) return
    setRootNode(prev => {
      if (!prev) return prev
      return updateNodeInTree(prev, panelId, node => ({
        ...node,
        tabs: [...(node.tabs || []), { id: task.id!, type: 'task', task }],
        activeTabId: task.id!,
      }))
    })
  }, [])

  // Handle adding a Team Desktop tab to a panel
  const handleAddDesktop = useCallback((panelId: string, sessionId: string, title?: string) => {
    const desktopTabId = `desktop-${sessionId}`
    setRootNode(prev => {
      if (!prev) return prev
      return updateNodeInTree(prev, panelId, node => {
        const tabs = node.tabs || []
        // Check if already open
        if (tabs.some(t => t.id === desktopTabId)) {
          return { ...node, activeTabId: desktopTabId }
        }
        // Add new desktop tab
        return {
          ...node,
          tabs: [...tabs, {
            id: desktopTabId,
            type: 'desktop' as const,
            sessionId,
            desktopTitle: title || 'Team Desktop',
          }],
          activeTabId: desktopTabId,
        }
      })
    })
  }, [])

  // Handle adding a "Create New Task" tab to a panel
  const handleAddCreateTab = useCallback((panelId: string) => {
    const createTabId = `create-${Date.now()}`
    setRootNode(prev => {
      if (!prev) return prev
      return updateNodeInTree(prev, panelId, node => {
        const tabs = node.tabs || []
        // Check if a create tab already exists in this panel
        const existingCreate = tabs.find(t => t.type === 'create')
        if (existingCreate) {
          return { ...node, activeTabId: existingCreate.id }
        }
        // Add new create tab
        return {
          ...node,
          tabs: [...tabs, {
            id: createTabId,
            type: 'create' as const,
          }],
          activeTabId: createTabId,
        }
      })
    })
  }, [])

  // Handle opening a review - creates a vertical split with review on the right
  // IMPORTANT: Use :: as delimiter since task/review IDs are UUIDs containing hyphens
  const handleOpenReview = useCallback((taskId: string, reviewId: string, reviewTitle?: string, sourcePanelId?: string) => {
    const tabId = `review::${taskId}::${reviewId}`

    setRootNode(prev => {
      if (!prev) return prev

      // Check if this review is already open in any leaf
      const allLeaves = getAllLeafNodes(prev)
      for (const leaf of allLeaves) {
        if (leaf.tabs?.some(t => t.id === tabId)) {
          // Activate it
          return updateNodeInTree(prev, leaf.id, node => ({ ...node, activeTabId: tabId }))
        }
      }

      // Create a new leaf node with the review tab
      const newReviewLeaf = createLeafNode(
        [{
          id: tabId,
          type: 'review' as const,
          taskId,
          reviewId,
          reviewTitle: reviewTitle || 'Spec Review',
        }],
        tabId
      )

      // If we have a source panel, create a vertical split with it
      if (sourcePanelId) {
        const sourcePanel = findNode(prev, sourcePanelId)
        if (sourcePanel && sourcePanel.type === 'leaf') {
          // Create a split node with source panel on left, review on right
          const newSplit = createSplitNode('vertical', [sourcePanel, newReviewLeaf])
          // Replace the source panel with the new split
          return replaceNodeInTree(prev, sourcePanelId, newSplit)
        }
      }

      // Fallback: if no source panel or it's not a leaf, add to first leaf
      if (allLeaves.length > 0) {
        // Create a vertical split with the first leaf panel
        const firstLeaf = allLeaves[0]
        const newSplit = createSplitNode('vertical', [firstLeaf, newReviewLeaf])
        return replaceNodeInTree(prev, firstLeaf.id, newSplit)
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

  // Calculate total leaf count for panelCount prop
  const totalPanelCount = countLeafNodes(rootNode)

  // Recursive renderer for the tree structure
  const renderPanelNode = (node: PanelNode): React.ReactNode => {
    if (node.type === 'leaf') {
      // Convert PanelNode to PanelData for TaskPanel component
      const panelData: PanelData = {
        id: node.id,
        tabs: node.tabs || [],
        activeTabId: node.activeTabId || null,
      }
      return (
        <TaskPanel
          panel={panelData}
          tasks={tasks}
          projectId={projectId}
          exploratorySessionId={exploratorySessionId}
          onTabSelect={handleTabSelect}
          onTabClose={handleTabClose}
          onTabRename={handleTabRename}
          onAddTab={handleAddTab}
          onAddDesktop={handleAddDesktop}
          onAddCreateTab={handleAddCreateTab}
          onTaskCreated={handleTaskCreated}
          onSplitPanel={handleSplitPanel}
          onDropTab={handleDropTab}
          onClosePanel={handleClosePanel}
          onOpenReview={handleOpenReview}
          onTouchDragStart={handleTouchDragStart}
          onTouchDragEnd={handleTouchDragEnd}
          panelCount={totalPanelCount}
          panelRef={getPanelRef(node.id)}
        />
      )
    }

    // Split node - render nested PanelGroup
    const children = node.children || []
    return (
      <PanelGroup orientation={node.direction || 'horizontal'} style={{ height: '100%' }}>
        {children.map((child, index) => (
          <React.Fragment key={child.id}>
            {index > 0 && <ResizeHandle direction={node.direction || 'horizontal'} />}
            <Panel defaultSize={100 / children.length} minSize={15}>
              {renderPanelNode(child)}
            </Panel>
          </React.Fragment>
        ))}
      </PanelGroup>
    )
  }

  // When no rootNode exists, show an empty panel with just a + button
  if (!rootNode) {
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
      {renderPanelNode(rootNode)}
    </Box>
  )
}

export default TabsView
