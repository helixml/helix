import React, { FC, useState } from 'react'
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
} from '@mui/icons-material'
import { useTheme } from '@mui/material/styles'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'

import useApi from '../../hooks/useApi'
import useAccount from '../../hooks/useAccount'
import { IApp, AGENT_TYPE_ZED_EXTERNAL } from '../../types'
import { ServerPersonalDevEnvironmentResponse, ServerCreatePersonalDevEnvironmentRequest } from '../../api/api'
import MoonlightPairingOverlay from './MoonlightPairingOverlay'

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
  const [actionMenuAnchor, setActionMenuAnchor] = useState<null | HTMLElement>(null)
  const [selectedEnvironment, setSelectedEnvironment] = useState<ServerPersonalDevEnvironmentResponse | null>(null)
  const [pairingOverlayOpen, setPairingOverlayOpen] = useState(false)
  const [createAgentDialogOpen, setCreateAgentDialogOpen] = useState(false)
  const [newAgentName, setNewAgentName] = useState('')

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
    }
  })

  const handleCreateEnvironment = () => {
    if (!newEnvironmentName.trim() || !selectedAppId) {
      return
    }

    const request: ServerCreatePersonalDevEnvironmentRequest = {
      environment_name: newEnvironmentName,
      app_id: selectedAppId,
      description: description,
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
                            startIcon={<OpenInNewIcon />}
                            onClick={() => handleConnectToEnvironment(environment)}
                            disabled={!environment.stream_url}
                          >
                            Connect
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
          />

          <Alert severity="info" sx={{ mt: 2 }}>
            This will create a new personal development environment based on the selected Helix Agent's configuration.
            You'll be able to access it via Moonlight streaming.
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