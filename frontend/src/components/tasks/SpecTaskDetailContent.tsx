import React, { FC, useState, useEffect, useCallback, useMemo, useRef } from 'react'
import {
  Box,
  Typography,
  Chip,
  Divider,
  IconButton,
  TextField,
  Button,
  Checkbox,
  FormControlLabel,
  Tooltip,
  Select,
  FormControl,
  InputLabel,
  CircularProgress,
  MenuItem,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogContentText,
  DialogActions,
  ToggleButton,
  ToggleButtonGroup,
} from '@mui/material'
import CloseIcon from '@mui/icons-material/Close'
import EditIcon from '@mui/icons-material/Edit'
import PlayArrow from '@mui/icons-material/PlayArrow'
import Description from '@mui/icons-material/Description'
import SaveIcon from '@mui/icons-material/Save'
import CancelIcon from '@mui/icons-material/Cancel'
import RestartAltIcon from '@mui/icons-material/RestartAlt'
import LaunchIcon from '@mui/icons-material/Launch'
import ForumOutlinedIcon from '@mui/icons-material/ForumOutlined'
import ComputerIcon from '@mui/icons-material/Computer'
import TuneIcon from '@mui/icons-material/Tune'
import DifferenceIcon from '@mui/icons-material/Difference'
import CloudUploadIcon from '@mui/icons-material/CloudUpload'
import VerticalSplitIcon from '@mui/icons-material/VerticalSplit'
import ChevronLeftIcon from '@mui/icons-material/ChevronLeft'
import ContentCopyIcon from '@mui/icons-material/ContentCopy'
import LinkIcon from '@mui/icons-material/Link'
import { TypesSpecTask, TypesSpecTaskPriority, TypesSpecTaskStatus } from '../../api/api'
import ExternalAgentDesktopViewer from '../external-agent/ExternalAgentDesktopViewer'
import DesignDocViewer from './DesignDocViewer'
import DiffViewer from './DiffViewer'
import useSnackbar from '../../hooks/useSnackbar'
import useAccount from '../../hooks/useAccount'
import useApi from '../../hooks/useApi'
import { getBrowserLocale } from '../../hooks/useBrowserLocale'
import useApps from '../../hooks/useApps'
import { useStreaming } from '../../contexts/streaming'
import { useQueryClient } from '@tanstack/react-query'
import { useGetSession, GET_SESSION_QUERY_KEY } from '../../services/sessionService'
import { SESSION_TYPE_TEXT, AGENT_TYPE_ZED_EXTERNAL } from '../../types'
import { useUpdateSpecTask, useSpecTask, useCloneGroups } from '../../services/specTaskService'
import CloneTaskDialog from '../specTask/CloneTaskDialog'
import CloneGroupProgressFull from '../specTask/CloneGroupProgress'
import RobustPromptInput from '../common/RobustPromptInput'
import EmbeddedSessionView, { EmbeddedSessionViewHandle } from '../session/EmbeddedSessionView'
import { Panel, Group as PanelGroup, Separator as PanelResizeHandle } from 'react-resizable-panels'
import useIsBigScreen from '../../hooks/useIsBigScreen'

interface SpecTaskDetailContentProps {
  taskId: string
  onClose?: () => void
  /** Called when user clicks "Review Spec" - if provided, opens in workspace pane instead of navigating */
  onOpenReview?: (taskId: string, reviewId: string, reviewTitle?: string) => void
}

const SpecTaskDetailContent: FC<SpecTaskDetailContentProps> = ({
  taskId,
  onClose,
  onOpenReview,
}) => {
  const api = useApi()
  const snackbar = useSnackbar()
  const account = useAccount()
  const streaming = useStreaming()
  const apps = useApps()
  const updateSpecTask = useUpdateSpecTask()
  const queryClient = useQueryClient()
  // Use md breakpoint (900px) to enable split view on tablets
  const isBigScreen = useIsBigScreen({ breakpoint: 'md' })

  // Fetch task data
  const { data: task } = useSpecTask(taskId, {
    enabled: !!taskId,
    refetchInterval: 2300, // 2.3s - prime to avoid sync with other polling
  })

  // Edit mode state
  const [isEditMode, setIsEditMode] = useState(false)
  const [editFormData, setEditFormData] = useState({
    name: '',
    description: '',
    priority: '',
  })

  // Agent selection state
  const [selectedAgent, setSelectedAgent] = useState('')
  const [updatingAgent, setUpdatingAgent] = useState(false)

  // Start planning state - prevents double-click
  const [isStartingPlanning, setIsStartingPlanning] = useState(false)

  // Chat panel collapse state - when true, uses mobile-style tab layout even on desktop
  const [chatCollapsed, setChatCollapsed] = useState(false)

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
      return { width: 1920, height: 1080, fps: 60 }
    }
    const taskApp = apps.apps.find(a => a.id === task.helix_app_id)
    const config = taskApp?.config?.helix?.external_agent_config
    if (!config) {
      return { width: 1920, height: 1080, fps: 60 }
    }

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
    apps.loadApps()
  }, [])

  // On mobile, 'chat' is a separate tab; on desktop, chat is always visible
  const [currentView, setCurrentView] = useState<'chat' | 'desktop' | 'changes' | 'details'>('desktop')
  const [clientUniqueId, setClientUniqueId] = useState<string>('')

  // Ref for EmbeddedSessionView to trigger scroll on height changes
  const sessionViewRef = useRef<EmbeddedSessionViewHandle>(null)

  // Design review state
  const [docViewerOpen, setDocViewerOpen] = useState(false)
  const [implementationReviewMessageSent, setImplementationReviewMessageSent] = useState(false)

  // Session restart state
  const [restartConfirmOpen, setRestartConfirmOpen] = useState(false)
  const [isRestarting, setIsRestarting] = useState(false)


  // Just Do It mode state
  const [justDoItMode, setJustDoItMode] = useState(false)
  const [updatingJustDoIt, setUpdatingJustDoIt] = useState(false)

  // File upload state
  const fileInputRef = useRef<HTMLInputElement>(null)
  const [isUploading, setIsUploading] = useState(false)

  // Clone dialog state
  const [showCloneDialog, setShowCloneDialog] = useState(false)
  const [selectedCloneGroupId, setSelectedCloneGroupId] = useState<string | null>(null)

  // Fetch clone groups where this task was the source
  const { data: cloneGroups } = useCloneGroups(taskId)

  // Initialize edit form data when task changes
  useEffect(() => {
    if (task && isEditMode) {
      setEditFormData({
        name: task.name || '',
        description: task.description || task.original_prompt || '',
        priority: task.priority || 'medium',
      })
    }
  }, [task, isEditMode])

  // Get the active session ID
  const activeSessionId = task?.planning_session_id

  // Default to appropriate view based on session state and screen size
  useEffect(() => {
    if (activeSessionId && currentView === 'details') {
      // If there's an active session and we're on details, switch to appropriate view
      // On mobile, default to chat; on desktop, default to desktop (chat is always visible)
      setCurrentView(isBigScreen ? 'desktop' : 'chat')
    } else if (!activeSessionId && currentView !== 'details') {
      // If no active session, switch to details view
      setCurrentView('details')
    }
  }, [activeSessionId, isBigScreen])

  // Fetch session data
  const { data: sessionResponse } = useGetSession(activeSessionId || '', { enabled: !!activeSessionId })
  const sessionData = sessionResponse?.data


  // Sync justDoItMode when task changes
  useEffect(() => {
    if (task?.just_do_it_mode !== undefined) {
      setJustDoItMode(task.just_do_it_mode)
    }
  }, [task?.just_do_it_mode])

  const getPriorityColor = (priority: string) => {
    switch (priority?.toLowerCase()) {
      case 'critical': return 'error'
      case 'high': return 'warning'
      case 'medium': return 'info'
      case 'low': return 'success'
      default: return 'default'
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

  const handleStartPlanning = async () => {
    if (!task?.id || isStartingPlanning) return

    setIsStartingPlanning(true)
    try {
      const { keyboardLayout, timezone, isOverridden } = getBrowserLocale()
      const queryParams = new URLSearchParams()
      if (keyboardLayout) queryParams.set('keyboard', keyboardLayout)
      if (timezone) queryParams.set('timezone', timezone)
      const queryString = queryParams.toString()
      const url = `/api/v1/spec-tasks/${task.id}/start-planning${queryString ? `?${queryString}` : ''}`

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
      setCurrentView('session')
    } catch (err: any) {
      console.error('Failed to start planning:', err)
      snackbar.error(err?.message || 'Failed to start planning. Please try again.')
    } finally {
      setIsStartingPlanning(false)
    }
  }

  // Handle session restart
  const handleRestartSession = useCallback(async () => {
    if (!activeSessionId || isRestarting) return

    setIsRestarting(true)
    setRestartConfirmOpen(false)

    try {
      snackbar.info('Stopping agent session...')
      await api.getApiClient().v1SessionsStopExternalAgentDelete(activeSessionId)
      await new Promise(resolve => setTimeout(resolve, 1000))
      snackbar.info('Starting new agent session...')
      await api.getApiClient().v1SessionsResumeCreate(activeSessionId)
      queryClient.invalidateQueries({ queryKey: GET_SESSION_QUERY_KEY(activeSessionId) })
      snackbar.success('Session restarted successfully')
    } catch (err: any) {
      console.error('Failed to restart session:', err)
      snackbar.error(err?.message || 'Failed to restart session')
    } finally {
      setIsRestarting(false)
    }
  }, [activeSessionId, isRestarting, api, snackbar])

  // Toggle Just Do It mode
  const handleToggleJustDoIt = useCallback(async () => {
    if (!task?.id || updatingJustDoIt) return

    const newValue = !justDoItMode
    setUpdatingJustDoIt(true)

    try {
      await updateSpecTask.mutateAsync({
        taskId: task.id,
        updates: { just_do_it_mode: newValue },
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

  // Handle agent change
  const handleAgentChange = useCallback(async (newAgentId: string) => {
    if (!task?.id || updatingAgent || newAgentId === selectedAgent) return

    setUpdatingAgent(true)
    const previousAgent = selectedAgent
    setSelectedAgent(newAgentId)

    try {
      await updateSpecTask.mutateAsync({
        taskId: task.id,
        updates: { helix_app_id: newAgentId },
      })
      snackbar.success('Agent updated')
    } catch (err) {
      console.error('Failed to update agent:', err)
      snackbar.error('Failed to update agent')
      setSelectedAgent(previousAgent)
    } finally {
      setUpdatingAgent(false)
    }
  }, [task?.id, selectedAgent, updatingAgent, updateSpecTask, snackbar])

  // Handle edit mode
  const handleEditToggle = useCallback(() => {
    setIsEditMode(true)
  }, [])

  const handleCancelEdit = useCallback(() => {
    setIsEditMode(false)
    if (task) {
      setEditFormData({
        name: task.name || '',
        description: task.description || task.original_prompt || '',
        priority: task.priority || 'medium',
      })
    }
  }, [task])

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

  // Handle review spec navigation
  const handleReviewSpec = useCallback(async () => {
    if (!task?.id) return

    try {
      const response = await api.getApiClient().v1SpecTasksDesignReviewsDetail(task.id)
      const reviews = response.data?.reviews || []
      if (reviews.length > 0) {
        const latestReview = reviews.find((r: any) => r.status !== 'superseded') || reviews[0]
        if (onOpenReview) {
          onOpenReview(task.id, latestReview.id, task.name || 'Spec Review')
        } else {
          account.orgNavigate('project-task-review', {
            id: task.project_id,
            taskId: task.id,
            reviewId: latestReview.id,
          })
        }
      } else {
        snackbar.error('No design review found')
      }
    } catch (error) {
      console.error('Failed to fetch design reviews:', error)
      snackbar.error('Failed to load design review')
    }
  }, [task?.id, task?.name, task?.project_id, onOpenReview, account])

  // Handle file upload to sandbox
  const handleUploadClick = useCallback(() => {
    if (!activeSessionId) {
      snackbar.error('Please start the task first before uploading files')
      return
    }
    fileInputRef.current?.click()
  }, [activeSessionId, snackbar])

  const handleFileChange = useCallback(async (e: React.ChangeEvent<HTMLInputElement>) => {
    const files = e.target.files
    if (!files || files.length === 0 || !activeSessionId) return

    setIsUploading(true)
    let successCount = 0
    let errorCount = 0

    for (const file of Array.from(files)) {
      try {
        const formData = new FormData()
        formData.append('file', file)

        const response = await fetch(`/api/v1/external-agents/${activeSessionId}/upload?open_file_manager=false`, {
          method: 'POST',
          body: formData,
        })

        if (response.ok) {
          successCount++
        } else {
          errorCount++
        }
      } catch (error) {
        console.error(`Failed to upload ${file.name}:`, error)
        errorCount++
      }
    }

    setIsUploading(false)

    if (successCount > 0 && errorCount === 0) {
      snackbar.success(`Uploaded ${successCount} file${successCount > 1 ? 's' : ''} to ~/work/incoming`)
    } else if (successCount > 0 && errorCount > 0) {
      snackbar.warning(`Uploaded ${successCount}, ${errorCount} failed`)
    } else if (errorCount > 0) {
      snackbar.error(`Failed to upload ${errorCount} file${errorCount > 1 ? 's' : ''}`)
    }

    // Clear the input so the same file can be uploaded again
    if (fileInputRef.current) {
      fileInputRef.current.value = ''
    }
  }, [activeSessionId, snackbar])

  // Render the details content (used in both desktop left panel and mobile/no-session view)
  const renderDetailsContent = () => (
    <>
      {/* Description */}
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
            {task?.description || task?.original_prompt || 'No description provided'}
          </Typography>
        )}
      </Box>

      <Divider sx={{ my: 2 }} />

      {/* Priority */}
      <Box sx={{ mb: 2 }}>
        {isEditMode ? (
          <FormControl fullWidth size="small">
            <InputLabel>Priority</InputLabel>
            <Select
              value={editFormData.priority}
              onChange={(e) => setEditFormData(prev => ({ ...prev, priority: e.target.value }))}
              label="Priority"
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
            <Chip label={task?.priority || 'Medium'} color={getPriorityColor(task?.priority)} size="small" />
          </>
        )}
      </Box>

      {/* Agent Selection */}
      <Box sx={{ mb: 2 }}>
        <FormControl fullWidth size="small">
          <InputLabel>Agent</InputLabel>
          <Select
            value={selectedAgent}
            onChange={(e) => handleAgentChange(e.target.value)}
            label="Agent"
            disabled={updatingAgent}
            endAdornment={updatingAgent ? <CircularProgress size={16} sx={{ mr: 2 }} /> : null}
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
          Created: {task?.created_at ? new Date(task.created_at).toLocaleString() : 'N/A'}
        </Typography>
        <Typography variant="caption" color="text.secondary" display="block">
          Updated: {task?.updated_at ? new Date(task.updated_at).toLocaleString() : 'N/A'}
        </Typography>
      </Box>

      {/* Clone Info - Bidirectional links */}
      {(task?.cloned_from_id || (cloneGroups && cloneGroups.length > 0)) && (
        <Box sx={{ mt: 2 }}>
          <Typography variant="subtitle2" color="text.secondary" gutterBottom>
            Clone Info
          </Typography>

          {/* Link to source task if this was cloned */}
          {task?.cloned_from_id && (
            <Box
              sx={{
                display: 'flex',
                alignItems: 'center',
                gap: 1,
                p: 1,
                bgcolor: 'action.hover',
                borderRadius: 1,
                cursor: 'pointer',
                mb: 1,
                '&:hover': { bgcolor: 'action.selected' },
              }}
              onClick={() => {
                if (task.cloned_from_project_id && task.cloned_from_id) {
                  account.orgNavigate('project-task-detail', {
                    id: task.cloned_from_project_id,
                    taskId: task.cloned_from_id,
                  })
                }
              }}
            >
              <LinkIcon sx={{ fontSize: 16, color: 'text.secondary' }} />
              <Typography variant="caption" color="text.secondary">
                Cloned from another task
              </Typography>
            </Box>
          )}

          {/* Links to clone groups if this task was cloned to others */}
          {cloneGroups && cloneGroups.length > 0 && (
            <Box>
              <Typography variant="caption" color="text.secondary" display="block" sx={{ mb: 0.5 }}>
                Cloned to {cloneGroups.length} batch{cloneGroups.length > 1 ? 'es' : ''}:
              </Typography>
              {cloneGroups.map((group) => (
                <Box
                  key={group.id}
                  sx={{
                    display: 'flex',
                    alignItems: 'center',
                    gap: 1,
                    p: 1,
                    bgcolor: 'action.hover',
                    borderRadius: 1,
                    cursor: 'pointer',
                    mb: 0.5,
                    '&:hover': { bgcolor: 'action.selected' },
                  }}
                  onClick={() => setSelectedCloneGroupId(group.id || null)}
                >
                  <ContentCopyIcon sx={{ fontSize: 16, color: 'text.secondary' }} />
                  <Typography variant="caption">
                    {group.total_targets} project{group.total_targets !== 1 ? 's' : ''} • {new Date(group.created_at || '').toLocaleDateString()}
                  </Typography>
                </Box>
              ))}
            </Box>
          )}
        </Box>
      )}

      {/* Debug Info */}
      <Divider sx={{ my: 2 }} />
      <Box sx={{ mt: 2, p: 2, bgcolor: 'grey.900', borderRadius: 1 }}>
        <Typography variant="caption" color="grey.400" display="block" gutterBottom>
          Debug Information
        </Typography>
        <Typography variant="caption" color="grey.300" sx={{ fontFamily: 'monospace', display: 'block' }}>
          Task ID: {task?.id || 'N/A'}
        </Typography>
        {task?.branch_name && (
          <Tooltip title="Spectask branches push changes to upstream repository">
            <Typography variant="caption" color="grey.300" sx={{ fontFamily: 'monospace', display: 'block' }}>
              Branch: {task.branch_name} <Box component="span" sx={{ color: 'success.main' }}>→ PUSH</Box>
            </Typography>
          </Tooltip>
        )}
        {task?.base_branch && task.base_branch !== task.branch_name && (
          <Tooltip title="Base branch pulls updates from upstream repository">
            <Typography variant="caption" color="grey.300" sx={{ fontFamily: 'monospace', display: 'block' }}>
              Base: {task.base_branch} <Box component="span" sx={{ color: 'info.main' }}>← PULL</Box>
            </Typography>
          </Tooltip>
        )}
        {activeSessionId && (
          <Typography variant="caption" color="grey.300" sx={{ fontFamily: 'monospace', display: 'block' }}>
            Session ID: {activeSessionId}
          </Typography>
        )}
        {sessionData?.config?.sway_version && (
          <Typography variant="caption" color="grey.300" sx={{ fontFamily: 'monospace', display: 'block' }}>
            Desktop: {sessionData.config.sway_version}
          </Typography>
        )}
        {sessionData?.config?.gpu_vendor && (
          <Typography variant="caption" color="grey.300" sx={{ fontFamily: 'monospace', display: 'block' }}>
            GPU: {sessionData.config.gpu_vendor.toUpperCase()}
          </Typography>
        )}
        {sessionData?.config?.render_node && (
          <Typography variant="caption" color="grey.300" sx={{ fontFamily: 'monospace', display: 'block' }}>
            Render: {sessionData.config.render_node}
          </Typography>
        )}
      </Box>
    </>
  )

  if (!task) {
    return (
      <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '100%' }}>
        <CircularProgress />
      </Box>
    )
  }

  return (
    <Box sx={{ display: 'flex', flexDirection: 'column', height: '100%', overflow: 'hidden' }}>
      {/* Hidden file input for upload */}
      <input
        type="file"
        ref={fileInputRef}
        onChange={handleFileChange}
        style={{ display: 'none' }}
        multiple
      />

      {/* Tab Content */}
      <Box sx={{ flex: 1, overflow: 'hidden', display: 'flex', flexDirection: 'column' }}>
        {/* Desktop layout: left panel (chat always visible) + right panel (content toggleable) */}
        {/* When chatCollapsed is true, use mobile-style tab layout even on desktop */}
        {activeSessionId && isBigScreen && !chatCollapsed ? (
          <PanelGroup direction="horizontal" style={{ height: '100%', flex: 1 }}>
            {/* Left: Chat panel - always visible on desktop */}
            <Panel defaultSize={30} minSize={15} style={{ overflow: 'hidden' }}>
              <Box sx={{ height: '100%', display: 'flex', flexDirection: 'column', minHeight: 0 }}>
                {/* Chat panel header with collapse button */}
                <Box
                  sx={{
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'space-between',
                    px: 1.5,
                    py: 0.75,
                    minHeight: 40,
                    borderBottom: '1px solid',
                    borderColor: 'divider',
                    backgroundColor: 'background.paper',
                    flexShrink: 0,
                  }}
                >
                  <Typography variant="body2" sx={{ fontWeight: 500, color: 'text.secondary' }}>
                    Chat
                  </Typography>
                  <Tooltip title="Collapse chat panel">
                    <IconButton
                      size="small"
                      onClick={() => {
                        setChatCollapsed(true)
                        // Switch to desktop view when collapsing chat
                        if (currentView === 'chat') {
                          setCurrentView('desktop')
                        }
                      }}
                      sx={{ p: 0.25 }}
                    >
                      <ChevronLeftIcon sx={{ fontSize: 18 }} />
                    </IconButton>
                  </Tooltip>
                </Box>
                <EmbeddedSessionView ref={sessionViewRef} sessionId={activeSessionId} />
                <Box sx={{ p: 1.5, flexShrink: 0 }}>
                  <RobustPromptInput
                    sessionId={activeSessionId}
                    specTaskId={task.id}
                    projectId={task.project_id}
                    apiClient={api.getApiClient()}
                    onSend={async (message: string, interrupt?: boolean) => {
                      await streaming.NewInference({
                        type: SESSION_TYPE_TEXT,
                        message,
                        sessionId: activeSessionId,
                        interrupt: interrupt ?? true,
                      })
                    }}
                    onHeightChange={() => sessionViewRef.current?.scrollToBottom()}
                    placeholder="Send message to agent..."
                  />
                </Box>
              </Box>
            </Panel>

            {/* Resize handle */}
            <PanelResizeHandle style={{
              width: 6,
              background: 'rgba(255, 255, 255, 0.08)',
              cursor: 'col-resize',
              transition: 'background 0.15s',
            }}>
              <div style={{
                width: 2,
                height: '100%',
                margin: '0 auto',
                background: 'rgba(255, 255, 255, 0.12)',
                borderRadius: 1,
              }} />
            </PanelResizeHandle>

            {/* Right: Content panel - switches between desktop/changes/details */}
            <Panel defaultSize={70} minSize={25} style={{ overflow: 'hidden' }}>
              <Box sx={{ height: '100%', display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
                {/* View toggle header - above content area only */}
                <Box
                  sx={{
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'space-between',
                    px: 1.5,
                    py: 0.75,
                    minHeight: 40,
                    borderBottom: '1px solid',
                    borderColor: 'divider',
                    backgroundColor: 'background.paper',
                    gap: 1,
                  }}
                >
                  {/* Left: View toggle icons */}
                  <ToggleButtonGroup
                    value={currentView}
                    exclusive
                    onChange={(_, newView) => newView && setCurrentView(newView)}
                    size="small"
                    sx={{
                      '& .MuiToggleButton-root': {
                        py: 0.25,
                        px: 1,
                        border: 'none',
                        borderRadius: '4px !important',
                        '&.Mui-selected': {
                          backgroundColor: 'action.selected',
                        },
                      },
                    }}
                  >
                    <ToggleButton value="desktop" aria-label="Desktop view">
                      <Tooltip title="Desktop">
                        <ComputerIcon sx={{ fontSize: 18 }} />
                      </Tooltip>
                    </ToggleButton>
                    <ToggleButton value="changes" aria-label="Changes view">
                      <Tooltip title="Changes">
                        <DifferenceIcon sx={{ fontSize: 18 }} />
                      </Tooltip>
                    </ToggleButton>
                    <ToggleButton value="details" aria-label="Details view">
                      <Tooltip title="Details">
                        <TuneIcon sx={{ fontSize: 18 }} />
                      </Tooltip>
                    </ToggleButton>
                  </ToggleButtonGroup>

                  {/* Status-specific action buttons */}
                  {task.status === 'backlog' && (
                    <Button
                      variant="contained"
                      color={justDoItMode ? 'success' : 'warning'}
                      size="small"
                      startIcon={isStartingPlanning ? <CircularProgress size={16} color="inherit" /> : <PlayArrow />}
                      onClick={handleStartPlanning}
                      disabled={isStartingPlanning}
                      sx={{ ml: 1, fontSize: '0.75rem' }}
                    >
                      {isStartingPlanning ? 'Starting...' : (justDoItMode ? 'Just Do It' : 'Start Planning')}
                    </Button>
                  )}
                  {task.status === 'spec_review' && (
                    <Button
                      variant="contained"
                      color="info"
                      size="small"
                      startIcon={<Description />}
                      onClick={handleReviewSpec}
                      sx={{
                        ml: 1,
                        fontSize: '0.75rem',
                        animation: 'pulse-glow 2s infinite',
                        '@keyframes pulse-glow': {
                          '0%, 100%': { boxShadow: '0 0 5px rgba(41, 182, 246, 0.5)' },
                          '50%': { boxShadow: '0 0 15px rgba(41, 182, 246, 0.8)' },
                        },
                      }}
                    >
                      Review Spec
                    </Button>
                  )}
                  {task.pull_request_url && (
                    <Button
                      variant="outlined"
                      color="secondary"
                      size="small"
                      startIcon={<LaunchIcon />}
                      onClick={() => window.open(task.pull_request_url, '_blank')}
                      sx={{ ml: 1, fontSize: '0.75rem' }}
                    >
                      View PR
                    </Button>
                  )}
                  {/* View Spec button - show when spec exists */}
                  {task.design_docs_pushed_at && task.status !== 'spec_review' && (
                    <Button
                      variant="outlined"
                      size="small"
                      startIcon={<Description />}
                      onClick={handleReviewSpec}
                      sx={{ ml: 1, fontSize: '0.75rem' }}
                    >
                      View Spec
                    </Button>
                  )}

                  {/* Spacer */}
                  <Box sx={{ flex: 1 }} />

                  {/* Right: Action buttons */}
                  <Box sx={{ display: 'flex', gap: 0.5, alignItems: 'center' }}>
                    {isEditMode ? (
                      <>
                        <Button size="small" startIcon={<CancelIcon />} onClick={handleCancelEdit} sx={{ fontSize: '0.75rem' }}>
                          Cancel
                        </Button>
                        <Button
                          size="small"
                          color="secondary"
                          startIcon={<SaveIcon />}
                          onClick={handleSaveEdit}
                          disabled={updateSpecTask.isPending}
                          sx={{ fontSize: '0.75rem' }}
                        >
                          Save
                        </Button>
                      </>
                    ) : (
                      <>
                        {task.status === TypesSpecTaskStatus.TaskStatusBacklog && (
                          <Tooltip title="Edit task">
                            <IconButton size="small" onClick={handleEditToggle}>
                              <EditIcon sx={{ fontSize: 18 }} />
                            </IconButton>
                          </Tooltip>
                        )}
                        {task.design_docs_pushed_at && (
                          <Tooltip title="Clone task to other projects">
                            <IconButton size="small" onClick={() => setShowCloneDialog(true)}>
                              <ContentCopyIcon sx={{ fontSize: 18 }} />
                            </IconButton>
                          </Tooltip>
                        )}
                        <Tooltip title="Restart agent session">
                          <IconButton
                            size="small"
                            onClick={() => setRestartConfirmOpen(true)}
                            disabled={isRestarting}
                            color="warning"
                          >
                            {isRestarting ? <CircularProgress size={16} /> : <RestartAltIcon sx={{ fontSize: 18 }} />}
                          </IconButton>
                        </Tooltip>
                        <Tooltip title="Upload files to sandbox">
                          <IconButton
                            size="small"
                            onClick={handleUploadClick}
                            disabled={isUploading}
                            color="primary"
                          >
                            {isUploading ? <CircularProgress size={16} /> : <CloudUploadIcon sx={{ fontSize: 18 }} />}
                          </IconButton>
                        </Tooltip>
                      </>
                    )}
                    {onClose && (
                      <IconButton size="small" onClick={onClose}>
                        <CloseIcon sx={{ fontSize: 18 }} />
                      </IconButton>
                    )}
                  </Box>
                </Box>

                {currentView === 'desktop' && (
                  <ExternalAgentDesktopViewer
                    sessionId={activeSessionId}
                    sandboxId={activeSessionId}
                    mode="stream"
                    onClientIdCalculated={setClientUniqueId}
                    displayWidth={displaySettings.width}
                    displayHeight={displaySettings.height}
                    displayFps={displaySettings.fps}
                  />
                )}
                {currentView === 'changes' && (
                  <DiffViewer
                    sessionId={activeSessionId}
                    baseBranch="main"
                    pollInterval={3000}
                  />
                )}
                {currentView === 'details' && (
                  <Box sx={{ flex: 1, overflow: 'auto', p: 3 }}>
                    {renderDetailsContent()}
                  </Box>
                )}
              </Box>
            </Panel>

          </PanelGroup>
        ) : (
          <>
            {/* Mobile layout OR no active session: single view at a time */}
            {/* View toggle header for mobile/no-session */}
            <Box
              sx={{
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'space-between',
                flexWrap: 'wrap',
                px: 1,
                py: 0.5,
                borderBottom: '1px solid',
                borderColor: 'divider',
                backgroundColor: 'background.paper',
                gap: 0.5,
                minHeight: 'auto',
              }}
            >
              {/* Left: View toggle icons */}
              <ToggleButtonGroup
                value={currentView}
                exclusive
                onChange={(_, newView) => newView && setCurrentView(newView)}
                size="small"
                sx={{
                  flexShrink: 0,
                  '& .MuiToggleButton-root': {
                    py: 0.25,
                    px: 0.75,
                    minWidth: 32,
                    border: 'none',
                    borderRadius: '4px !important',
                    '&.Mui-selected': {
                      backgroundColor: 'action.selected',
                    },
                  },
                }}
              >
                {/* Chat tab - only on mobile when there's an active session */}
                {activeSessionId && (
                  <ToggleButton value="chat" aria-label="Chat view">
                    <Tooltip title="Chat">
                      <ForumOutlinedIcon sx={{ fontSize: 18 }} />
                    </Tooltip>
                  </ToggleButton>
                )}
                {activeSessionId && (
                  <ToggleButton value="desktop" aria-label="Desktop view">
                    <Tooltip title="Desktop">
                      <ComputerIcon sx={{ fontSize: 18 }} />
                    </Tooltip>
                  </ToggleButton>
                )}
                {activeSessionId && (
                  <ToggleButton value="changes" aria-label="Changes view">
                    <Tooltip title="Changes">
                      <DifferenceIcon sx={{ fontSize: 18 }} />
                    </Tooltip>
                  </ToggleButton>
                )}
                <ToggleButton value="details" aria-label="Details view">
                  <Tooltip title="Details">
                    <TuneIcon sx={{ fontSize: 18 }} />
                  </Tooltip>
                </ToggleButton>
              </ToggleButtonGroup>

              {/* Restore split view button - only on desktop when chat is collapsed */}
              {isBigScreen && chatCollapsed && (
                <Tooltip title="Restore split view">
                  <IconButton
                    size="small"
                    onClick={() => setChatCollapsed(false)}
                    sx={{ ml: 0.5 }}
                  >
                    <VerticalSplitIcon sx={{ fontSize: 18 }} />
                  </IconButton>
                </Tooltip>
              )}

              {/* Status-specific action buttons */}
              {task.status === 'backlog' && (
                <Button
                  variant="contained"
                  color={justDoItMode ? 'success' : 'warning'}
                  size="small"
                  startIcon={isStartingPlanning ? <CircularProgress size={16} color="inherit" /> : <PlayArrow />}
                  onClick={handleStartPlanning}
                  disabled={isStartingPlanning}
                  sx={{ ml: 0.5, fontSize: '0.75rem' }}
                >
                  {isStartingPlanning ? 'Starting...' : (justDoItMode ? 'Just Do It' : 'Start')}
                </Button>
              )}
              {task.status === 'spec_review' && (
                <Button
                  variant="contained"
                  color="info"
                  size="small"
                  startIcon={<Description />}
                  onClick={handleReviewSpec}
                  sx={{
                    ml: 0.5,
                    fontSize: '0.75rem',
                    animation: 'pulse-glow 2s infinite',
                    '@keyframes pulse-glow': {
                      '0%, 100%': { boxShadow: '0 0 5px rgba(41, 182, 246, 0.5)' },
                      '50%': { boxShadow: '0 0 15px rgba(41, 182, 246, 0.8)' },
                    },
                  }}
                >
                  Review Spec
                </Button>
              )}
              {task.pull_request_url && (
                <Button
                  variant="outlined"
                  color="secondary"
                  size="small"
                  startIcon={<LaunchIcon />}
                  onClick={() => window.open(task.pull_request_url, '_blank')}
                  sx={{ ml: 0.5, fontSize: '0.75rem' }}
                >
                  View PR
                </Button>
              )}

              {/* Spacer - hidden on very small screens to allow wrapping */}
              <Box sx={{ flex: 1, minWidth: { xs: 0, sm: 8 } }} />

              {/* Right: Action buttons */}
              <Box sx={{ display: 'flex', gap: 0.25, alignItems: 'center', flexShrink: 0 }}>
                {isEditMode ? (
                  <>
                    <Button size="small" startIcon={<CancelIcon />} onClick={handleCancelEdit} sx={{ fontSize: '0.75rem' }}>
                      Cancel
                    </Button>
                    <Button
                      size="small"
                      color="secondary"
                      startIcon={<SaveIcon />}
                      onClick={handleSaveEdit}
                      disabled={updateSpecTask.isPending}
                      sx={{ fontSize: '0.75rem' }}
                    >
                      Save
                    </Button>
                  </>
                ) : (
                  <>
                    {task.status === TypesSpecTaskStatus.TaskStatusBacklog && (
                      <Tooltip title="Edit task">
                        <IconButton size="small" onClick={handleEditToggle}>
                          <EditIcon sx={{ fontSize: 18 }} />
                        </IconButton>
                      </Tooltip>
                    )}
                    {task.design_docs_pushed_at && (
                      <Tooltip title="Clone task to other projects">
                        <IconButton size="small" onClick={() => setShowCloneDialog(true)}>
                          <ContentCopyIcon sx={{ fontSize: 18 }} />
                        </IconButton>
                      </Tooltip>
                    )}
                    {activeSessionId && (
                      <Tooltip title="Restart agent session">
                        <IconButton
                          size="small"
                          onClick={() => setRestartConfirmOpen(true)}
                          disabled={isRestarting}
                          color="warning"
                        >
                          {isRestarting ? <CircularProgress size={16} /> : <RestartAltIcon sx={{ fontSize: 18 }} />}
                        </IconButton>
                      </Tooltip>
                    )}
                    <Tooltip title={activeSessionId ? "Upload files to sandbox" : "Start task to enable file upload"}>
                      <span>
                        <IconButton
                          size="small"
                          onClick={handleUploadClick}
                          disabled={isUploading || !activeSessionId}
                          color="primary"
                        >
                          {isUploading ? <CircularProgress size={16} /> : <CloudUploadIcon sx={{ fontSize: 18 }} />}
                        </IconButton>
                      </span>
                    </Tooltip>
                  </>
                )}
                {onClose && (
                  <IconButton size="small" onClick={onClose}>
                    <CloseIcon sx={{ fontSize: 18 }} />
                  </IconButton>
                )}
              </Box>
            </Box>

            {/* Chat View - mobile only */}
            {activeSessionId && currentView === 'chat' && (
              <Box sx={{ flex: 1, display: 'flex', flexDirection: 'column', minHeight: 0, overflow: 'hidden' }}>
                <EmbeddedSessionView ref={sessionViewRef} sessionId={activeSessionId} />
                <Box sx={{ p: 1.5, flexShrink: 0, display: 'flex', alignItems: 'flex-start', gap: 1 }}>
                  <Box sx={{ flex: 1 }}>
                    <RobustPromptInput
                      sessionId={activeSessionId}
                      specTaskId={task.id}
                      projectId={task.project_id}
                      apiClient={api.getApiClient()}
                      onSend={async (message: string, interrupt?: boolean) => {
                        await streaming.NewInference({
                          type: SESSION_TYPE_TEXT,
                          message,
                          sessionId: activeSessionId,
                          interrupt: interrupt ?? true,
                        })
                      }}
                      onHeightChange={() => sessionViewRef.current?.scrollToBottom()}
                      placeholder="Send message to agent..."
                    />
                  </Box>
                </Box>
              </Box>
            )}

            {/* Desktop View - mobile */}
            {activeSessionId && currentView === 'desktop' && (
              <Box sx={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
                <ExternalAgentDesktopViewer
                  sessionId={activeSessionId}
                  sandboxId={activeSessionId}
                  mode="stream"
                  onClientIdCalculated={setClientUniqueId}
                  displayWidth={displaySettings.width}
                  displayHeight={displaySettings.height}
                  displayFps={displaySettings.fps}
                />
              </Box>
            )}

            {/* Changes View - mobile */}
            {activeSessionId && currentView === 'changes' && (
              <Box sx={{ flex: 1, overflow: 'hidden' }}>
                <DiffViewer
                  sessionId={activeSessionId}
                  baseBranch="main"
                  pollInterval={3000}
                />
              </Box>
            )}

            {/* Details View - mobile/no session */}
            {currentView === 'details' && (
              <Box sx={{ flex: 1, overflow: 'auto', p: 3 }}>
                {renderDetailsContent()}
              </Box>
            )}
          </>
        )}
      </Box>

      {/* Restart Session Confirmation */}
      <Dialog open={restartConfirmOpen} onClose={() => setRestartConfirmOpen(false)}>
        <DialogTitle>Restart Agent Session?</DialogTitle>
        <DialogContent>
          <DialogContentText>
            This will stop the current agent container and start a fresh one.
            Any unsaved files in the sandbox may be lost.
          </DialogContentText>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setRestartConfirmOpen(false)}>Cancel</Button>
          <Button
            onClick={handleRestartSession}
            color="warning"
            variant="contained"
            disabled={isRestarting}
            startIcon={isRestarting ? <CircularProgress size={16} /> : <RestartAltIcon />}
          >
            Restart
          </Button>
        </DialogActions>
      </Dialog>

      {/* Clone Task Dialog */}
      <CloneTaskDialog
        open={showCloneDialog}
        onClose={() => setShowCloneDialog(false)}
        taskId={taskId}
        taskName={task?.name || ''}
        sourceProjectId={task?.project_id || ''}
      />

      {/* Clone Group Progress Dialog */}
      <Dialog
        open={selectedCloneGroupId !== null}
        onClose={() => setSelectedCloneGroupId(null)}
        maxWidth="md"
        fullWidth
      >
        <DialogTitle sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
          Clone Batch Progress
          <IconButton size="small" onClick={() => setSelectedCloneGroupId(null)}>
            <CloseIcon />
          </IconButton>
        </DialogTitle>
        <DialogContent>
          {selectedCloneGroupId && (
            <CloneGroupProgressFull groupId={selectedCloneGroupId} />
          )}
        </DialogContent>
      </Dialog>
    </Box>
  )
}

export default SpecTaskDetailContent
