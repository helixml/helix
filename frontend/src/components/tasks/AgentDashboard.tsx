import React, { FC, useState, useEffect, useMemo, useRef } from 'react'
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
  Visibility as VisibilityIcon,
} from '@mui/icons-material'
import { useTheme } from '@mui/material/styles'
// Removed date-fns dependency - using native JavaScript instead

import useApi from '../../hooks/useApi'
import useAccount from '../../hooks/useAccount'
import { IApp } from '../../types'
import { TypesAgentFleetSummary, TypesAgentSessionStatus, TypesAgentWorkItem, TypesHelpRequest, TypesJobCompletion, TypesAgentWorkQueueStats } from '../../api/api'
import { useFloatingModal } from '../../contexts/floatingModal'
import ScreenshotViewer from '../external-agent/ScreenshotViewer'
import MoonlightConnectionButton from '../external-agent/MoonlightConnectionButton'

// Using generated API types instead of local interfaces

// External agent connection type (not yet in generated API)
interface ExternalAgentConnection {
  session_id: string
  connected_at: string
  last_ping: string
  status: string
}



interface AgentDashboardProps {
  apps: IApp[]
}

const AgentDashboard: FC<AgentDashboardProps> = ({ apps }) => {
  const theme = useTheme()
  const api = useApi()
  const account = useAccount()
  const floatingModal = useFloatingModal()
  
  const [dashboardData, setDashboardData] = useState<TypesAgentFleetSummary | null>(null)
  const [externalAgentConnections, setExternalAgentConnections] = useState<ExternalAgentConnection[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [createWorkItemOpen, setCreateWorkItemOpen] = useState(false)
  const [helpRequestOpen, setHelpRequestOpen] = useState<TypesHelpRequest | null>(null)
  const [selectedSession, setSelectedSession] = useState<TypesAgentSessionStatus | null>(null)
  const [newTaskPrompt, setNewTaskPrompt] = useState('')
  const [newTaskPriority, setNewTaskPriority] = useState('medium')
  const [newTaskType, setNewTaskType] = useState('feature')
  const [selectedProjectId, setSelectedProjectId] = useState('')
  const [createTaskLoading, setCreateTaskLoading] = useState(false)
  const [specReviewOpen, setSpecReviewOpen] = useState<TypesAgentWorkItem | null>(null)
  const [approvalComments, setApprovalComments] = useState('')
  const [runnerRDPViewerOpen, setRunnerRDPViewerOpen] = useState<string | null>(null) // runner ID
  const [approvalLoading, setApprovalLoading] = useState(false)
  const [selectedDemoRepo, setSelectedDemoRepo] = useState('')
  const [useDemoRepo, setUseDemoRepo] = useState(false)
  
  // Use ref to store API to prevent dependency issues
  const apiRef = useRef(api)
  apiRef.current = api

  // Auto-refresh every 10 seconds
  useEffect(() => {
    const fetchData = async () => {
      try {
        setLoading(true)
        
        // Fetch fleet data
        const fleetResponse = await apiRef.current.getApiClient().v1AgentsFleetList()
        setDashboardData(fleetResponse.data)
        
        // Fetch external agent connections
        try {
          const connectionsResponse = await apiRef.current.get('/api/v1/external-agents/connections')
          if (connectionsResponse) {
            setExternalAgentConnections(connectionsResponse)
          }
        } catch (extErr: any) {
          console.warn('Failed to load external agent connections:', extErr.message)
          setExternalAgentConnections([])
        }
        
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
  }, [])

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

  const openRDPSession = (session: TypesAgentSessionStatus) => {
    if (session.rdp_port) {
      const rdpUrl = `rdp://localhost:${session.rdp_port}`
      window.open(rdpUrl, '_blank')
    }
  }

  const openRunnerWebRDP = (runnerId: string) => {
    setRunnerRDPViewerOpen(runnerId)
  }

  const resolveHelpRequest = async (requestId: string, resolution: string) => {
    try {
      await apiRef.current.getApiClient().v1AgentsHelpRequestsResolveCreate(requestId, {
        resolution
      })
      // Refresh data
      const response = await apiRef.current.getApiClient().v1AgentsFleetList()
      setDashboardData(response.data)
      
      // Also refresh external agent connections
      try {
        const connectionsResponse = await apiRef.current.get('/api/v1/external-agents/connections')
        if (connectionsResponse) {
          setExternalAgentConnections(connectionsResponse)
        }
      } catch (extErr) {
        console.warn('Failed to refresh external agent connections')
      }
      
      setHelpRequestOpen(null)
    } catch (err: any) {
      setError(err.message || 'Failed to resolve help request')
    }
  }

  const createTwoPhaseTask = async () => {
    if (!newTaskPrompt.trim()) {
      setError('Please enter a prompt')
      return
    }

    // Validate project or demo repo selection
    if (!useDemoRepo && !selectedProjectId) {
      setError('Please select a project or use a demo repo')
      return
    }

    if (useDemoRepo && !selectedDemoRepo) {
      setError('Please select a demo repository')
      return
    }

    setCreateTaskLoading(true)
    try {
      if (useDemoRepo) {
        // Create task with demo repo
        await apiRef.current.post('/api/v1/spec-tasks/from-demo', {
          prompt: newTaskPrompt,
          demo_repo: selectedDemoRepo,
          type: newTaskType,
          priority: newTaskPriority
        })
      } else {
        // Create task with existing project
        await apiRef.current.getApiClient().v1SpecTasksFromPromptCreate({
          project_id: selectedProjectId,
          prompt: newTaskPrompt,
          type: newTaskType,
          priority: newTaskPriority
        })
      }

      // Reset form
      setNewTaskPrompt('')
      setNewTaskPriority('medium')
      setNewTaskType('feature')
      setSelectedProjectId('')
      setSelectedDemoRepo('')
      setUseDemoRepo(false)
      setCreateWorkItemOpen(false)

      // Refresh data
      const response = await apiRef.current.getApiClient().v1AgentsFleetList()
      setDashboardData(response.data)

      // Also refresh external agent connections
      try {
        const connectionsResponse = await api.get('/api/v1/external-agents/connections')
        if (connectionsResponse) {
          setExternalAgentConnections(connectionsResponse)
        }
      } catch (extErr) {
        console.warn('Failed to refresh external agent connections')
      }

      setError(null)
    } catch (err: any) {
      setError(err.message || 'Failed to create task')
    } finally {
      setCreateTaskLoading(false)
    }
  }

  const approveSpecs = async (approved: boolean) => {
    if (!specReviewOpen || !specReviewOpen.id) return

    setApprovalLoading(true)
    try {
      await api.getApiClient().v1SpecTasksApproveSpecsCreate(specReviewOpen.id, {
        approved,
        comments: approvalComments
      })

      // Reset form
      setApprovalComments('')
      setSpecReviewOpen(null)
      
      // Refresh data
      const response = await apiRef.current.getApiClient().v1AgentsFleetList()
      setDashboardData(response.data)
      
      // Also refresh external agent connections
      try {
        const connectionsResponse = await api.get('/api/v1/external-agents/connections')
        if (connectionsResponse) {
          setExternalAgentConnections(connectionsResponse)
        }
      } catch (extErr) {
        console.warn('Failed to refresh external agent connections')
      }
      
      setError(null)
    } catch (err: any) {
      setError(err.message || 'Failed to process spec approval')
    } finally {
      setApprovalLoading(false)
    }
  }

  const getConnectionStatus = (connection: ExternalAgentConnection) => {
    const now = new Date()
    const lastPing = new Date(connection.last_ping)
    const timeDiff = now.getTime() - lastPing.getTime()
    const secondsSinceLastPing = timeDiff / 1000

    // Consider offline if no ping for more than 2 minutes
    if (secondsSinceLastPing > 120) {
      return 'offline'
    }
    // Consider stale if no ping for more than 1 minute
    if (secondsSinceLastPing > 60) {
      return 'stale'
    }
    return 'online'
  }

  const getConnectionStatusColor = (status: string) => {
    switch (status) {
      case 'online':
        return 'success'
      case 'stale':
        return 'warning'
      case 'offline':
        return 'error'
      default:
        return 'info'
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
      <Box sx={{ 
        display: 'flex', 
        flexWrap: 'wrap', 
        gap: 3, 
        mb: 4,
        '& > *': { 
          flex: { xs: '1 1 100%', sm: '1 1 calc(50% - 12px)', md: '1 1 calc(20% - 19.2px)' },
          minWidth: { md: '180px' }
        }
      }}>
        <Card>
          <CardContent>
            <Typography color="textSecondary" gutterBottom>
              Active Sessions
            </Typography>
            <Typography variant="h4">
              {dashboardData.work_queue_stats?.active_sessions || 0}
            </Typography>
            <Typography variant="body2" color="warning.main">
              {dashboardData.sessions_needing_help?.length || 0} need help
            </Typography>
          </CardContent>
        </Card>
        <Card>
          <CardContent>
            <Typography color="textSecondary" gutterBottom>
              Pending Work
            </Typography>
            <Typography variant="h4">
              {dashboardData.work_queue_stats?.total_pending || 0}
            </Typography>
            <Typography variant="body2" color="info.main">
              Avg wait: {Math.round(dashboardData.work_queue_stats?.average_wait_time_minutes || 0)}min
            </Typography>
          </CardContent>
        </Card>
        <Card>
          <CardContent>
            <Typography color="textSecondary" gutterBottom>
              Running Work
            </Typography>
            <Typography variant="h4">
              {dashboardData.work_queue_stats?.total_running || 0}
            </Typography>
            <Typography variant="body2" color="warning.main">
              In progress
            </Typography>
          </CardContent>
        </Card>
        <Card>
          <CardContent>
            <Typography color="textSecondary" gutterBottom>
              Completed Today
            </Typography>
            <Typography variant="h4">
              {dashboardData.work_queue_stats?.total_completed || 0}
            </Typography>
            <Typography variant="body2" color="success.main">
              {dashboardData.pending_reviews?.length || 0} pending review
            </Typography>
          </CardContent>
        </Card>
        <Card>
          <CardContent>
            <Typography color="textSecondary" gutterBottom>
              External Agents
            </Typography>
            <Typography variant="h4">
              {externalAgentConnections.length}
            </Typography>
            <Typography variant="body2" color="primary.main">
              {externalAgentConnections.filter(conn => getConnectionStatus(conn) === 'online').length} online
            </Typography>
          </CardContent>
        </Card>
      </Box>

      <Grid container spacing={3}>
        {/* Active Sessions */}
        <Grid item xs={12} lg={8}>
          <Card sx={{ height: 'fit-content' }}>
            <CardHeader 
              title="Active Agent Sessions"
              action={
                <Chip 
                  label={`${dashboardData.active_sessions?.length || 0} active`}
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
                    {dashboardData.active_sessions?.map((session) => (
                      <TableRow 
                        key={session.id || Math.random()}
                        sx={{ 
                          backgroundColor: session.status === 'waiting_for_help' 
                            ? theme.palette.error.light + '20' 
                            : 'inherit'
                        }}
                      >
                        <TableCell>
                          <Box sx={{ display: 'flex', alignItems: 'center' }}>
                            <Avatar sx={{ width: 24, height: 24, mr: 1, bgcolor: getStatusColor(session.status || '') }}>
                              {getStatusIcon(session.status || '')}
                            </Avatar>
                            <Box>
                              <Typography variant="body2" fontWeight="bold">
                                {(session.session_id || 'unknown').slice(0, 8)}...
                              </Typography>
                              <Typography variant="caption" color="textSecondary">
                                {session.last_activity ? formatTimeAgo(new Date(session.last_activity)) : 'Unknown'}
                              </Typography>
                            </Box>
                          </Box>
                        </TableCell>
                        <TableCell>
                          <Chip 
                            label={session.agent_type || 'unknown'}
                            size="small"
                            variant="outlined"
                          />
                        </TableCell>
                        <TableCell>
                          <Chip
                            label={(session.status || 'unknown').replace('_', ' ')}
                            size="small"
                            sx={{
                              backgroundColor: getStatusColor(session.status || '') + '20',
                              color: getStatusColor(session.status || ''),
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
                                  disabled={!session.rdp_port}
                                >
                                  <ComputerIcon />
                                </IconButton>
                              </Tooltip>
                            )}
                            <MoonlightConnectionButton
                              sessionId={session.session_id}
                              wolfMode="lobbies"
                            />
                            <Tooltip title="View Session">
                              <IconButton 
                                size="small"
                                onClick={() => setSelectedSession(session)}
                              >
                                <OpenInNewIcon />
                              </IconButton>
                            </Tooltip>
                          </Box>
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </TableContainer>
              {(!dashboardData.active_sessions || dashboardData.active_sessions.length === 0) && (
                <Typography textAlign="center" color="textSecondary" sx={{ py: 4 }}>
                  No active agent sessions
                </Typography>
              )}
            </CardContent>
          </Card>
        </Grid>

        {/* External Agents & Help Requests */}
        <Grid item xs={12} lg={4}>
          <Grid container spacing={2}>
            {/* External Agent Connections */}
            <Grid item xs={12}>
              <Card>
                <CardHeader
                  title="External Agents"
                  action={
                    <Badge badgeContent={externalAgentConnections.length} color="primary">
                      <ComputerIcon />
                    </Badge>
                  }
                />
                <CardContent sx={{ maxHeight: 200, overflow: 'auto' }}>
                  {externalAgentConnections.length > 0 ? (
                    <List dense>
                      {externalAgentConnections.map((connection) => (
                        <ListItem
                          key={connection.session_id}
                          sx={{
                            border: 1,
                            borderColor: `${getConnectionStatusColor(getConnectionStatus(connection))}.main`,
                            borderRadius: 1,
                            mb: 1,
                            backgroundColor: `${getConnectionStatusColor(getConnectionStatus(connection))}.light` + '10'
                          }}
                        >
                          <ListItemIcon>
                            <ComputerIcon color={getConnectionStatusColor(getConnectionStatus(connection))} />
                          </ListItemIcon>
                          <ListItemText
                            primary={`External Agent ${connection.session_id.slice(0, 8)}...`}
                            secondary={
                              <Box>
                                <Typography variant="caption" display="block">
                                  Status: <Chip 
                                    label={getConnectionStatus(connection)} 
                                    size="small" 
                                    color={getConnectionStatusColor(getConnectionStatus(connection))}
                                  />
                                </Typography>
                                <Typography variant="caption" display="block">
                                  Connected: {connection.connected_at ? formatTimeAgo(new Date(connection.connected_at)) : 'Unknown'}
                                </Typography>
                                <Typography variant="caption" display="block">
                                  Last ping: {connection.last_ping ? formatTimeAgo(new Date(connection.last_ping)) : 'Unknown'}
                                </Typography>
                              </Box>
                            }
                          />
                          <Tooltip title="View Desktop">
                            <IconButton
                              size="small" 
                              onClick={() => openRunnerWebRDP(connection.session_id)}
                              sx={{ 
                                backgroundColor: theme.palette.success.light + '20',
                                '&:hover': {
                                  backgroundColor: theme.palette.success.light + '40'
                                }
                              }}
                            >
                              <VisibilityIcon fontSize="small" />
                            </IconButton>
                          </Tooltip>
                          <Box sx={{ ml: 1 }}>
                            <MoonlightConnectionButton
                              sessionId={connection.session_id}
                              wolfMode="lobbies"
                            />
                          </Box>
                        </ListItem>
                      ))}
                    </List>
                  ) : (
                    <Box sx={{ textAlign: 'center', py: 2 }}>
                      <Typography color="textSecondary" variant="body2">
                        No external agents connected
                      </Typography>
                      <Typography variant="caption" color="textSecondary" sx={{ display: 'block', mt: 1 }}>
                        Zed instances connected via WebSocket will appear here
                      </Typography>
                    </Box>
                  )}
                </CardContent>
              </Card>
            </Grid>
            {/* Help Requests */}
            <Grid item xs={12}>
              <Card>
                <CardHeader 
                  title="Help Requests"
                  action={
                    <Badge badgeContent={dashboardData.active_help_requests?.length || 0} color="error">
                      <HelpIcon />
                    </Badge>
                  }
                />
                <CardContent sx={{ maxHeight: 300, overflow: 'auto' }}>
                  <List dense>
                    {dashboardData.active_help_requests?.map((request) => (
                      <ListItem
                        key={request.id || Math.random()}
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
                          primary={(request.help_type || 'unknown').replace('_', ' ')}
                          secondary={
                            <>
                              <Typography variant="caption" display="block">
                                {(request.context || 'No context').slice(0, 50)}...
                              </Typography>
                              <Chip 
                                label={request.urgency || 'medium'}
                                size="small"
                                color={request.urgency === 'critical' ? 'error' : request.urgency === 'high' ? 'warning' : 'info'}
                              />
                            </>
                          }
                        />
                      </ListItem>
                    ))}
                  </List>
                  {(!dashboardData.active_help_requests || dashboardData.active_help_requests.length === 0) && (
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
                    <Badge badgeContent={dashboardData.pending_reviews?.length || 0} color="success">
                      <CheckCircleIcon />
                    </Badge>
                  }
                />
                <CardContent sx={{ maxHeight: 300, overflow: 'auto' }}>
                  <List dense>
                    {dashboardData.recent_completions?.slice(0, 5).map((completion) => (
                      <ListItem key={completion.id || Math.random()}>
                        <ListItemIcon>
                          <CheckCircleIcon color="success" />
                        </ListItemIcon>
                        <ListItemText
                          primary={(completion.summary || 'No summary').slice(0, 40) + '...'}
                          secondary={
                            <Box>
                              <Typography variant="caption" display="block">
                                {completion.created_at ? formatTimeAgo(new Date(completion.created_at)) : 'Unknown time'}
                              </Typography>
                              <Box sx={{ display: 'flex', gap: 0.5, mt: 0.5 }}>
                                <Chip 
                                  label={completion.confidence || 'unknown'}
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
                  {(!dashboardData.recent_completions || dashboardData.recent_completions.length === 0) && (
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
                    {dashboardData.pending_work?.map((item) => (
                      <TableRow 
                        key={item.id || Math.random()}
                        sx={{
                          backgroundColor: getStatusColor(item.status || '') + '10',
                          borderLeft: `4px solid ${getStatusColor(item.status || '')}`,
                        }}
                      >
                        <TableCell>
                          <Chip
                            label={item.priority || 5}
                            size="small"
                            sx={{
                              backgroundColor: getPriorityColor(item.priority || 5) + '20',
                              color: getPriorityColor(item.priority || 5)
                            }}
                          />
                        </TableCell>
                        <TableCell>
                          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 1 }}>
                            {getStatusIcon(item.status || '')}
                            <Typography variant="body2" fontWeight="bold">
                              {item.name || 'Untitled'}
                            </Typography>
                          </Box>
                          
                          {/* Phase indicator */}
                          <Box sx={{ mb: 1 }}>
                            <Chip 
                              label={getPhaseLabel(item.status || '')}
                              size="small"
                              sx={{
                                backgroundColor: getStatusColor(item.status || ''),
                                color: 'white',
                                fontWeight: 'bold'
                              }}
                            />
                          </Box>

                          {/* Original prompt */}
                          <Typography variant="caption" color="textSecondary" sx={{ display: 'block', mb: 1 }}>
                            <strong>Original:</strong> {(item.description || 'No description').slice(0, 80)}...
                          </Typography>

                          {/* Show specs when available - using status to determine */}
                          {(item.status === 'spec_review' || item.status === 'spec_revision' || 
                            item.status === 'spec_approved' || item.status === 'implementation_queued' ||
                            item.status === 'implementation' || item.status === 'implementation_review' ||
                            item.status === 'done') && (
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
                            item.status === 'done') && (
                            <Box sx={{ mt: 1, p: 1, backgroundColor: theme.palette.success.light, borderRadius: 1 }}>
                              <Typography variant="caption" fontWeight="bold" color="success.dark">
                                Implementation in Progress
                              </Typography>
                              <Typography variant="caption" sx={{ display: 'block' }}>
                                Status: {item.status}
                              </Typography>
                            </Box>
                          )}
                        </TableCell>
                        <TableCell>
                          <Chip label={item.source || 'unknown'} size="small" variant="outlined" />
                        </TableCell>
                        <TableCell>
                          <Typography variant="caption">
                            {item.agent_type || 'unknown'}
                            {/* Agent info if available */}
                          </Typography>
                        </TableCell>
                        <TableCell>
                          <Typography variant="caption">
                            {item.created_at ? formatTimeAgo(new Date(item.created_at)) : 'Unknown'}
                          </Typography>
                          {/* Revision count if available */}
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </TableContainer>
              {(!dashboardData.pending_work || dashboardData.pending_work.length === 0) && (
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
                    {dashboardData.running_work?.map((item) => {
                      const startTime = item.started_at ? new Date(item.started_at) : null
                      const duration = startTime ? formatTimeAgo(startTime) : 'Unknown'
                      
                      return (
                        <TableRow 
                          key={item.id || Math.random()}
                          sx={{
                            backgroundColor: getStatusColor(item.status || '') + '10',
                            borderLeft: `4px solid ${getStatusColor(item.status || '')}`,
                          }}
                        >
                          <TableCell>
                            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 1 }}>
                              {getStatusIcon(item.status || '')}
                              <Typography variant="body2" fontWeight="bold">
                                {item.name || 'Untitled'}
                              </Typography>
                            </Box>
                            
                            {/* Phase indicator */}
                            <Box sx={{ mb: 1 }}>
                              <Chip 
                                label={getPhaseLabel(item.status || '')}
                                size="small"
                                sx={{
                                  backgroundColor: getStatusColor(item.status || ''),
                                  color: 'white',
                                  fontWeight: 'bold'
                                }}
                              />
                            </Box>

                            <Typography variant="caption" color="textSecondary">
                              {(item.description || 'No description').slice(0, 60)}...
                            </Typography>

                            {/* Show current phase status */}
                            {item.status === 'spec_generation' && (
                              <Box sx={{ mt: 1, p: 1, backgroundColor: theme.palette.warning.light, borderRadius: 1 }}>
                                <Typography variant="caption" fontWeight="bold" color="warning.dark">
                                  🤖 Generating specifications
                                </Typography>
                              </Box>
                            )}

                            {item.status === 'implementation' && (
                              <Box sx={{ mt: 1, p: 1, backgroundColor: theme.palette.success.light, borderRadius: 1 }}>
                                <Typography variant="caption" fontWeight="bold" color="success.dark">
                                  💻 Coding in progress
                                </Typography>
                              </Box>
                            )}
                          </TableCell>
                          <TableCell>
                            {/* Show session ID if available */}
                            {item.assigned_session_id ? (
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
                            {/* Show additional status info if needed */}
                          </TableCell>
                          <TableCell>
                            <Typography variant="caption">
                              {duration}
                            </Typography>
                            {/* Show revision count if available */}
                          </TableCell>
                        </TableRow>
                      )
                    })}
                  </TableBody>
                </Table>
              </TableContainer>
              {(!dashboardData.running_work || dashboardData.running_work.length === 0) && (
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
              label={helpRequestOpen.urgency || 'medium'}
              size="small"
              color={helpRequestOpen.urgency === 'critical' ? 'error' : helpRequestOpen.urgency === 'high' ? 'warning' : 'info'}
              sx={{ ml: 2 }}
            />
          </DialogTitle>
          <DialogContent>
            <Box sx={{ mb: 2 }}>
              <Typography variant="subtitle2" gutterBottom>Context:</Typography>
              <Typography variant="body2" sx={{ mb: 2 }}>{helpRequestOpen.context || 'No context provided'}</Typography>
              
              <Typography variant="subtitle2" gutterBottom>Specific Need:</Typography>
              <Typography variant="body2" sx={{ mb: 2 }}>{helpRequestOpen.specific_need || 'No specific need provided'}</Typography>
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
                if (resolution && helpRequestOpen.id) {
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

            {/* Demo repo toggle */}
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 2 }}>
              <Typography variant="body2">Use demo repository:</Typography>
              <Button
                variant={useDemoRepo ? 'contained' : 'outlined'}
                size="small"
                onClick={() => setUseDemoRepo(!useDemoRepo)}
              >
                {useDemoRepo ? 'Yes' : 'No'}
              </Button>
            </Box>

            {/* Project or Demo repo selection */}
            {useDemoRepo ? (
              <FormControl fullWidth>
                <InputLabel>Demo Repository</InputLabel>
                <Select
                  value={selectedDemoRepo}
                  onChange={(e) => setSelectedDemoRepo(e.target.value)}
                  label="Demo Repository"
                >
                  <MenuItem value="nodejs-todo">Node.js Todo App</MenuItem>
                  <MenuItem value="python-api">Python FastAPI Service</MenuItem>
                  <MenuItem value="react-dashboard">React Admin Dashboard</MenuItem>
                  <MenuItem value="linkedin-outreach">LinkedIn Outreach Campaign</MenuItem>
                  <MenuItem value="helix-blog-posts">Helix Blog Posts Project</MenuItem>
                  <MenuItem value="empty">Empty Project</MenuItem>
                </Select>
              </FormControl>
            ) : (
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
            )}

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
            disabled={createTaskLoading || !newTaskPrompt.trim() || (!useDemoRepo && !selectedProjectId) || (useDemoRepo && !selectedDemoRepo)}
          >
            {createTaskLoading ? 'Creating...' : 'Start Spec-Driven Development'}
          </Button>
        </DialogActions>
      </Dialog>



      {/* Web RDP Viewer Modal */}
      <Dialog
        open={!!runnerRDPViewerOpen}
        onClose={() => setRunnerRDPViewerOpen(null)}
        maxWidth="lg"
        fullWidth
        PaperProps={{
          sx: { height: '80vh' }
        }}
      >
        <DialogTitle>
          External Agent Desktop
          <Button
            onClick={() => setRunnerRDPViewerOpen(null)}
            sx={{ position: 'absolute', right: 8, top: 8 }}
          >
            ✕
          </Button>
        </DialogTitle>
        <DialogContent sx={{ p: 0, height: '100%' }}>
          {runnerRDPViewerOpen && (
            <ScreenshotViewer
              sessionId={runnerRDPViewerOpen}
              isRunner={true}
              onConnectionChange={(connected) => {
                console.log('External agent desktop connection status:', connected);
              }}
              onError={(error) => {
                console.error('External agent desktop error:', error);
              }}
              width={1024}
              height={768}
              className="web-rdp-viewer"
            />
          )}
        </DialogContent>
      </Dialog>

      {/* Last updated indicator */}
      <Box sx={{ mt: 3, textAlign: 'center' }}>
        <Typography variant="caption" color="textSecondary">
          Last updated: {dashboardData.last_updated ? formatTimeAgo(new Date(dashboardData.last_updated)) : 'Unknown'}
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