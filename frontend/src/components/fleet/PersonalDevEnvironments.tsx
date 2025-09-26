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

import useApi from '../../hooks/useApi'
import useAccount from '../../hooks/useAccount'
import { IApp, AGENT_TYPE_ZED_EXTERNAL } from '../../types'
import MoonlightPairingOverlay from './MoonlightPairingOverlay'

// Personal dev environment types
interface PersonalDevEnvironment {
  instance_id: string
  user_id: string
  app_id: string
  environment_name: string
  status: string
  created_at: string
  last_activity: string
  stream_url?: string
  configured_tools: string[]
  data_sources: string[]
}

interface PersonalDevEnvironmentsProps {
  apps: IApp[]
}

const PersonalDevEnvironments: FC<PersonalDevEnvironmentsProps> = ({ apps }) => {
  const theme = useTheme()
  const api = useApi()
  const account = useAccount()

  const [environments, setEnvironments] = useState<PersonalDevEnvironment[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [createDialogOpen, setCreateDialogOpen] = useState(false)
  const [newEnvironmentName, setNewEnvironmentName] = useState('')
  const [selectedAppId, setSelectedAppId] = useState('')
  const [description, setDescription] = useState('')
  const [actionMenuAnchor, setActionMenuAnchor] = useState<null | HTMLElement>(null)
  const [selectedEnvironment, setSelectedEnvironment] = useState<PersonalDevEnvironment | null>(null)
  const [pairingOverlayOpen, setPairingOverlayOpen] = useState(false)
  const [createAgentDialogOpen, setCreateAgentDialogOpen] = useState(false)
  const [newAgentName, setNewAgentName] = useState('')

  // Filter apps to only show those with zed_external agent type
  const zedAgentApps = apps.filter(app =>
    app.config?.helix?.default_agent_type === AGENT_TYPE_ZED_EXTERNAL
  )

  const loadEnvironments = async () => {
    try {
      setLoading(true)
      setError(null)

      const response = await api.get('/api/v1/personal-dev-environments')
      setEnvironments(response.data || [])
    } catch (err: any) {
      console.error('Failed to load personal dev environments:', err)
      setError(err.message || 'Failed to load personal dev environments')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    if (account.user) {
      loadEnvironments()
    }
  }, [account.user])

  const handleCreateEnvironment = async () => {
    if (!newEnvironmentName.trim() || !selectedAppId) {
      return
    }

    try {
      await api.post('/api/v1/personal-dev-environments', {
        environment_name: newEnvironmentName,
        app_id: selectedAppId,
        description: description,
      })

      setCreateDialogOpen(false)
      setNewEnvironmentName('')
      setSelectedAppId('')
      setDescription('')
      await loadEnvironments()
    } catch (err: any) {
      console.error('Failed to create personal dev environment:', err)
      setError(err.message || 'Failed to create environment')
    }
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

  const handleStartEnvironment = async (environmentId: string) => {
    try {
      await api.post(`/api/v1/personal-dev-environments/${environmentId}/start`)
      await loadEnvironments()
    } catch (err: any) {
      console.error('Failed to start environment:', err)
      setError(err.message || 'Failed to start environment')
    }
  }

  const handleStopEnvironment = async (environmentId: string) => {
    try {
      await api.post(`/api/v1/personal-dev-environments/${environmentId}/stop`)
      await loadEnvironments()
    } catch (err: any) {
      console.error('Failed to stop environment:', err)
      setError(err.message || 'Failed to stop environment')
    }
  }

  const handleDeleteEnvironment = async (environmentId: string) => {
    if (!confirm('Are you sure you want to delete this personal dev environment?')) {
      return
    }

    try {
      await api.delete(`/api/v1/personal-dev-environments/${environmentId}`)
      await loadEnvironments()
    } catch (err: any) {
      console.error('Failed to delete environment:', err)
      setError(err.message || 'Failed to delete environment')
    }
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
          <IconButton onClick={loadEnvironments} disabled={loading}>
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
          {error}
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
              const app = apps.find(a => a.id === environment.app_id)
              return (
                <Grid item xs={12} md={6} lg={4} key={environment.instance_id}>
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
                        Created: {new Date(environment.created_at).toLocaleDateString()}
                      </Typography>
                      
                      <Typography variant="body2" color="text.secondary" gutterBottom>
                        Last Activity: {new Date(environment.last_activity).toLocaleDateString()}
                      </Typography>

                      {environment.configured_tools.length > 0 && (
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
                            onClick={() => handleStopEnvironment(environment.instance_id)}
                          >
                            Stop
                          </Button>
                        </>
                      ) : (
                        <Button
                          size="small"
                          startIcon={<PlayArrowIcon />}
                          onClick={() => handleStartEnvironment(environment.instance_id)}
                          disabled={environment.status === 'starting' || environment.status === 'creating'}
                        >
                          Start
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
            handleDeleteEnvironment(selectedEnvironment.instance_id)
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
            disabled={!selectedAppId}
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
          loadEnvironments() // Refresh environments after pairing
        }}
      />
    </Box>
  )
}

export default PersonalDevEnvironments