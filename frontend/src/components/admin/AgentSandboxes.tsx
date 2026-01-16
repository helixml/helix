import React, { FC } from 'react'
import {
  Box,
  Grid,
  Paper,
  Typography,
  Card,
  CardContent,
  CardHeader,
  Chip,
  IconButton,
  Alert,
  CircularProgress,
  Tooltip,
  Avatar,
  AvatarGroup,
  LinearProgress,
} from '@mui/material'
import RefreshIcon from '@mui/icons-material/Refresh'
import MemoryIcon from '@mui/icons-material/Memory'
import ComputerIcon from '@mui/icons-material/Computer'
import DesktopWindowsIcon from '@mui/icons-material/DesktopWindows'
import PersonIcon from '@mui/icons-material/Person'
import ThermostatIcon from '@mui/icons-material/Thermostat'
import { useQuery } from '@tanstack/react-query'
import useApi from '../../hooks/useApi'

// Types matching the backend response
interface GPUInfo {
  index: number
  name: string
  vendor: string
  memory_total_bytes: number
  memory_used_bytes: number
  memory_free_bytes: number
  utilization_percent: number
  temperature_celsius: number
}

interface ClientInfo {
  id: number
  user_id: string
  user_name: string
  avatar_url?: string
  color: string
  last_x: number
  last_y: number
  last_seen: string
}

interface DevContainerWithClients {
  session_id: string
  container_id: string
  container_name: string
  status: string
  ip_address?: string
  container_type: string
  desktop_version?: string
  gpu_vendor?: string
  render_node?: string
  sandbox_id: string
  clients?: ClientInfo[]
}

interface SandboxInstanceInfo {
  id: string
  session_id: string
  status: string
  container_id?: string
}

interface AgentSandboxesDebugResponse {
  message: string
  sandboxes?: SandboxInstanceInfo[]
  gpus?: GPUInfo[]
  dev_containers?: DevContainerWithClients[]
}

const formatBytes = (bytes: number): string => {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return Math.round((bytes / Math.pow(k, i)) * 100) / 100 + ' ' + sizes[i]
}

const getStatusColor = (status: string): 'success' | 'warning' | 'error' | 'default' => {
  switch (status) {
    case 'running':
      return 'success'
    case 'starting':
      return 'warning'
    case 'stopped':
    case 'error':
      return 'error'
    default:
      return 'default'
  }
}

const getContainerTypeLabel = (type: string): string => {
  switch (type) {
    case 'sway':
      return 'Sway Desktop'
    case 'ubuntu':
      return 'Ubuntu/GNOME'
    case 'headless':
      return 'Headless'
    default:
      return type
  }
}

// GPU Stats Card Component
const GPUStatsCard: FC<{ gpus: GPUInfo[] }> = ({ gpus }) => {
  if (gpus.length === 0) return null

  return (
    <Card>
      <CardHeader
        avatar={<MemoryIcon />}
        title="GPU Statistics"
        subheader={`${gpus.length} GPU${gpus.length > 1 ? 's' : ''} available`}
      />
      <CardContent>
        <Grid container spacing={3}>
          {gpus.map((gpu) => (
            <Grid item xs={12} md={6} lg={4} key={gpu.index}>
              <Paper
                sx={{
                  p: 2,
                  backgroundColor: 'rgba(255,255,255,0.02)',
                  border: '1px solid rgba(255,255,255,0.1)',
                }}
              >
                <Typography variant="subtitle1" fontWeight="bold" gutterBottom>
                  {gpu.name}
                </Typography>
                <Typography variant="caption" color="text.secondary" display="block" gutterBottom>
                  {gpu.vendor.toUpperCase()} GPU #{gpu.index}
                </Typography>

                {/* GPU Utilization */}
                <Box sx={{ mt: 2, mb: 1 }}>
                  <Box sx={{ display: 'flex', justifyContent: 'space-between', mb: 0.5 }}>
                    <Typography variant="body2" color="text.secondary">
                      Utilization
                    </Typography>
                    <Typography variant="body2" fontWeight="bold">
                      {gpu.utilization_percent}%
                    </Typography>
                  </Box>
                  <LinearProgress
                    variant="determinate"
                    value={gpu.utilization_percent}
                    color={gpu.utilization_percent > 80 ? 'error' : gpu.utilization_percent > 50 ? 'warning' : 'success'}
                    sx={{ height: 8, borderRadius: 4 }}
                  />
                </Box>

                {/* Memory Usage */}
                <Box sx={{ mt: 2, mb: 1 }}>
                  <Box sx={{ display: 'flex', justifyContent: 'space-between', mb: 0.5 }}>
                    <Typography variant="body2" color="text.secondary">
                      VRAM
                    </Typography>
                    <Typography variant="body2" fontWeight="bold">
                      {formatBytes(gpu.memory_used_bytes)} / {formatBytes(gpu.memory_total_bytes)}
                    </Typography>
                  </Box>
                  <LinearProgress
                    variant="determinate"
                    value={(gpu.memory_used_bytes / gpu.memory_total_bytes) * 100}
                    sx={{ height: 8, borderRadius: 4 }}
                  />
                </Box>

                {/* Temperature */}
                <Box sx={{ mt: 2, display: 'flex', alignItems: 'center', gap: 1 }}>
                  <ThermostatIcon
                    fontSize="small"
                    color={gpu.temperature_celsius > 80 ? 'error' : gpu.temperature_celsius > 60 ? 'warning' : 'success'}
                  />
                  <Typography variant="body2">
                    {gpu.temperature_celsius}°C
                  </Typography>
                </Box>
              </Paper>
            </Grid>
          ))}
        </Grid>
      </CardContent>
    </Card>
  )
}

// Dev Container Card Component
const DevContainerCard: FC<{ container: DevContainerWithClients }> = ({ container }) => {
  const clients = container.clients || []

  return (
    <Paper
      sx={{
        p: 2,
        backgroundColor: container.status === 'running' ? 'rgba(76, 175, 80, 0.05)' : 'rgba(255,255,255,0.02)',
        border: '1px solid',
        borderColor: container.status === 'running' ? 'rgba(76, 175, 80, 0.3)' : 'rgba(255,255,255,0.1)',
      }}
    >
      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', mb: 2 }}>
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
          <DesktopWindowsIcon color="primary" />
          <Box>
            <Typography variant="subtitle1" fontWeight="bold">
              {container.session_id}
            </Typography>
            <Typography variant="caption" color="text.secondary">
              {getContainerTypeLabel(container.container_type)}
            </Typography>
          </Box>
        </Box>
        <Chip
          label={container.status}
          size="small"
          color={getStatusColor(container.status)}
        />
      </Box>

      <Grid container spacing={2}>
        <Grid item xs={6}>
          <Typography variant="caption" color="text.secondary">
            Sandbox
          </Typography>
          <Typography variant="body2" sx={{ fontFamily: 'monospace' }}>
            {container.sandbox_id}
          </Typography>
        </Grid>
        <Grid item xs={6}>
          <Typography variant="caption" color="text.secondary">
            GPU
          </Typography>
          <Typography variant="body2">
            {container.gpu_vendor ? container.gpu_vendor.toUpperCase() : 'None'}
          </Typography>
        </Grid>
        {container.ip_address && (
          <Grid item xs={6}>
            <Typography variant="caption" color="text.secondary">
              IP Address
            </Typography>
            <Typography variant="body2" sx={{ fontFamily: 'monospace' }}>
              {container.ip_address}
            </Typography>
          </Grid>
        )}
        {container.desktop_version && (
          <Grid item xs={6}>
            <Typography variant="caption" color="text.secondary">
              Version
            </Typography>
            <Typography variant="body2" sx={{ fontFamily: 'monospace' }}>
              {container.desktop_version.slice(0, 8)}
            </Typography>
          </Grid>
        )}
      </Grid>

      {/* Connected Users */}
      <Box sx={{ mt: 2, pt: 2, borderTop: '1px solid rgba(255,255,255,0.1)' }}>
        <Typography variant="caption" color="text.secondary" gutterBottom display="block">
          Connected Users ({clients.length})
        </Typography>
        {clients.length > 0 ? (
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, flexWrap: 'wrap', mt: 1 }}>
            <AvatarGroup max={5}>
              {clients.map((client) => (
                <Tooltip
                  key={client.id}
                  title={`${client.user_name} (${client.user_id})`}
                  arrow
                >
                  <Avatar
                    sx={{
                      width: 28,
                      height: 28,
                      bgcolor: client.color,
                      fontSize: '0.75rem',
                      border: `2px solid ${client.color}`,
                    }}
                    src={client.avatar_url}
                  >
                    {client.user_name?.charAt(0)?.toUpperCase() || <PersonIcon fontSize="small" />}
                  </Avatar>
                </Tooltip>
              ))}
            </AvatarGroup>
            {clients.map((client) => (
              <Chip
                key={client.id}
                label={client.user_name}
                size="small"
                sx={{
                  backgroundColor: client.color,
                  color: 'white',
                  fontWeight: 'bold',
                }}
              />
            ))}
          </Box>
        ) : (
          <Typography variant="body2" color="text.secondary" sx={{ fontStyle: 'italic' }}>
            No users connected
          </Typography>
        )}
      </Box>
    </Paper>
  )
}

interface AgentSandboxesProps {
  selectedSandboxId?: string
}

const AgentSandboxes: FC<AgentSandboxesProps> = ({ selectedSandboxId }) => {
  const api = useApi()
  const apiClient = api.getApiClient()

  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['agent-sandboxes-debug'],
    queryFn: async () => {
      const response = await apiClient.v1AdminAgentSandboxesDebugList({})
      return response.data as AgentSandboxesDebugResponse
    },
    refetchInterval: 5000,
  })

  const gpus = data?.gpus || []
  const devContainers = data?.dev_containers || []
  const sandboxes = data?.sandboxes || []

  // Filter by selected sandbox if specified
  const filteredContainers = selectedSandboxId
    ? devContainers.filter((c) => c.sandbox_id === selectedSandboxId)
    : devContainers

  const runningSandboxes = sandboxes.filter((s) => s.status === 'running').length
  const runningContainers = devContainers.filter((c) => c.status === 'running').length
  const totalClients = devContainers.reduce((sum, c) => sum + (c.clients?.length || 0), 0)

  return (
    <Box sx={{ p: 3 }}>
      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 3 }}>
        <Box>
          <Typography variant="h5">Agent Sandboxes</Typography>
          <Typography variant="body2" color="text.secondary">
            Hydra-managed dev containers with WebSocket video streaming
          </Typography>
        </Box>
        <IconButton onClick={() => refetch()} disabled={isLoading} sx={{ color: 'primary.main' }}>
          {isLoading ? <CircularProgress size={24} /> : <RefreshIcon />}
        </IconButton>
      </Box>

      {error && (
        <Alert severity="error" sx={{ mb: 3 }}>
          {error instanceof Error ? error.message : 'Failed to fetch sandbox data'}
        </Alert>
      )}

      {/* Summary Stats */}
      <Grid container spacing={2} sx={{ mb: 3 }}>
        <Grid item xs={12} sm={4}>
          <Paper sx={{ p: 2, textAlign: 'center' }}>
            <ComputerIcon sx={{ fontSize: 40, color: 'primary.main', mb: 1 }} />
            <Typography variant="h4">{runningSandboxes}</Typography>
            <Typography variant="body2" color="text.secondary">
              Running Sandboxes
            </Typography>
          </Paper>
        </Grid>
        <Grid item xs={12} sm={4}>
          <Paper sx={{ p: 2, textAlign: 'center' }}>
            <DesktopWindowsIcon sx={{ fontSize: 40, color: 'success.main', mb: 1 }} />
            <Typography variant="h4">{runningContainers}</Typography>
            <Typography variant="body2" color="text.secondary">
              Dev Containers
            </Typography>
          </Paper>
        </Grid>
        <Grid item xs={12} sm={4}>
          <Paper sx={{ p: 2, textAlign: 'center' }}>
            <PersonIcon sx={{ fontSize: 40, color: 'info.main', mb: 1 }} />
            <Typography variant="h4">{totalClients}</Typography>
            <Typography variant="body2" color="text.secondary">
              Connected Users
            </Typography>
          </Paper>
        </Grid>
      </Grid>

      <Grid container spacing={3}>
        {/* GPU Stats */}
        {gpus.length > 0 && (
          <Grid item xs={12}>
            <GPUStatsCard gpus={gpus} />
          </Grid>
        )}

        {/* Dev Containers */}
        <Grid item xs={12}>
          <Card>
            <CardHeader
              avatar={<DesktopWindowsIcon />}
              title="Dev Containers"
              subheader={`${filteredContainers.length} container${filteredContainers.length !== 1 ? 's' : ''}`}
            />
            <CardContent>
              {filteredContainers.length > 0 ? (
                <Grid container spacing={2}>
                  {filteredContainers.map((container) => (
                    <Grid item xs={12} md={6} lg={4} key={`${container.sandbox_id}-${container.session_id}`}>
                      <DevContainerCard container={container} />
                    </Grid>
                  ))}
                </Grid>
              ) : (
                <Box sx={{ textAlign: 'center', py: 4 }}>
                  <DesktopWindowsIcon sx={{ fontSize: 60, color: 'text.secondary', mb: 2 }} />
                  <Typography variant="body1" color="text.secondary">
                    No dev containers running
                  </Typography>
                  <Typography variant="body2" color="text.secondary">
                    Start a task to launch a desktop sandbox
                  </Typography>
                </Box>
              )}
            </CardContent>
          </Card>
        </Grid>

        {/* Sandboxes List */}
        {sandboxes.length > 0 && (
          <Grid item xs={12}>
            <Card>
              <CardHeader
                avatar={<ComputerIcon />}
                title="Sandbox Instances"
                subheader="Registered sandboxes with Hydra"
              />
              <CardContent>
                <Grid container spacing={2}>
                  {sandboxes.map((sandbox) => (
                    <Grid item xs={12} sm={6} md={4} key={sandbox.id}>
                      <Paper
                        sx={{
                          p: 2,
                          display: 'flex',
                          justifyContent: 'space-between',
                          alignItems: 'center',
                          backgroundColor:
                            selectedSandboxId === sandbox.id
                              ? 'rgba(33, 150, 243, 0.1)'
                              : 'rgba(255,255,255,0.02)',
                          border: '1px solid',
                          borderColor:
                            selectedSandboxId === sandbox.id
                              ? 'rgba(33, 150, 243, 0.5)'
                              : 'rgba(255,255,255,0.1)',
                        }}
                      >
                        <Box>
                          <Typography variant="subtitle2" sx={{ fontFamily: 'monospace' }}>
                            {sandbox.id}
                          </Typography>
                          {sandbox.container_id && (
                            <Typography variant="caption" color="text.secondary">
                              Container: {sandbox.container_id.slice(0, 12)}
                            </Typography>
                          )}
                        </Box>
                        <Chip
                          label={sandbox.status}
                          size="small"
                          color={getStatusColor(sandbox.status)}
                        />
                      </Paper>
                    </Grid>
                  ))}
                </Grid>
              </CardContent>
            </Card>
          </Grid>
        )}
      </Grid>
    </Box>
  )
}

export default AgentSandboxes
