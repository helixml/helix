import React, { FC, useState, useEffect, useCallback, useMemo } from 'react'
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
import MenuBookIcon from '@mui/icons-material/MenuBook'
import ChatIcon from '@mui/icons-material/Chat'
import DesktopWindowsIcon from '@mui/icons-material/DesktopWindows'
import InfoOutlinedIcon from '@mui/icons-material/InfoOutlined'
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
import { useUpdateSpecTask, useSpecTask } from '../../services/specTaskService'
import RobustPromptInput from '../common/RobustPromptInput'
import EmbeddedSessionView from '../session/EmbeddedSessionView'
import PromptLibrarySidebar from '../common/PromptLibrarySidebar'
import { usePromptHistory } from '../../hooks/usePromptHistory'

interface SpecTaskDetailContentProps {
  taskId: string
  onClose?: () => void
}

const SpecTaskDetailContent: FC<SpecTaskDetailContentProps> = ({
  taskId,
  onClose,
}) => {
  const api = useApi()
  const snackbar = useSnackbar()
  const streaming = useStreaming()
  const apps = useApps()
  const updateSpecTask = useUpdateSpecTask()
  const queryClient = useQueryClient()

  // Fetch task data
  const { data: task } = useSpecTask(taskId, {
    enabled: !!taskId,
    refetchInterval: 2000,
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

  const [currentView, setCurrentView] = useState<'session' | 'desktop' | 'details'>('session')
  const [clientUniqueId, setClientUniqueId] = useState<string>('')

  // Design review state
  const [docViewerOpen, setDocViewerOpen] = useState(false)
  const [designReviewViewerOpen, setDesignReviewViewerOpen] = useState(false)
  const [activeReviewId, setActiveReviewId] = useState<string | null>(null)
  const [implementationReviewMessageSent, setImplementationReviewMessageSent] = useState(false)

  // Session restart state
  const [restartConfirmOpen, setRestartConfirmOpen] = useState(false)
  const [isRestarting, setIsRestarting] = useState(false)

  // Prompt library sidebar state
  const [showPromptLibrary, setShowPromptLibrary] = useState(false)

  // Just Do It mode state
  const [justDoItMode, setJustDoItMode] = useState(false)
  const [updatingJustDoIt, setUpdatingJustDoIt] = useState(false)

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

  // Default to details view when no active session
  useEffect(() => {
    if (!activeSessionId && currentView !== 'details') {
      setCurrentView('details')
    }
  }, [activeSessionId, currentView])

  // Fetch session data
  const { data: sessionResponse } = useGetSession(activeSessionId || '', { enabled: !!activeSessionId })
  const sessionData = sessionResponse?.data
  const wolfLobbyId = sessionData?.config?.wolf_lobby_id

  // Initialize prompt history for the session
  const promptHistory = usePromptHistory({
    sessionId: activeSessionId || 'default',
    specTaskId: task?.id,
    projectId: task?.project_id,
    apiClient: api.getApiClient(),
  })

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
    if (!task?.id) return

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
      setCurrentTab(0)
    } catch (err: any) {
      console.error('Failed to start planning:', err)
      snackbar.error(err?.message || 'Failed to start planning. Please try again.')
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

  if (!task) {
    return (
      <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '100%' }}>
        <CircularProgress />
      </Box>
    )
  }

  return (
    <Box sx={{ display: 'flex', flexDirection: 'column', height: '100%', overflow: 'hidden' }}>
      {/* Header with view toggles */}
      <Box
        sx={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          px: 1.5,
          py: 1,
          borderBottom: '1px solid',
          borderColor: 'divider',
          backgroundColor: 'background.paper',
          gap: 1,
        }}
      >
        {/* Left: Task info */}
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, flex: 1, minWidth: 0 }}>
          <Typography variant="body2" noWrap sx={{ fontWeight: 500, flex: 1 }}>
            {task.name || task.description || 'Unnamed task'}
          </Typography>
          <Chip label={formatStatus(task.status)} color={getStatusColor(task.status)} size="small" sx={{ height: 20, fontSize: '0.7rem' }} />
          <Chip label={task.priority || 'Medium'} color={getPriorityColor(task.priority)} size="small" sx={{ height: 20, fontSize: '0.7rem' }} />
        </Box>

        {/* Center: View toggle icons */}
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
          {activeSessionId && (
            <ToggleButton value="session" aria-label="Session view">
              <Tooltip title="Session">
                <ChatIcon sx={{ fontSize: 18 }} />
              </Tooltip>
            </ToggleButton>
          )}
          {activeSessionId && (
            <ToggleButton value="desktop" aria-label="Desktop view">
              <Tooltip title="Desktop">
                <DesktopWindowsIcon sx={{ fontSize: 18 }} />
              </Tooltip>
            </ToggleButton>
          )}
          <ToggleButton value="details" aria-label="Details view">
            <Tooltip title="Details">
              <InfoOutlinedIcon sx={{ fontSize: 18 }} />
            </Tooltip>
          </ToggleButton>
        </ToggleButtonGroup>

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
            </>
          )}
          {onClose && (
            <IconButton size="small" onClick={onClose}>
              <CloseIcon sx={{ fontSize: 18 }} />
            </IconButton>
          )}
        </Box>
      </Box>

      {/* Tab Content */}
      <Box sx={{ flex: 1, overflow: 'hidden', display: 'flex', flexDirection: 'column' }}>
        {/* Session View */}
        {activeSessionId && currentView === 'session' && (
          <Box sx={{ display: 'flex', flex: 1, overflow: 'hidden' }}>
            <Box sx={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
              <EmbeddedSessionView sessionId={activeSessionId} />
              <Box sx={{ p: 2, borderTop: 1, borderColor: 'divider', flexShrink: 0, display: 'flex', alignItems: 'flex-start', gap: 1 }}>
                <Box sx={{ flex: 1 }}>
                  <RobustPromptInput
                    sessionId={activeSessionId}
                    specTaskId={task.id}
                    projectId={task.project_id}
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
                <Tooltip title={showPromptLibrary ? 'Hide prompt library' : 'Show prompt library'}>
                  <IconButton
                    size="small"
                    onClick={() => setShowPromptLibrary(!showPromptLibrary)}
                    sx={{ mt: 0.5, color: showPromptLibrary ? 'primary.main' : 'text.secondary' }}
                  >
                    <MenuBookIcon sx={{ fontSize: 20 }} />
                  </IconButton>
                </Tooltip>
              </Box>
            </Box>
            {showPromptLibrary && (
              <Box sx={{ width: 320, flexShrink: 0 }}>
                <PromptLibrarySidebar
                  pinnedPrompts={promptHistory.history.filter(h => h.pinned)}
                  templates={promptHistory.history.filter(h => h.isTemplate)}
                  recentPrompts={promptHistory.history.filter(h => h.status === 'sent').slice(-20).reverse()}
                  onSelectPrompt={(content) => promptHistory.setDraft(content)}
                  onPinPrompt={promptHistory.pinPrompt}
                  onSetTemplate={promptHistory.setTemplate}
                  onSearch={promptHistory.searchHistory}
                  onClose={() => setShowPromptLibrary(false)}
                />
              </Box>
            )}
          </Box>
        )}

        {/* Desktop View */}
        {activeSessionId && currentView === 'desktop' && (
          <>
            <ExternalAgentDesktopViewer
              sessionId={activeSessionId}
              wolfLobbyId={wolfLobbyId}
              mode="stream"
              onClientIdCalculated={setClientUniqueId}
              displayWidth={displaySettings.width}
              displayHeight={displaySettings.height}
              displayFps={displaySettings.fps}
            />
            <Box sx={{ p: 2, borderTop: 1, borderColor: 'divider', flexShrink: 0 }}>
              <RobustPromptInput
                sessionId={activeSessionId}
                specTaskId={task.id}
                projectId={task.project_id}
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

        {/* Details View */}
        {currentView === 'details' && (
          <Box sx={{ flex: 1, overflow: 'auto', p: 3 }}>
            {/* Action Buttons */}
            <Box sx={{ mb: 3, display: 'flex', gap: 1, flexWrap: 'wrap', alignItems: 'center' }}>
              {task.status === 'backlog' && (
                <>
                  <Button
                    variant="contained"
                    color={justDoItMode ? 'success' : 'warning'}
                    startIcon={<PlayArrow />}
                    onClick={handleStartPlanning}
                  >
                    {justDoItMode ? 'Just Do It' : 'Start Planning'}
                  </Button>
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
                    label={<Typography variant="body2">Just Do It</Typography>}
                    sx={{ ml: 1 }}
                  />
                </>
              )}
              {task.status === 'spec_review' && (
                <Button
                  variant="contained"
                  color="info"
                  startIcon={<Description />}
                  onClick={async () => {
                    try {
                      const response = await api.getApiClient().v1SpecTasksDesignReviewsDetail(task.id!)
                      const reviews = response.data?.reviews || []
                      if (reviews.length > 0) {
                        const latestReview = reviews.find((r: any) => r.status !== 'superseded') || reviews[0]
                        setActiveReviewId(latestReview.id)
                        setDesignReviewViewerOpen(true)
                      } else {
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
              {task.pull_request_url && (
                <Button
                  variant="outlined"
                  color="primary"
                  startIcon={<LaunchIcon />}
                  onClick={() => window.open(task.pull_request_url, '_blank')}
                >
                  View Pull Request
                </Button>
              )}
            </Box>

            <Divider sx={{ mb: 3 }} />

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
                  {task.description || task.original_prompt || 'No description provided'}
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
                  <Chip label={task.priority || 'Medium'} color={getPriorityColor(task.priority)} size="small" />
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
                Created: {task.created_at ? new Date(task.created_at).toLocaleString() : 'N/A'}
              </Typography>
              <Typography variant="caption" color="text.secondary" display="block">
                Updated: {task.updated_at ? new Date(task.updated_at).toLocaleString() : 'N/A'}
              </Typography>
            </Box>

            {/* Debug Info */}
            <Divider sx={{ my: 2 }} />
            <Box sx={{ mt: 2, p: 2, bgcolor: 'grey.900', borderRadius: 1 }}>
              <Typography variant="caption" color="grey.400" display="block" gutterBottom>
                Debug Information
              </Typography>
              <Typography variant="caption" color="grey.300" sx={{ fontFamily: 'monospace', display: 'block' }}>
                Task ID: {task.id || 'N/A'}
              </Typography>
              {task.branch_name && (
                <Typography variant="caption" color="grey.300" sx={{ fontFamily: 'monospace', display: 'block' }}>
                  Branch: {task.branch_name}
                </Typography>
              )}
              {activeSessionId && (
                <Typography variant="caption" color="grey.300" sx={{ fontFamily: 'monospace', display: 'block' }}>
                  Session ID: {activeSessionId}
                </Typography>
              )}
            </Box>
          </Box>
        )}
      </Box>

      {/* Design Review Viewer */}
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
    </Box>
  )
}

export default SpecTaskDetailContent
