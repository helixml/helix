import React, { FC, useState, useEffect } from 'react'
import {
  Box,
  Typography,
  Grid,
  Card,
  CardContent,
  CardHeader,
  CardActions,
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
  Chip,
  Alert,
  LinearProgress,
  Menu,
  ListItemIcon,
  ListItemText,
  Tooltip,
  Collapse,
} from '@mui/material'
import {
  Add as AddIcon,
  PlayArrow as PlayArrowIcon,
  Stop as StopIcon,
  Delete as DeleteIcon,
  Edit as EditIcon,
  MoreVert as MoreVertIcon,
  Computer as ComputerIcon,
  OpenInNew as OpenInNewIcon,
  Refresh as RefreshIcon,
  Link as LinkIcon,
  ExpandMore as ExpandMoreIcon,
  Visibility as VisibilityIcon,
} from '@mui/icons-material'
import { useTheme } from '@mui/material/styles'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'

import useApi from '../../hooks/useApi'
import useAccount from '../../hooks/useAccount'
import { IApp, AGENT_TYPE_ZED_EXTERNAL } from '../../types'
import { ServerPersonalDevEnvironmentResponse, ServerCreatePersonalDevEnvironmentRequest } from '../../api/api'
import MoonlightPairingOverlay from './MoonlightPairingOverlay'
import ScreenshotViewer from '../external-agent/ScreenshotViewer'

interface PersonalDevEnvironmentsProps {
  apps: IApp[]
}

const PersonalDevEnvironments: FC<PersonalDevEnvironmentsProps> = ({ apps }) => {
  const theme = useTheme()
  const api = useApi()
  const account = useAccount()
  const queryClient = useQueryClient()

  const [createDialogOpen, setCreateDialogOpen] = useState(false)
  const [newEnvironmentName, setNewEnvironmentName] = useState('')
  const [selectedAppId, setSelectedAppId] = useState('')
  const [description, setDescription] = useState('')

  // Display configuration state
  const [displayPreset, setDisplayPreset] = useState('ipad') // 'ipad', 'macbook-pro', 'iphone', '4k', 'custom'
  const [customWidth, setCustomWidth] = useState(3024)
  const [customHeight, setCustomHeight] = useState(1964)
  const [customFPS, setCustomFPS] = useState(120)
  const [actionMenuAnchor, setActionMenuAnchor] = useState<null | HTMLElement>(null)
  const [selectedEnvironment, setSelectedEnvironment] = useState<ServerPersonalDevEnvironmentResponse | null>(null)
  const [pairingOverlayOpen, setPairingOverlayOpen] = useState(false)
  const [createAgentDialogOpen, setCreateAgentDialogOpen] = useState(false)
  const [newAgentName, setNewAgentName] = useState('')
  const [expandedEnvironments, setExpandedEnvironments] = useState<Set<string>>(new Set())
  const [screenshots, setScreenshots] = useState<Map<string, string>>(new Map())

  // Filter apps to only show those with zed_external agent type
  const zedAgentApps = apps.filter(app =>
    app.config?.helix?.default_agent_type === AGENT_TYPE_ZED_EXTERNAL
  )

  // Get the API client
  const apiClient = api.getApiClient()

  // Use React Query for data fetching
  const { data: environments = [], isLoading: loading, error } = useQuery({
    queryKey: ['personal-dev-environments'],
    queryFn: () => apiClient.v1PersonalDevEnvironmentsList(),
    select: (response) => response.data || [],
    enabled: !!account.user
  })

  // Poll screenshots for running environments every second
  useEffect(() => {
    const fetchScreenshots = async () => {
      const runningEnvironments = environments.filter(env =>
        env.status === 'running' || env.status === 'starting'
      )

      for (const env of runningEnvironments) {
        try {
          // Use fetch to get screenshot as blob
          const response = await fetch(`/api/v1/personal-dev-environments/${env.instanceID}/screenshot`, {
            credentials: 'include' // Include cookies for authentication
          })

          if (!response.ok) {
            throw new Error(`HTTP ${response.status}`)
          }

          // Create object URL from blob
          const blob = await response.blob()
          const url = URL.createObjectURL(blob)

          // Update screenshots map
          setScreenshots(prev => {
            const newMap = new Map(prev)
            // Revoke old URL to prevent memory leak
            const oldUrl = newMap.get(env.instanceID || '')
            if (oldUrl) {
              URL.revokeObjectURL(oldUrl)
            }
            newMap.set(env.instanceID || '', url)
            return newMap
          })
        } catch (err) {
          // Silently fail - screenshot might not be ready yet
          console.debug(`Failed to fetch screenshot for ${env.instanceID}:`, err)
        }
      }
    }

    // Fetch immediately
    fetchScreenshots()

    // Set up interval to fetch every second
    const interval = setInterval(fetchScreenshots, 1000)

    // Cleanup
    return () => {
      clearInterval(interval)
      // Revoke all object URLs on cleanup
      screenshots.forEach(url => URL.revokeObjectURL(url))
    }
  }, [environments])

  // Display preset configurations
  const getDisplayConfig = () => {
    switch (displayPreset) {
      case 'ipad':
        return { width: 2360, height: 1640, fps: 120 }
      case 'macbook-pro':
        return { width: 3024, height: 1964, fps: 120 } // 14" MacBook Pro (16:10, no notch area)
      case 'iphone':
        return { width: 2556, height: 1179, fps: 120 } // iPhone resolution
      case '4k':
        return { width: 3840, height: 2160, fps: 120 }
      case 'custom':
        return { width: customWidth, height: customHeight, fps: customFPS }
      default:
        return { width: 2360, height: 1640, fps: 120 }
    }
  }

  // Create environment mutation
  const createEnvironmentMutation = useMutation({
    mutationFn: (request: ServerCreatePersonalDevEnvironmentRequest) =>
      apiClient.v1PersonalDevEnvironmentsCreate(request),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['personal-dev-environments'] })
      setCreateDialogOpen(false)
      setNewEnvironmentName('')
      setSelectedAppId('')
      setDescription('')
      setDisplayPreset('ipad')
      setCustomWidth(3024)
      setCustomHeight(1964)
      setCustomFPS(120)
    }
  })

  const handleCreateEnvironment = () => {
    if (!newEnvironmentName.trim() || !selectedAppId) {
      return
    }

    const displayConfig = getDisplayConfig()
    const request: ServerCreatePersonalDevEnvironmentRequest = {
      environment_name: newEnvironmentName,
      app_id: selectedAppId,
      description: description,
      display_width: displayConfig.width,
      display_height: displayConfig.height,
      display_fps: displayConfig.fps,
    }

    createEnvironmentMutation.mutate(request)
  }

  const handleCreateZedAgent = async () => {
    if (!newAgentName.trim()) {
      return
    }

    try {
      const response = await api.post('/api/v1/apps', {
        name: newAgentName,
        description: 'Zed external agent for personal development environments',
        global: false,
        config: {
          helix: {
            name: newAgentName,
            description: 'Zed external agent for personal development environments',
            default_agent_type: AGENT_TYPE_ZED_EXTERNAL,
            assistants: [{
              id: '1',
              name: 'Default Assistant',
              description: 'Default assistant for Zed external agent',
              agent_type: AGENT_TYPE_ZED_EXTERNAL,
              model: 'claude-3-sonnet',
              provider: 'anthropic',
              system_prompt: 'You are a helpful AI assistant integrated with the Zed code editor.',
            }]
          },
          secrets: {},
          allowed_domains: []
        }
      })

      setCreateAgentDialogOpen(false)
      setNewAgentName('')

      // Auto-select the newly created agent
      setSelectedAppId(response.data.id)

      // Refresh the parent component to get the new app
      window.location.reload()
    } catch (err: any) {
      console.error('Failed to create Zed agent:', err)
      setError(err.message || 'Failed to create Zed agent')
    }
  }

  // Start environment mutation
  const startEnvironmentMutation = useMutation({
    mutationFn: (environmentId: string) =>
      apiClient.v1PersonalDevEnvironmentsStartCreate(environmentId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['personal-dev-environments'] })
    }
  })

  // Stop environment mutation
  const stopEnvironmentMutation = useMutation({
    mutationFn: (environmentId: string) =>
      apiClient.v1PersonalDevEnvironmentsStopCreate(environmentId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['personal-dev-environments'] })
    }
  })

  // Delete environment mutation
  const deleteEnvironmentMutation = useMutation({
    mutationFn: (environmentId: string) =>
      apiClient.v1PersonalDevEnvironmentsDelete(environmentId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['personal-dev-environments'] })
    }
  })

  const handleStartEnvironment = (environmentId: string) => {
    startEnvironmentMutation.mutate(environmentId)
  }

  const handleStopEnvironment = (environmentId: string) => {
    stopEnvironmentMutation.mutate(environmentId)
  }

  const handleDeleteEnvironment = (environmentId: string) => {
    if (!confirm('Are you sure you want to delete this personal dev environment?')) {
      return
    }
    deleteEnvironmentMutation.mutate(environmentId)
  }

  const handleConnectToEnvironment = (environment: PersonalDevEnvironment) => {
    if (environment.stream_url) {
      window.open(environment.stream_url, '_blank')
    }
  }

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'running':
        return 'success'
      case 'stopped':
        return 'default'
      case 'starting':
      case 'creating':
        return 'warning'
      case 'error':
        return 'error'
      default:
        return 'default'
    }
  }

  const getStatusIcon = (status: string) => {
    switch (status) {
      case 'running':
        return <PlayArrowIcon />
      case 'stopped':
        return <StopIcon />
      case 'starting':
      case 'creating':
        return <LinearProgress />
      default:
        return <ComputerIcon />
    }
  }

  const handleActionMenuOpen = (event: React.MouseEvent<HTMLElement>, environment: PersonalDevEnvironment) => {
    setActionMenuAnchor(event.currentTarget)
    setSelectedEnvironment(environment)
  }

  const handleActionMenuClose = () => {
    setActionMenuAnchor(null)
    setSelectedEnvironment(null)
  }

  const toggleEnvironmentExpansion = (environmentId: string) => {
    setExpandedEnvironments(prev => {
      const newSet = new Set(prev)
      if (newSet.has(environmentId)) {
        newSet.delete(environmentId)
      } else {
        newSet.add(environmentId)
      }
      return newSet
    })
  }

  return (
    <Box>
      <Box display="flex" justifyContent="space-between" alignItems="center" mb={3}>
        <Typography variant="h5" component="h2">
          Personal Dev Environments
        </Typography>
        <Box>
          <IconButton onClick={() => queryClient.invalidateQueries({ queryKey: ['personal-dev-environments'] })} disabled={loading}>
            <RefreshIcon />
          </IconButton>
          <Button
            variant="outlined"
            startIcon={<LinkIcon />}
            onClick={() => setPairingOverlayOpen(true)}
            sx={{ ml: 1 }}
          >
            Pair Moonlight Client
          </Button>
          <Button
            variant="contained"
            startIcon={<AddIcon />}
            onClick={() => setCreateDialogOpen(true)}
            sx={{ ml: 1 }}
          >
            Create Environment
          </Button>
        </Box>
      </Box>

      {error && (
        <Alert severity="error" sx={{ mb: 2 }}>
          {error instanceof Error ? error.message : 'Failed to load personal dev environments'}
        </Alert>
      )}

      {createEnvironmentMutation.error && (
        <Alert severity="error" sx={{ mb: 2 }}>
          {createEnvironmentMutation.error instanceof Error
            ? createEnvironmentMutation.error.message
            : 'Failed to create environment'}
        </Alert>
      )}

      {startEnvironmentMutation.error && (
        <Alert severity="error" sx={{ mb: 2 }}>
          {startEnvironmentMutation.error instanceof Error
            ? startEnvironmentMutation.error.message
            : 'Failed to start environment'}
        </Alert>
      )}

      {stopEnvironmentMutation.error && (
        <Alert severity="error" sx={{ mb: 2 }}>
          {stopEnvironmentMutation.error instanceof Error
            ? stopEnvironmentMutation.error.message
            : 'Failed to stop environment'}
        </Alert>
      )}

      {deleteEnvironmentMutation.error && (
        <Alert severity="error" sx={{ mb: 2 }}>
          {deleteEnvironmentMutation.error instanceof Error
            ? deleteEnvironmentMutation.error.message
            : 'Failed to delete environment'}
        </Alert>
      )}

      {loading ? (
        <LinearProgress sx={{ mb: 2 }} />
      ) : (
        <Grid container spacing={3}>
          {environments.length === 0 ? (
            <Grid item xs={12}>
              <Card>
                <CardContent sx={{ textAlign: 'center', py: 4 }}>
                  <ComputerIcon sx={{ fontSize: 48, color: 'text.secondary', mb: 2 }} />
                  <Typography variant="h6" color="text.secondary" gutterBottom>
                    No Personal Dev Environments
                  </Typography>
                  <Typography variant="body2" color="text.secondary" sx={{ mb: 3 }}>
                    Create your first personal development environment to get started with coding.
                  </Typography>
                  <Button
                    variant="contained"
                    startIcon={<AddIcon />}
                    onClick={() => setCreateDialogOpen(true)}
                  >
                    Create Your First Environment
                  </Button>
                </CardContent>
              </Card>
            </Grid>
          ) : (
            environments.map((environment) => {
              const app = apps.find(a => a.id === environment.appID)
              return (
                <Grid item xs={12} md={6} lg={4} key={environment.instanceID}>
                  <Card>
                    <CardHeader
                      title={environment.environment_name}
                      subheader={app?.config?.helix?.name || app?.name || 'Unknown Agent'}
                      action={
                        <IconButton onClick={(e) => handleActionMenuOpen(e, environment)}>
                          <MoreVertIcon />
                        </IconButton>
                      }
                    />
                    <CardContent>
                      {/* Screenshot Thumbnail */}
                      {screenshots.get(environment.instanceID || '') && (
                        <Box
                          sx={{
                            width: '100%',
                            height: 150,
                            mb: 2,
                            borderRadius: 1,
                            overflow: 'hidden',
                            backgroundColor: '#000',
                            display: 'flex',
                            alignItems: 'center',
                            justifyContent: 'center'
                          }}
                        >
                          <img
                            src={screenshots.get(environment.instanceID || '')}
                            alt={`Screenshot of ${environment.environment_name}`}
                            style={{
                              width: '100%',
                              height: '100%',
                              objectFit: 'contain'
                            }}
                          />
                        </Box>
                      )}

                      <Box display="flex" alignItems="center" mb={2}>
                        <Chip
                          icon={getStatusIcon(environment.status)}
                          label={environment.status}
                          color={getStatusColor(environment.status) as any}
                          size="small"
                        />
                      </Box>
                      
                      <Typography variant="body2" color="text.secondary" gutterBottom>
                        Created: {new Date(environment.createdAt || '').toLocaleDateString()}
                      </Typography>

                      <Typography variant="body2" color="text.secondary" gutterBottom>
                        Last Activity: {new Date(environment.lastActivity || '').toLocaleDateString()}
                      </Typography>

                      {/* Display Configuration */}
                      {(environment.display_width && environment.display_height) && (
                        <Typography variant="body2" color="text.secondary" gutterBottom>
                          Display: {environment.display_width}×{environment.display_height} @ {environment.display_fps || 60}fps
                        </Typography>
                      )}

                      {/* Lobby PIN Display - Only show to environment owner or admin */}
                      {(environment.userID === account.user?.id || account.admin) && environment.wolf_lobby_pin && (
                        <Box sx={{
                          mt: 2,
                          p: 1.5,
                          bgcolor: 'primary.dark',
                          borderRadius: 1,
                          border: '1px solid',
                          borderColor: 'primary.main'
                        }}>
                          <Typography variant="caption" color="primary.light" sx={{ fontWeight: 'bold' }}>
                            🔐 Moonlight PIN
                          </Typography>
                          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mt: 0.5 }}>
                            <Typography
                              variant="h6"
                              sx={{
                                fontFamily: 'monospace',
                                letterSpacing: 4,
                                color: 'primary.light'
                              }}
                            >
                              {environment.wolf_lobby_pin}
                            </Typography>
                            <IconButton
                              size="small"
                              onClick={() => {
                                navigator.clipboard.writeText(environment.wolf_lobby_pin || '')
                              }}
                              sx={{ color: 'primary.light' }}
                            >
                              <LinkIcon fontSize="small" />
                            </IconButton>
                          </Box>
                          <Typography variant="caption" color="text.secondary" sx={{ fontSize: '0.65rem' }}>
                            Use in Wolf UI to join
                          </Typography>
                        </Box>
                      )}

                      {environment.configured_tools && environment.configured_tools.length > 0 && (
                        <Box mt={2}>
                          <Typography variant="caption" color="text.secondary">
                            Tools:
                          </Typography>
                          <Box display="flex" flexWrap="wrap" gap={0.5} mt={0.5}>
                            {environment.configured_tools.map((tool) => (
                              <Chip key={tool} label={tool} size="small" variant="outlined" />
                            ))}
                          </Box>
                        </Box>
                      )}
                    </CardContent>
                    
                    <CardActions>
                      {environment.status === 'running' ? (
                        <>
                          <Button
                            size="small"
                            startIcon={<VisibilityIcon />}
                            onClick={() => toggleEnvironmentExpansion(environment.instanceID || '')}
                          >
                            {expandedEnvironments.has(environment.instanceID || '') ? 'Hide VNC' : 'Show VNC'}
                          </Button>
                          <Button
                            size="small"
                            startIcon={<OpenInNewIcon />}
                            onClick={() => handleConnectToEnvironment(environment)}
                            disabled={!environment.stream_url}
                          >
                            Moonlight
                          </Button>
                          <Button
                            size="small"
                            startIcon={<StopIcon />}
                            onClick={() => handleStopEnvironment(environment.instanceID || '')}
                            disabled={stopEnvironmentMutation.isPending}
                          >
                            {stopEnvironmentMutation.isPending ? 'Stopping...' : 'Stop'}
                          </Button>
                        </>
                      ) : (
                        <Button
                          size="small"
                          startIcon={<PlayArrowIcon />}
                          onClick={() => handleStartEnvironment(environment.instanceID || '')}
                          disabled={environment.status === 'starting' || environment.status === 'creating' || startEnvironmentMutation.isPending}
                        >
                          {startEnvironmentMutation.isPending ? 'Starting...' : 'Start'}
                        </Button>
                      )}
                    </CardActions>

                    {/* VNC Viewer - Collapsible */}
                    <Collapse in={expandedEnvironments.has(environment.instanceID || '')} timeout="auto" unmountOnExit>
                      <Box sx={{ p: 2, pt: 0, height: 600, backgroundColor: '#000' }}>
                        <ScreenshotViewer
                          sessionId={environment.instanceID || ''}
                          isPersonalDevEnvironment={true}
                          wolfLobbyId={environment.wolf_lobby_id}
                          enableStreaming={true}
                          width={environment.display_width || 1920}
                          height={environment.display_height || 1080}
                        />
                      </Box>
                    </Collapse>
                  </Card>
                </Grid>
              )
            })
          )}
        </Grid>
      )}

      {/* Action Menu */}
      <Menu
        anchorEl={actionMenuAnchor}
        open={Boolean(actionMenuAnchor)}
        onClose={handleActionMenuClose}
      >
        <MenuItem onClick={() => {
          if (selectedEnvironment) {
            handleDeleteEnvironment(selectedEnvironment.instanceID || '')
          }
          handleActionMenuClose()
        }}>
          <ListItemIcon>
            <DeleteIcon />
          </ListItemIcon>
          <ListItemText>Delete Environment</ListItemText>
        </MenuItem>
      </Menu>

      {/* Create Environment Dialog */}
      <Dialog open={createDialogOpen} onClose={() => setCreateDialogOpen(false)} maxWidth="sm" fullWidth>
        <DialogTitle>Create Personal Dev Environment</DialogTitle>
        <DialogContent>
          <TextField
            autoFocus
            margin="dense"
            label="Environment Name"
            fullWidth
            variant="outlined"
            value={newEnvironmentName}
            onChange={(e) => setNewEnvironmentName(e.target.value)}
            sx={{ mb: 2 }}
          />
          
          <FormControl fullWidth sx={{ mb: 2 }}>
            <InputLabel>Base Helix Agent</InputLabel>
            <Select
              value={selectedAppId}
              onChange={(e) => setSelectedAppId(e.target.value)}
              label="Base Helix Agent"
            >
              {zedAgentApps.map((app) => (
                <MenuItem key={app.id} value={app.id}>
                  {app.config?.helix?.name || app.name} (Zed Agent)
                </MenuItem>
              ))}
            </Select>
          </FormControl>

          {zedAgentApps.length === 0 && (
            <Alert severity="info" sx={{ mb: 2 }}>
              No Zed agents found. You need to create a Zed agent first to use personal development environments.
              <Button
                variant="outlined"
                size="small"
                startIcon={<AddIcon />}
                onClick={() => setCreateAgentDialogOpen(true)}
                sx={{ ml: 2 }}
              >
                Create Zed Agent
              </Button>
            </Alert>
          )}

          <TextField
            margin="dense"
            label="Description (Optional)"
            fullWidth
            multiline
            rows={3}
            variant="outlined"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            sx={{ mb: 3 }}
          />

          {/* Display Configuration Section */}
          <FormControl fullWidth sx={{ mb: 3 }}>
            <InputLabel>Streaming Resolution</InputLabel>
            <Select
              value={displayPreset}
              onChange={(e) => setDisplayPreset(e.target.value)}
              label="Streaming Resolution"
            >
              <MenuItem value="ipad">iPad (2360×1640 @ 120fps) - ~16:11 aspect ratio</MenuItem>
              <MenuItem value="macbook-pro">MacBook Pro (3024×1964 @ 120fps) - ~16:10 aspect ratio</MenuItem>
              <MenuItem value="iphone">iPhone (2556×1179 @ 120fps) - ~21:10 aspect ratio</MenuItem>
              <MenuItem value="4k">4K (3840×2160 @ 120fps) - 16:9 aspect ratio</MenuItem>
              <MenuItem value="custom">Custom resolution</MenuItem>
            </Select>
          </FormControl>

          {/* Custom resolution fields */}
          {displayPreset === 'custom' && (
            <Grid container spacing={2} sx={{ mt: 1 }}>
                <Grid item xs={4}>
                  <TextField
                    label="Width"
                    type="number"
                    size="small"
                    value={customWidth}
                    onChange={(e) => setCustomWidth(parseInt(e.target.value) || 1920)}
                    inputProps={{ min: 800, max: 7680 }}
                  />
                </Grid>
                <Grid item xs={4}>
                  <TextField
                    label="Height"
                    type="number"
                    size="small"
                    value={customHeight}
                    onChange={(e) => setCustomHeight(parseInt(e.target.value) || 1080)}
                    inputProps={{ min: 600, max: 4320 }}
                  />
                </Grid>
                <Grid item xs={4}>
                  <TextField
                    label="FPS"
                    type="number"
                    size="small"
                    value={customFPS}
                    onChange={(e) => setCustomFPS(parseInt(e.target.value) || 60)}
                    inputProps={{ min: 30, max: 144 }}
                  />
                </Grid>
            </Grid>
          )}

          <Alert severity="info" sx={{ mt: 2 }}>
            This will create a new personal development environment based on the selected Helix Agent's configuration.
            You'll be able to access it via Moonlight streaming with the selected resolution settings.
          </Alert>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setCreateDialogOpen(false)}>Cancel</Button>
          <Button
            onClick={handleCreateEnvironment}
            disabled={!selectedAppId || createEnvironmentMutation.isPending}
            variant="contained"
          >
            Create Environment
          </Button>
        </DialogActions>
      </Dialog>

      {/* Create Zed Agent Dialog */}
      <Dialog open={createAgentDialogOpen} onClose={() => setCreateAgentDialogOpen(false)} maxWidth="sm" fullWidth>
        <DialogTitle>Create Zed Agent</DialogTitle>
        <DialogContent>
          <TextField
            autoFocus
            margin="dense"
            label="Agent Name"
            fullWidth
            variant="outlined"
            value={newAgentName}
            onChange={(e) => setNewAgentName(e.target.value)}
            sx={{ mb: 2 }}
          />

          <Alert severity="info" sx={{ mt: 2 }}>
            This will create a new Zed external agent that can be used for personal development environments.
            The agent will be configured with default settings for code editing and development tasks.
          </Alert>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setCreateAgentDialogOpen(false)}>Cancel</Button>
          <Button
            onClick={handleCreateZedAgent}
            disabled={!newAgentName.trim()}
            variant="contained"
          >
            Create Agent
          </Button>
        </DialogActions>
      </Dialog>

      {/* Moonlight Pairing Overlay */}
      <MoonlightPairingOverlay
        open={pairingOverlayOpen}
        onClose={() => setPairingOverlayOpen(false)}
        onPairingComplete={() => {
          queryClient.invalidateQueries({ queryKey: ['personal-dev-environments'] }) // Refresh environments after pairing
        }}
      />
    </Box>
  )
}

export default PersonalDevEnvironments