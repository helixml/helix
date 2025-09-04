import React, { FC, useState, useEffect, useMemo } from 'react'
import {
  Box,
  Typography,
  Grid,
  Card,
  CardContent,
  CardHeader,
  Chip,
  Button,
  IconButton,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  TextField,
  Select,
  MenuItem,
  FormControl,
  InputLabel,
  Tooltip,
  Alert,
  LinearProgress,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Paper,
  Badge,
  Avatar,
  List,
  ListItem,
  ListItemIcon,
  ListItemText,

  Divider
} from '@mui/material'
import {
  Refresh as RefreshIcon,
  Add as AddIcon,
  Computer as ComputerIcon,
  Help as HelpIcon,
  CheckCircle as CheckCircleIcon,
  Warning as WarningIcon,
  Error as ErrorIcon,
  Schedule as ScheduleIcon,
  PlayArrow as PlayArrowIcon,
  Pause as PauseIcon,
  OpenInNew as OpenInNewIcon,
  Assignment as AssignmentIcon,
  Person as PersonIcon,
  Timeline as TimelineIcon,
  Edit as EditIcon,
  Visibility as VisibilityIcon
} from '@mui/icons-material'
import { useTheme } from '@mui/material/styles'
// Removed date-fns dependency - using native JavaScript instead

import useApi from '../../hooks/useApi'
import useAccount from '../../hooks/useAccount'
import { IApp } from '../../types'

// Types for agent dashboard data
interface AgentSession {
  id: string
  session_id: string
  agent_type: string
  status: string
  current_task: string
  current_work_item: string
  user_id: string
  app_id: string
  health_status: string
  created_at: string
  last_activity: string
  rdp_port?: number
  workspace_dir?: string
}

interface WorkItem {
  id: string
  name: string
  description: string
  source: string
  source_url?: string
  priority: number
  status: string
  agent_type: string
  assigned_session_id?: string
  created_at: string
  started_at?: string
  completed_at?: string
  
  // Spec-driven workflow fields
  original_prompt?: string
  requirements_spec?: string
  technical_design?: string
  implementation_plan?: string
  spec_agent?: string
  implementation_agent?: string
  spec_session_id?: string
  implementation_session_id?: string
  spec_approved_by?: string
  spec_approved_at?: string
  spec_revision_count?: number
  branch_name?: string
}

interface HelpRequest {
  id: string
  session_id: string
  help_type: string
  context: string
  specific_need: string
  urgency: string
  status: string
  created_at: string
}

interface JobCompletion {
  id: string
  session_id: string
  completion_status: string
  summary: string
  review_needed: boolean
  confidence: string
  created_at: string
}

interface AgentDashboardData {
  active_sessions: AgentSession[]
  sessions_needing_help: AgentSession[]
  pending_work: WorkItem[]
  running_work: WorkItem[]
  recent_completions: JobCompletion[]
  pending_reviews: JobCompletion[]
  active_help_requests: HelpRequest[]
  work_queue_stats: {
    total_pending: number
    total_running: number
    total_completed: number
    total_failed: number
    active_sessions: number
    by_agent_type: Record<string, number>
    by_source: Record<string, number>
    average_wait_time_minutes: number
  }
  last_updated: string
}

interface AgentDashboardProps {
  apps: IApp[]
}

const AgentDashboard: FC<AgentDashboardProps> = ({ apps }) => {
  const theme = useTheme()
  const api = useApi()
  const account = useAccount()
  
  const [dashboardData, setDashboardData] = useState<AgentDashboardData | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [createWorkItemOpen, setCreateWorkItemOpen] = useState(false)
  const [helpRequestOpen, setHelpRequestOpen] = useState<HelpRequest | null>(null)
  const [selectedSession, setSelectedSession] = useState<AgentSession | null>(null)
  const [newTaskPrompt, setNewTaskPrompt] = useState('')
  const [newTaskPriority, setNewTaskPriority] = useState('medium')
  const [newTaskType, setNewTaskType] = useState('feature')
  const [selectedProjectId, setSelectedProjectId] = useState('')
  const [createTaskLoading, setCreateTaskLoading] = useState(false)
  const [specReviewOpen, setSpecReviewOpen] = useState<WorkItem | null>(null)
  const [approvalComments, setApprovalComments] = useState('')
  const [approvalLoading, setApprovalLoading] = useState(false)
  
  // Auto-refresh every 10 seconds
  useEffect(() => {
    const fetchData = async () => {
      try {
        setLoading(true)
        const response = await api.get('/api/v1/dashboard/agent')
        setDashboardData(response.data)
        setError(null)
      } catch (err: any) {
        setError(err.message || 'Failed to load dashboard data')
      } finally {
        setLoading(false)
      }
    }

    fetchData()
    const interval = setInterval(fetchData, 10000) // Refresh every 10 seconds
    return () => clearInterval(interval)
  }, [api])

  // Status color mapping
  const getStatusColor = (status: string) => {
    switch (status) {
      // Spec-driven workflow statuses
      case 'backlog':
        return theme.palette.text.disabled // Gray - waiting
      case 'spec_generation':
        return theme.palette.warning.light // Light amber - spec being generated
      case 'spec_review':
        return theme.palette.info.main // Blue - awaiting human review
      case 'spec_revision':
        return theme.palette.warning.main // Amber - needs spec changes
      case 'spec_approved':
        return theme.palette.success.light // Light green - specs approved
      case 'implementation_queued':
        return theme.palette.secondary.main // Purple - ready for coding
      case 'implementation':
        return theme.palette.warning.main // Amber - coding in progress
      case 'implementation_review':
        return theme.palette.info.dark // Dark blue - code review
      case 'done':
        return theme.palette.success.main // Green - completed
      case 'spec_failed':
      case 'implementation_failed':
        return theme.palette.error.main // Red - failed
      // Legacy statuses
      case 'active':
        return theme.palette.warning.main // Amber - working
      case 'completed':
      case 'pending_review':
        return theme.palette.success.main // Green - done
      case 'waiting_for_help':
        return theme.palette.error.main // Red - needs help
      case 'failed':
        return theme.palette.error.main
      case 'starting':
      case 'paused':
        return theme.palette.info.main
      default:
        return theme.palette.text.secondary
    }
  }

  const getStatusIcon = (status: string) => {
    switch (status) {
      // Spec-driven workflow statuses
      case 'backlog':
        return <ScheduleIcon />
      case 'spec_generation':
        return <PlayArrowIcon />
      case 'spec_review':
        return <HelpIcon />
      case 'spec_revision':
        return <EditIcon />
      case 'spec_approved':
        return <CheckCircleIcon />
      case 'implementation_queued':
        return <ScheduleIcon />
      case 'implementation':
        return <ComputerIcon />
      case 'implementation_review':
        return <VisibilityIcon />
      case 'done':
        return <CheckCircleIcon />
      case 'spec_failed':
      case 'implementation_failed':
        return <ErrorIcon />
      // Legacy statuses
      case 'active':
        return <PlayArrowIcon />
      case 'completed':
      case 'pending_review':
        return <CheckCircleIcon />
      case 'waiting_for_help':
        return <HelpIcon />
      case 'failed':
        return <ErrorIcon />
      case 'starting':
        return <ScheduleIcon />
      case 'paused':
        return <PauseIcon />
      default:
        return <ComputerIcon />
    }
  }

  const getPriorityColor = (priority: number) => {
    if (priority <= 3) return theme.palette.error.main // High priority
    if (priority <= 6) return theme.palette.warning.main // Medium priority
    return theme.palette.info.main // Low priority
  }

  const getPhaseLabel = (status: string) => {
    switch (status) {
      case 'backlog':
        return 'Backlog'
      case 'spec_generation':
        return 'Creating Specs'
      case 'spec_review':
        return 'Awaiting Approval'
      case 'spec_revision':
        return 'Revising Specs'
      case 'spec_approved':
        return '✓ Specs Approved'
      case 'implementation_queued':
        return 'Ready for Implementation'
      case 'implementation':
        return 'Coding in Progress'
      case 'implementation_review':
        return 'Code Review'
      case 'done':
        return '✓ Completed'
      case 'spec_failed':
        return '✗ Spec Creation Failed'
      case 'implementation_failed':
        return '✗ Implementation Failed'
      default:
        return status.replace(/_/g, ' ').replace(/\b\w/g, l => l.toUpperCase())
    }
  }

  const openRDPSession = (session: AgentSession) => {
    if (session.rdp_port) {
      const rdpUrl = `rdp://localhost:${session.rdp_port}`
      window.open(rdpUrl, '_blank')
    }
  }

  const resolveHelpRequest = async (requestId: string, resolution: string) => {
    try {
      await api.post(`/api/v1/agents/help-requests/${requestId}/resolve`, {
        resolution
      })
      // Refresh data
      const response = await api.get('/api/v1/dashboard/agent')
      setDashboardData(response.data)
      setHelpRequestOpen(null)
    } catch (err: any) {
      setError(err.message || 'Failed to resolve help request')
    }
  }

  const createTwoPhaseTask = async () => {
    if (!newTaskPrompt.trim() || !selectedProjectId) {
      setError('Please enter a prompt and select a project')
      return
    }

    setCreateTaskLoading(true)
    try {
      await api.post('/api/v1/spec-tasks/from-prompt', {
        project_id: selectedProjectId,
        prompt: newTaskPrompt,
        type: newTaskType,
        priority: newTaskPriority
      })
      
      // Reset form
      setNewTaskPrompt('')
      setNewTaskPriority('medium')
      setNewTaskType('feature')
      setSelectedProjectId('')
      setCreateWorkItemOpen(false)
      
      // Refresh data
      const response = await api.get('/api/v1/dashboard/agent')
      setDashboardData(response.data)
      setError(null)
    } catch (err: any) {
      setError(err.message || 'Failed to create task')
    } finally {
      setCreateTaskLoading(false)
    }
  }

  const approveSpecs = async (approved: boolean) => {
    if (!specReviewOpen) return

    setApprovalLoading(true)
    try {
      await api.post(`/api/v1/tasks/${specReviewOpen.id}/approve-specs`, {
        approved,
        comments: approvalComments
      })
      
      // Reset form
      setApprovalComments('')
      setSpecReviewOpen(null)
      
      // Refresh data
      const response = await api.get('/api/v1/dashboard/agent')
      setDashboardData(response.data)
      setError(null)
    } catch (err: any) {
      setError(err.message || 'Failed to process spec approval')
    } finally {
      setApprovalLoading(false)
    }
  }

  if (loading && !dashboardData) {
    return <LinearProgress />
  }

  if (error) {
    return <Alert severity="error">{error}</Alert>
  }

  if (!dashboardData) {
    return <Typography>No dashboard data available</Typography>
  }

  return (
    <Box sx={{ p: 3 }}>
      {/* Header */}
      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 3 }}>
        <Typography variant="h4" component="h1">
          Agent Dashboard
        </Typography>
        <Box>
          <Button
            variant="contained"
            startIcon={<AddIcon />}
            onClick={() => setCreateWorkItemOpen(true)}
            sx={{ mr: 2 }}
          >
            Create Spec-Driven Task
          </Button>
          <IconButton onClick={() => window.location.reload()}>
            <RefreshIcon />
          </IconButton>
        </Box>
      </Box>

      {/* Summary Cards */}
      <Grid container spacing={3} sx={{ mb: 4 }}>
        <Grid item xs={12} sm={6} md={3}>
          <Card>
            <CardContent>
              <Typography color="textSecondary" gutterBottom>
                Active Sessions
              </Typography>
              <Typography variant="h4">
                {dashboardData.work_queue_stats.active_sessions}
              </Typography>
              <Typography variant="body2" color="warning.main">
                {dashboardData.sessions_needing_help.length} need help
              </Typography>
            </CardContent>
          </Card>
        </Grid>
        <Grid item xs={12} sm={6} md={3}>
          <Card>
            <CardContent>
              <Typography color="textSecondary" gutterBottom>
                Pending Work
              </Typography>
              <Typography variant="h4">
                {dashboardData.work_queue_stats.total_pending}
              </Typography>
              <Typography variant="body2" color="info.main">
                Avg wait: {Math.round(dashboardData.work_queue_stats.average_wait_time_minutes)}min
              </Typography>
            </CardContent>
          </Card>
        </Grid>
        <Grid item xs={12} sm={6} md={3}>
          <Card>
            <CardContent>
              <Typography color="textSecondary" gutterBottom>
                Running Work
              </Typography>
              <Typography variant="h4">
                {dashboardData.work_queue_stats.total_running}
              </Typography>
              <Typography variant="body2" color="warning.main">
                In progress
              </Typography>
            </CardContent>
          </Card>
        </Grid>
        <Grid item xs={12} sm={6} md={3}>
          <Card>
            <CardContent>
              <Typography color="textSecondary" gutterBottom>
                Completed Today
              </Typography>
              <Typography variant="h4">
                {dashboardData.work_queue_stats.total_completed}
              </Typography>
              <Typography variant="body2" color="success.main">
                {dashboardData.pending_reviews.length} pending review
              </Typography>
            </CardContent>
          </Card>
        </Grid>
      </Grid>

      <Grid container spacing={3}>
        {/* Active Sessions */}
        <Grid item xs={12} lg={8}>
          <Card sx={{ height: 'fit-content' }}>
            <CardHeader 
              title="Active Agent Sessions"
              action={
                <Chip 
                  label={`${dashboardData.active_sessions.length} active`}
                  color="primary"
                />
              }
            />
            <CardContent>
              <TableContainer>
                <Table size="small">
                  <TableHead>
                    <TableRow>
                      <TableCell>Session</TableCell>
                      <TableCell>Agent</TableCell>
                      <TableCell>Status</TableCell>
                      <TableCell>Current Task</TableCell>
                      <TableCell>Actions</TableCell>
                    </TableRow>
                  </TableHead>
                  <TableBody>
                    {dashboardData.active_sessions.map((session) => (
                      <TableRow 
                        key={session.id}
                        sx={{ 
                          backgroundColor: session.status === 'waiting_for_help' 
                            ? theme.palette.error.light + '20' 
                            : 'inherit'
                        }}
                      >
                        <TableCell>
                          <Box sx={{ display: 'flex', alignItems: 'center' }}>
                            <Avatar sx={{ width: 24, height: 24, mr: 1, bgcolor: getStatusColor(session.status) }}>
                              {getStatusIcon(session.status)}
                            </Avatar>
                            <Box>
                              <Typography variant="body2" fontWeight="bold">
                                {session.session_id.slice(0, 8)}...
                              </Typography>
                              <Typography variant="caption" color="textSecondary">
                                {formatTimeAgo(new Date(session.last_activity))}
                              </Typography>
                            </Box>
                          </Box>
                        </TableCell>
                        <TableCell>
                          <Chip 
                            label={session.agent_type}
                            size="small"
                            variant="outlined"
                          />
                        </TableCell>
                        <TableCell>
                          <Chip
                            label={session.status.replace('_', ' ')}
                            size="small"
                            sx={{
                              backgroundColor: getStatusColor(session.status) + '20',
                              color: getStatusColor(session.status),
                              fontWeight: 'bold'
                            }}
                          />
                          {session.status === 'waiting_for_help' && (
                            <Badge color="error" variant="dot" sx={{ ml: 1 }} />
                          )}
                        </TableCell>
                        <TableCell>
                          <Typography variant="body2" sx={{ maxWidth: 200 }}>
                            {session.current_task || 'Idle'}
                          </Typography>
                        </TableCell>
                        <TableCell>
                          <Box sx={{ display: 'flex', gap: 1 }}>
                            {session.rdp_port && (
                              <Tooltip title="Open RDP Session">
                                <IconButton 
                                  size="small"
                                  onClick={() => openRDPSession(session)}
                                >
                                  <OpenInNewIcon />
                                </IconButton>
                              </Tooltip>
                            )}
                            <Tooltip title="View Details">
                              <IconButton 
                                size="small"
                                onClick={() => setSelectedSession(session)}
                              >
                                <TimelineIcon />
                              </IconButton>
                            </Tooltip>
                          </Box>
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </TableContainer>
              {dashboardData.active_sessions.length === 0 && (
                <Typography textAlign="center" color="textSecondary" sx={{ py: 4 }}>
                  No active agent sessions
                </Typography>
              )}
            </CardContent>
          </Card>
        </Grid>

        {/* Help Requests & Alerts */}
        <Grid item xs={12} lg={4}>
          <Grid container spacing={2}>
            {/* Help Requests */}
            <Grid item xs={12}>
              <Card>
                <CardHeader 
                  title="Help Requests"
                  action={
                    <Badge badgeContent={dashboardData.active_help_requests.length} color="error">
                      <HelpIcon />
                    </Badge>
                  }
                />
                <CardContent sx={{ maxHeight: 300, overflow: 'auto' }}>
                  <List dense>
                    {dashboardData.active_help_requests.map((request) => (
                      <ListItem
                        key={request.id}
                        button
                        onClick={() => setHelpRequestOpen(request)}
                        sx={{
                          border: 1,
                          borderColor: 'error.main',
                          borderRadius: 1,
                          mb: 1,
                          backgroundColor: 'error.light' + '10'
                        }}
                      >
                        <ListItemIcon>
                          <HelpIcon color="error" />
                        </ListItemIcon>
                        <ListItemText
                          primary={request.help_type.replace('_', ' ')}
                          secondary={
                            <>
                              <Typography variant="caption" display="block">
                                {request.context.slice(0, 50)}...
                              </Typography>
                              <Chip 
                                label={request.urgency}
                                size="small"
                                color={request.urgency === 'critical' ? 'error' : request.urgency === 'high' ? 'warning' : 'info'}
                              />
                            </>
                          }
                        />
                      </ListItem>
                    ))}
                  </List>
                  {dashboardData.active_help_requests.length === 0 && (
                    <Typography textAlign="center" color="textSecondary" sx={{ py: 2 }}>
                      No help requests
                    </Typography>
                  )}
                </CardContent>
              </Card>
            </Grid>

            {/* Recent Completions */}
            <Grid item xs={12}>
              <Card>
                <CardHeader 
                  title="Recent Completions"
                  action={
                    <Badge badgeContent={dashboardData.pending_reviews.length} color="success">
                      <CheckCircleIcon />
                    </Badge>
                  }
                />
                <CardContent sx={{ maxHeight: 300, overflow: 'auto' }}>
                  <List dense>
                    {dashboardData.recent_completions.slice(0, 5).map((completion) => (
                      <ListItem key={completion.id}>
                        <ListItemIcon>
                          <CheckCircleIcon color="success" />
                        </ListItemIcon>
                        <ListItemText
                          primary={completion.summary.slice(0, 40) + '...'}
                          secondary={
                            <Box>
                              <Typography variant="caption" display="block">
                                {formatTimeAgo(new Date(completion.created_at))}
                              </Typography>
                              <Box sx={{ display: 'flex', gap: 0.5, mt: 0.5 }}>
                                <Chip 
                                  label={completion.confidence}
                                  size="small"
                                  color={completion.confidence === 'high' ? 'success' : completion.confidence === 'medium' ? 'warning' : 'error'}
                                />
                                {completion.review_needed && (
                                  <Chip label="Review needed" size="small" color="info" />
                                )}
                              </Box>
                            </Box>
                          }
                        />
                      </ListItem>
                    ))}
                  </List>
                  {dashboardData.recent_completions.length === 0 && (
                    <Typography textAlign="center" color="textSecondary" sx={{ py: 2 }}>
                      No recent completions
                    </Typography>
                  )}
                </CardContent>
              </Card>
            </Grid>
          </Grid>
        </Grid>
      </Grid>

      {/* Work Queue */}
      <Grid container spacing={3} sx={{ mt: 2 }}>
        <Grid item xs={12} md={6}>
          <Card>
            <CardHeader title="Pending Work Queue" />
            <CardContent>
              <TableContainer sx={{ maxHeight: 400, overflow: 'auto' }}>
                <Table size="small">
                  <TableHead>
                    <TableRow>
                      <TableCell>Priority</TableCell>
                      <TableCell>Task</TableCell>
                      <TableCell>Source</TableCell>
                      <TableCell>Agent Type</TableCell>
                      <TableCell>Created</TableCell>
                    </TableRow>
                  </TableHead>
                  <TableBody>
                    {dashboardData.pending_work.map((item) => (
                      <TableRow 
                        key={item.id}
                        sx={{
                          backgroundColor: getStatusColor(item.status) + '10',
                          borderLeft: `4px solid ${getStatusColor(item.status)}`,
                        }}
                      >
                        <TableCell>
                          <Chip
                            label={item.priority}
                            size="small"
                            sx={{
                              backgroundColor: getPriorityColor(item.priority) + '20',
                              color: getPriorityColor(item.priority)
                            }}
                          />
                        </TableCell>
                        <TableCell>
                          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 1 }}>
                            {getStatusIcon(item.status)}
                            <Typography variant="body2" fontWeight="bold">
                              {item.name}
                            </Typography>
                          </Box>
                          
                          {/* Phase indicator */}
                          <Box sx={{ mb: 1 }}>
                            <Chip 
                              label={getPhaseLabel(item.status)}
                              size="small"
                              sx={{
                                backgroundColor: getStatusColor(item.status),
                                color: 'white',
                                fontWeight: 'bold'
                              }}
                            />
                          </Box>

                          {/* Original prompt */}
                          <Typography variant="caption" color="textSecondary" sx={{ display: 'block', mb: 1 }}>
                            <strong>Original:</strong> {(item.original_prompt || item.description).slice(0, 80)}...
                          </Typography>

                          {/* Show specs when available */}
                          {(item.status === 'spec_review' || item.status === 'spec_revision' || 
                            item.status === 'spec_approved' || item.status === 'implementation_queued' ||
                            item.status === 'implementation' || item.status === 'implementation_review' ||
                            item.status === 'done') && item.requirements_spec && (
                            <Box sx={{ mt: 1, p: 1, backgroundColor: theme.palette.grey.A100, borderRadius: 1 }}>
                              <Typography variant="caption" fontWeight="bold" color="textSecondary">
                                Generated Specs Available
                              </Typography>
                              <Typography variant="caption" sx={{ display: 'block' }}>
                                ✓ Requirements • ✓ Technical Design • ✓ Implementation Plan
                              </Typography>
                            </Box>
                          )}

                          {/* Show implementation info when in coding phase */}
                          {(item.status === 'implementation' || item.status === 'implementation_review' || 
                            item.status === 'done') && item.implementation_agent && (
                            <Box sx={{ mt: 1, p: 1, backgroundColor: theme.palette.success.light, borderRadius: 1 }}>
                              <Typography variant="caption" fontWeight="bold" color="success.dark">
                                Implementation in Progress
                              </Typography>
                              <Typography variant="caption" sx={{ display: 'block' }}>
                                Agent: {item.implementation_agent}
                                {item.branch_name && ` • Branch: ${item.branch_name}`}
                              </Typography>
                            </Box>
                          )}

                          {/* Show approval info */}
                          {item.spec_approved_by && (
                            <Typography variant="caption" color="success.main" sx={{ display: 'block', mt: 1 }}>
                              ✓ Specs approved by {item.spec_approved_by}
                              {item.spec_approved_at && ` ${formatTimeAgo(new Date(item.spec_approved_at))}`}
                            </Typography>
                          )}
                        </TableCell>
                        <TableCell>
                          <Chip label={item.source} size="small" variant="outlined" />
                        </TableCell>
                        <TableCell>
                          <Typography variant="caption">
                            {item.agent_type}
                            {item.spec_agent && item.implementation_agent && (
                              <Box sx={{ mt: 0.5 }}>
                                <Typography variant="caption" color="textSecondary">
                                  Spec: {item.spec_agent}
                                </Typography>
                                <Typography variant="caption" color="textSecondary" sx={{ display: 'block' }}>
                                  Code: {item.implementation_agent}
                                </Typography>
                              </Box>
                            )}
                          </Typography>
                        </TableCell>
                        <TableCell>
                          <Typography variant="caption">
                            {formatTimeAgo(new Date(item.created_at))}
                          </Typography>
                          {item.spec_revision_count && item.spec_revision_count > 0 && (
                            <Typography variant="caption" color="warning.main" sx={{ display: 'block' }}>
                              {item.spec_revision_count} revision{item.spec_revision_count > 1 ? 's' : ''}
                            </Typography>
                          )}
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </TableContainer>
              {dashboardData.pending_work.length === 0 && (
                <Typography textAlign="center" color="textSecondary" sx={{ py: 4 }}>
                  No pending work items
                </Typography>
              )}
            </CardContent>
          </Card>
        </Grid>

        <Grid item xs={12} md={6}>
          <Card>
            <CardHeader title="Running Work" />
            <CardContent>
              <TableContainer sx={{ maxHeight: 400, overflow: 'auto' }}>
                <Table size="small">
                  <TableHead>
                    <TableRow>
                      <TableCell>Task</TableCell>
                      <TableCell>Session</TableCell>
                      <TableCell>Started</TableCell>
                      <TableCell>Duration</TableCell>
                    </TableRow>
                  </TableHead>
                  <TableBody>
                    {dashboardData.running_work.map((item) => {
                      const startTime = item.started_at ? new Date(item.started_at) : null
                      const duration = startTime ? formatTimeAgo(startTime) : 'Unknown'
                      
                      return (
                        <TableRow 
                          key={item.id}
                          sx={{
                            backgroundColor: getStatusColor(item.status) + '10',
                            borderLeft: `4px solid ${getStatusColor(item.status)}`,
                          }}
                        >
                          <TableCell>
                            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 1 }}>
                              {getStatusIcon(item.status)}
                              <Typography variant="body2" fontWeight="bold">
                                {item.name}
                              </Typography>
                            </Box>
                            
                            {/* Phase indicator */}
                            <Box sx={{ mb: 1 }}>
                              <Chip 
                                label={getPhaseLabel(item.status)}
                                size="small"
                                sx={{
                                  backgroundColor: getStatusColor(item.status),
                                  color: 'white',
                                  fontWeight: 'bold'
                                }}
                              />
                            </Box>

                            <Typography variant="caption" color="textSecondary">
                              {(item.original_prompt || item.description).slice(0, 60)}...
                            </Typography>

                            {/* Show current agent working */}
                            {item.status === 'spec_generation' && item.spec_agent && (
                              <Box sx={{ mt: 1, p: 1, backgroundColor: theme.palette.warning.light, borderRadius: 1 }}>
                                <Typography variant="caption" fontWeight="bold" color="warning.dark">
                                  🤖 {item.spec_agent} generating specifications
                                </Typography>
                              </Box>
                            )}

                            {item.status === 'implementation' && item.implementation_agent && (
                              <Box sx={{ mt: 1, p: 1, backgroundColor: theme.palette.success.light, borderRadius: 1 }}>
                                <Typography variant="caption" fontWeight="bold" color="success.dark">
                                  💻 {item.implementation_agent} coding
                                </Typography>
                                {item.branch_name && (
                                  <Typography variant="caption" sx={{ display: 'block' }}>
                                    Branch: {item.branch_name}
                                  </Typography>
                                )}
                              </Box>
                            )}
                          </TableCell>
                          <TableCell>
                            {/* Show appropriate session ID based on current phase */}
                            {item.status === 'spec_generation' && item.spec_session_id ? (
                              <Box>
                                <Typography variant="caption" fontWeight="bold">
                                  Spec Session
                                </Typography>
                                <Typography variant="caption" sx={{ display: 'block' }}>
                                  {item.spec_session_id.slice(0, 8)}...
                                </Typography>
                              </Box>
                            ) : item.status === 'implementation' && item.implementation_session_id ? (
                              <Box>
                                <Typography variant="caption" fontWeight="bold">
                                  Code Session
                                </Typography>
                                <Typography variant="caption" sx={{ display: 'block' }}>
                                  {item.implementation_session_id.slice(0, 8)}...
                                </Typography>
                              </Box>
                            ) : item.assigned_session_id ? (
                              <Typography variant="caption">
                                {item.assigned_session_id.slice(0, 8)}...
                              </Typography>
                            ) : (
                              <Typography variant="caption" color="textSecondary">
                                Not assigned
                              </Typography>
                            )}
                          </TableCell>
                          <TableCell>
                            <Typography variant="caption">
                              {startTime ? formatDateTime(startTime) : 'Unknown'}
                            </Typography>
                            {/* Show spec approval time if relevant */}
                            {item.spec_approved_at && item.status !== 'spec_generation' && (
                              <Typography variant="caption" color="success.main" sx={{ display: 'block' }}>
                                ✓ {formatTimeAgo(new Date(item.spec_approved_at))}
                              </Typography>
                            )}
                          </TableCell>
                          <TableCell>
                            <Typography variant="caption">
                              {duration}
                            </Typography>
                            {/* Show revision count if any */}
                            {item.spec_revision_count && item.spec_revision_count > 0 && (
                              <Typography variant="caption" color="warning.main" sx={{ display: 'block' }}>
                                {item.spec_revision_count} revision{item.spec_revision_count > 1 ? 's' : ''}
                              </Typography>
                            )}
                          </TableCell>
                        </TableRow>
                      )
                    })}
                  </TableBody>
                </Table>
              </TableContainer>
              {dashboardData.running_work.length === 0 && (
                <Typography textAlign="center" color="textSecondary" sx={{ py: 4 }}>
                  No running work items
                </Typography>
              )}
            </CardContent>
          </Card>
        </Grid>
      </Grid>

      {/* Help Request Dialog */}
      {helpRequestOpen && (
        <Dialog open={!!helpRequestOpen} onClose={() => setHelpRequestOpen(null)} maxWidth="md" fullWidth>
          <DialogTitle>
            Resolve Help Request
            <Chip 
              label={helpRequestOpen.urgency}
              size="small"
              color={helpRequestOpen.urgency === 'critical' ? 'error' : helpRequestOpen.urgency === 'high' ? 'warning' : 'info'}
              sx={{ ml: 2 }}
            />
          </DialogTitle>
          <DialogContent>
            <Box sx={{ mb: 2 }}>
              <Typography variant="subtitle2" gutterBottom>Context:</Typography>
              <Typography variant="body2" sx={{ mb: 2 }}>{helpRequestOpen.context}</Typography>
              
              <Typography variant="subtitle2" gutterBottom>Specific Need:</Typography>
              <Typography variant="body2" sx={{ mb: 2 }}>{helpRequestOpen.specific_need}</Typography>
            </Box>
            
            <TextField
              fullWidth
              multiline
              rows={4}
              label="Your Resolution"
              placeholder="Provide guidance or solution for the agent..."
              onChange={(e) => {
                // Store resolution in state
                setHelpRequestOpen({
                  ...helpRequestOpen,
                  resolution: e.target.value
                } as any)
              }}
            />
          </DialogContent>
          <DialogActions>
            <Button onClick={() => setHelpRequestOpen(null)}>Cancel</Button>
            <Button 
              variant="contained" 
              onClick={() => {
                const resolution = (helpRequestOpen as any).resolution
                if (resolution) {
                  resolveHelpRequest(helpRequestOpen.id, resolution)
                }
              }}
            >
              Resolve
            </Button>
          </DialogActions>
        </Dialog>
      )}

      {/* Create Spec-Driven Task Dialog */}
      <Dialog open={createWorkItemOpen} onClose={() => setCreateWorkItemOpen(false)} maxWidth="md" fullWidth>
        <DialogTitle>
          Create Spec-Driven Task
          <Typography variant="body2" color="textSecondary" sx={{ mt: 1 }}>
            Describe what you want built in simple terms. AI will generate detailed specs, then code it.
          </Typography>
        </DialogTitle>
        <DialogContent>
          <Box sx={{ display: 'flex', flexDirection: 'column', gap: 3, mt: 2 }}>
            {/* Simple prompt input */}
            <TextField
              label="Describe what you want built"
              placeholder="e.g., Add a user profile page with avatar upload and settings"
              multiline
              rows={4}
              value={newTaskPrompt}
              onChange={(e) => setNewTaskPrompt(e.target.value)}
              fullWidth
              helperText="Be as specific or as general as you like - the AI will ask for clarification if needed"
            />

            {/* Project selection */}
            <FormControl fullWidth>
              <InputLabel>Project</InputLabel>
              <Select
                value={selectedProjectId}
                onChange={(e) => setSelectedProjectId(e.target.value)}
                label="Project"
              >
                {apps.map((app) => (
                  <MenuItem key={app.id} value={app.id}>
                    {app.config.helix.name}
                  </MenuItem>
                ))}
              </Select>
            </FormControl>

            <Grid container spacing={2}>
              {/* Task type */}
              <Grid item xs={6}>
                <FormControl fullWidth>
                  <InputLabel>Type</InputLabel>
                  <Select
                    value={newTaskType}
                    onChange={(e) => setNewTaskType(e.target.value)}
                    label="Type"
                  >
                    <MenuItem value="feature">Feature</MenuItem>
                    <MenuItem value="bug">Bug Fix</MenuItem>
                    <MenuItem value="improvement">Improvement</MenuItem>
                    <MenuItem value="refactor">Refactor</MenuItem>
                  </Select>
                </FormControl>
              </Grid>

              {/* Priority */}
              <Grid item xs={6}>
                <FormControl fullWidth>
                  <InputLabel>Priority</InputLabel>
                  <Select
                    value={newTaskPriority}
                    onChange={(e) => setNewTaskPriority(e.target.value)}
                    label="Priority"
                  >
                    <MenuItem value="low">Low</MenuItem>
                    <MenuItem value="medium">Medium</MenuItem>
                    <MenuItem value="high">High</MenuItem>
                    <MenuItem value="critical">Critical</MenuItem>
                  </Select>
                </FormControl>
              </Grid>
            </Grid>

            {/* Process explanation */}
            <Paper sx={{ p: 2, backgroundColor: theme.palette.info.main + '10' }}>
              <Typography variant="subtitle2" color="info.main" gutterBottom>
                🤖 Spec-Driven Development
              </Typography>
              <Typography variant="body2" color="textSecondary">
                <strong>Specification:</strong> AI agent creates detailed specs from your description<br/>
                <strong>Review:</strong> You approve the specs or request changes<br/>
                <strong>Implementation:</strong> Coding agent builds the approved specifications<br/>
                <strong>Result:</strong> Working code in a pull request for your review
              </Typography>
            </Paper>
          </Box>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setCreateWorkItemOpen(false)} disabled={createTaskLoading}>
            Cancel
          </Button>
          <Button 
            variant="contained"
            onClick={createTwoPhaseTask}
            disabled={createTaskLoading || !newTaskPrompt.trim() || !selectedProjectId}
          >
            {createTaskLoading ? 'Creating...' : 'Start Spec-Driven Development'}
          </Button>
        </DialogActions>
      </Dialog>

      {/* Last updated indicator */}
      <Box sx={{ mt: 3, textAlign: 'center' }}>
        <Typography variant="caption" color="textSecondary">
          Last updated: {formatTimeAgo(new Date(dashboardData.last_updated))}
        </Typography>
      </Box>
    </Box>
  )
}

// Helper functions to replace date-fns
function formatTimeAgo(date: Date): string {
  const now = new Date();
  const diffMs = now.getTime() - date.getTime();
  const diffMins = Math.floor(diffMs / 60000);
  const diffHours = Math.floor(diffMs / 3600000);
  const diffDays = Math.floor(diffMs / 86400000);

  if (diffMins < 1) return 'just now';
  if (diffMins < 60) return `${diffMins} minute${diffMins === 1 ? '' : 's'} ago`;
  if (diffHours < 24) return `${diffHours} hour${diffHours === 1 ? '' : 's'} ago`;
  if (diffDays < 30) return `${diffDays} day${diffDays === 1 ? '' : 's'} ago`;
  
  return date.toLocaleDateString();
}

function formatDateTime(date: Date): string {
  return date.toLocaleString();
}

export default AgentDashboard