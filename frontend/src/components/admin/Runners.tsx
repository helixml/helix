import React, { FC, useEffect, useState } from 'react'
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
  Button,
  FormControl,
  InputLabel,
  MenuItem,
  Select,
  Tab,
  Tabs,
} from '@mui/material'
import RefreshIcon from '@mui/icons-material/Refresh'
import CloseIcon from '@mui/icons-material/Close'
import MemoryIcon from '@mui/icons-material/Memory'
import ComputerIcon from '@mui/icons-material/Computer'
import DesktopWindowsIcon from '@mui/icons-material/DesktopWindows'
import PersonIcon from '@mui/icons-material/Person'
import ThermostatIcon from '@mui/icons-material/Thermostat'
import VideocamIcon from '@mui/icons-material/Videocam'

import RunnerProfilesTable from '../dashboard/RunnerProfilesTable'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useAccount } from '../../contexts/account'
import useApi from '../../hooks/useApi'
import {
  useAssignRunnerProfile,
  useClearRunnerProfile,
  useListCompatibleRunnerProfiles,
  useListRunnerProfiles,
} from '../../services/runnerProfilesService'

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
  // sandbox_id identifies which Runner this GPU is attached to. Added so
  // the admin UI can filter the aggregated GPU list when a specific Runner
  // is selected in the dropdown.
  sandbox_id?: string
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

interface ClientBufferStats {
  client_id: number
  buffer_used: number
  buffer_size: number
  buffer_pct: number
}

interface VideoStreamingStats {
  client_count: number
  frames_received: number
  gop_buffer_size: number
  client_buffers?: ClientBufferStats[]
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
  video_stats?: VideoStreamingStats
  session_name?: string
  session_age?: string
  owner_name?: string
  organization_name?: string
  project_name?: string
  project_id?: string
  organization_id?: string
  task_number?: number
  task_name?: string
  task_prompt?: string
  task_id?: string
}

interface ServiceDownloadProgress {
  percent?: number
  current?: number
  total?: number
  eta?: string
  stage?: string
}

interface SandboxInstanceInfo {
  id: string
  session_id: string
  status: string
  container_id?: string
  active_profile_id?: string
  profile_status?: string
  profile_error?: string
  service_health?: Record<string, string>
  profile_progress?: Record<string, ServiceDownloadProgress>
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

// Map of compose-manager profile lifecycle states to chip colors. Keep in
// sync with composemgr.State.Status: assigning | pulling | starting |
// running | failed.
const getProfileStatusColor = (status: string): 'success' | 'warning' | 'error' | 'info' | 'default' => {
  switch (status) {
    case 'running':
      return 'success'
    case 'starting':
    case 'pulling':
    case 'assigning':
      return 'info'
    case 'failed':
      return 'error'
    default:
      return 'default'
  }
}

// SandboxProfileCard renders the inference-profile state for one sandbox:
// status chip, error if any, per-service health, and a progress bar per
// service that's actively downloading model weights from HF Hub. When no
// profile is assigned, it shows the assignment controls (compatible-
// profile dropdown + Assign button); when one is assigned, it shows a
// Clear button alongside the status.
const SandboxProfileCard: FC<{ sandbox: SandboxInstanceInfo }> = ({ sandbox }) => {
  const status = sandbox.profile_status || ''
  const profileID = sandbox.active_profile_id || ''
  const services = Object.entries(sandbox.service_health || {})
  const progressEntries = Object.entries(sandbox.profile_progress || {})

  const [pickedProfileID, setPickedProfileID] = useState<string>('')
  const [assignError, setAssignError] = useState<string | null>(null)

  // Compatible-profiles list filters by GPU vendor/arch/VRAM/count
  // server-side, so the dropdown only shows profiles that will actually
  // fit on this sandbox's GPUs. Don't query for offline sandboxes — the
  // endpoint would 404 since the runner state isn't in the router.
  const isOnline = sandbox.status === 'online'
  const { data: compatibleProfiles, isLoading: loadingProfiles } =
    useListCompatibleRunnerProfiles(sandbox.id, isOnline)
  const assignMutation = useAssignRunnerProfile()
  const clearMutation = useClearRunnerProfile()

  const handleAssign = () => {
    if (!pickedProfileID) return
    setAssignError(null)
    assignMutation.mutate(
      { runnerID: sandbox.id, profileID: pickedProfileID },
      {
        onError: (err: any) => {
          setAssignError(
            err?.response?.data?.error ||
              err?.message ||
              'Failed to assign profile',
          )
        },
        onSuccess: () => {
          setPickedProfileID('')
        },
      },
    )
  }
  const handleClear = () => {
    if (!window.confirm(`Clear the assigned profile from ${sandbox.id}? This stops the running compose stack.`)) return
    setAssignError(null)
    clearMutation.mutate(sandbox.id, {
      onError: (err: any) => {
        setAssignError(
          err?.response?.data?.error || err?.message || 'Failed to clear profile',
        )
      },
    })
  }

  const assignBusy = assignMutation.isPending || clearMutation.isPending

  return (
    <Paper sx={{ p: 2, backgroundColor: 'rgba(255,255,255,0.02)', border: '1px solid rgba(255,255,255,0.1)' }}>
      <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 1 }}>
        <Typography variant="caption" color="text.secondary" sx={{ fontFamily: 'monospace' }}>
          {sandbox.id}
        </Typography>
        <Chip label={status || 'idle'} size="small" color={getProfileStatusColor(status)} />
        {!isOnline && (
          <Chip label={sandbox.status} size="small" variant="outlined" color="default" />
        )}
      </Box>
      {profileID && (
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 0.5 }}>
          <Typography variant="body2">
            Profile: <span style={{ fontFamily: 'monospace' }}>{profileID}</span>
          </Typography>
          <Button
            size="small"
            variant="outlined"
            color="warning"
            onClick={handleClear}
            disabled={assignBusy}
          >
            {clearMutation.isPending ? 'Clearing…' : 'Clear'}
          </Button>
        </Box>
      )}
      {sandbox.profile_error && (
        <Alert severity="error" sx={{ my: 1, py: 0 }}>
          {sandbox.profile_error}
        </Alert>
      )}
      {assignError && (
        <Alert severity="error" sx={{ my: 1, py: 0 }} onClose={() => setAssignError(null)}>
          {assignError}
        </Alert>
      )}
      {progressEntries.length > 0 && (
        <Box sx={{ mt: 1 }}>
          {progressEntries.map(([svc, p]) => (
            <Box key={svc} sx={{ mb: 1.5 }}>
              <Box sx={{ display: 'flex', justifyContent: 'space-between', mb: 0.5 }}>
                <Typography variant="caption" color="text.secondary">
                  {svc} — {p.stage || 'downloading'}
                  {p.current && p.total ? ` (${p.current}/${p.total})` : ''}
                </Typography>
                <Typography variant="caption" fontWeight="bold">
                  {p.percent ?? 0}%{p.eta ? ` · ETA ${p.eta}` : ''}
                </Typography>
              </Box>
              <LinearProgress
                variant="determinate"
                value={p.percent ?? 0}
                sx={{ height: 6, borderRadius: 3 }}
              />
            </Box>
          ))}
        </Box>
      )}
      {services.length > 0 && (
        <Box sx={{ mt: 1, display: 'flex', flexWrap: 'wrap', gap: 0.5 }}>
          {services.map(([svc, health]) => (
            <Chip
              key={svc}
              label={`${svc}: ${health}`}
              size="small"
              variant="outlined"
              color={health === 'healthy' ? 'success' : health === 'failed' ? 'error' : 'warning'}
            />
          ))}
        </Box>
      )}

      {/* Assignment controls. Only render when there's no active
          profile — once assigned, the operator clears first then
          assigns again. */}
      {!profileID && isOnline && (
        <Box sx={{ mt: 2, display: 'flex', gap: 1, alignItems: 'center', flexWrap: 'wrap' }}>
          <FormControl size="small" sx={{ minWidth: 220, flex: 1 }} disabled={assignBusy || loadingProfiles}>
            <InputLabel>Assign profile</InputLabel>
            <Select
              label="Assign profile"
              value={pickedProfileID}
              onChange={(e) => setPickedProfileID(e.target.value)}
            >
              <MenuItem value="">
                <em>{loadingProfiles ? 'Loading compatible profiles…' : '(pick one)'}</em>
              </MenuItem>
              {(compatibleProfiles || []).map((p) => (
                <MenuItem key={p.id} value={p.id}>
                  {p.name}
                  {p.gpu_requirement?.count ? ` — ${p.gpu_requirement.count} GPU` : ''}
                </MenuItem>
              ))}
            </Select>
          </FormControl>
          <Button
            variant="contained"
            size="small"
            onClick={handleAssign}
            disabled={!pickedProfileID || assignBusy}
          >
            {assignMutation.isPending ? 'Assigning…' : 'Assign'}
          </Button>
          {!loadingProfiles && (compatibleProfiles || []).length === 0 && (
            <Typography variant="caption" color="text.secondary">
              No profiles match this sandbox's GPUs.
            </Typography>
          )}
        </Box>
      )}
    </Paper>
  )
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
interface DevContainerCardProps {
  container: DevContainerWithClients
  onStop: (sessionId: string) => void
  isStopping: boolean
}

const DevContainerCard: FC<DevContainerCardProps> = ({ container, onStop, isStopping }) => {
  const clients = container.clients || []
  const account = useAccount()

  const hasTaskLink = !!(container.task_id && container.project_id)

  const handleTaskClick = () => {
    if (!hasTaskLink) return
    account.orgNavigate('project-task-detail', {
      id: container.project_id!,
      taskId: container.task_id!,
      ...(container.organization_id ? { org_id: container.organization_id } : {}),
    })
  }

  return (
    <Paper
      sx={{
        p: 2,
        backgroundColor: 'rgba(255,255,255,0.02)',
        border: '1px solid',
        borderColor: container.status === 'running' ? 'rgba(255,255,255,0.2)' : 'rgba(255,255,255,0.1)',
      }}
    >
      {/* Header: status chip + stop button */}
      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 1 }}>
        <Chip
          label={container.status}
          size="small"
          color={getStatusColor(container.status)}
        />
        <Tooltip title="Stop container" arrow>
          <IconButton
            size="small"
            onClick={() => onStop(container.session_id)}
            disabled={isStopping}
            sx={{
              color: 'error.main',
              '&:hover': { backgroundColor: 'rgba(244, 67, 54, 0.1)' },
            }}
          >
            {isStopping ? <CircularProgress size={16} /> : <CloseIcon fontSize="small" />}
          </IconButton>
        </Tooltip>
      </Box>

      {/* Task/session title */}
      <Typography
        variant="subtitle1"
        fontWeight="bold"
        sx={{
          overflow: 'hidden',
          textOverflow: 'ellipsis',
          whiteSpace: 'nowrap',
          cursor: hasTaskLink ? 'pointer' : 'default',
          '&:hover': hasTaskLink ? { textDecoration: 'underline' } : {},
        }}
        onClick={hasTaskLink ? handleTaskClick : undefined}
      >
        {container.task_number
          ? `#${container.task_number} ${container.task_name || container.session_name || ''}`
          : container.session_name || 'Unnamed session'
        }
      </Typography>

      {/* Task prompt excerpt */}
      {container.task_prompt && (
        <Tooltip title={container.task_prompt} arrow>
          <Typography
            variant="caption"
            color="text.secondary"
            sx={{
              overflow: 'hidden',
              textOverflow: 'ellipsis',
              whiteSpace: 'nowrap',
              display: 'block',
              fontStyle: 'italic',
              mb: 0.5,
            }}
          >
            {container.task_prompt}
          </Typography>
        </Tooltip>
      )}

      {/* Metadata rows */}
      <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 0.5, mt: 1, mb: 1 }}>
        <Chip label={getContainerTypeLabel(container.container_type)} size="small" variant="outlined" />
        {container.session_age && (
          <Chip label={container.session_age} size="small" variant="outlined" />
        )}
        {container.owner_name && (
          <Chip icon={<PersonIcon />} label={container.owner_name} size="small" variant="outlined" />
        )}
      </Box>

      {/* Org / Project breadcrumb */}
      {(container.organization_name || container.project_name) && (
        <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mb: 0.5 }}>
          {container.organization_name || 'Personal'}
          {container.project_name && ` / ${container.project_name}`}
        </Typography>
      )}

      {/* Session ID */}
      <Tooltip title={container.session_id} arrow>
        <Typography
          variant="caption"
          sx={{
            fontFamily: 'monospace',
            fontSize: '0.7rem',
            color: 'text.disabled',
            overflow: 'hidden',
            textOverflow: 'ellipsis',
            whiteSpace: 'nowrap',
            display: 'block',
          }}
        >
          {container.session_id}
        </Typography>
      </Tooltip>

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

      {/* Video Streaming Buffer Stats */}
      {container.video_stats && container.video_stats.client_buffers && container.video_stats.client_buffers.length > 0 && (
        <Box sx={{ mt: 2, pt: 2, borderTop: '1px solid rgba(255,255,255,0.1)' }}>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 1 }}>
            <VideocamIcon fontSize="small" color="info" />
            <Typography variant="caption" color="text.secondary">
              Video Buffer ({container.video_stats.client_count} streaming)
            </Typography>
          </Box>
          {container.video_stats.client_buffers.map((cb) => (
            <Box key={cb.client_id} sx={{ mb: 1 }}>
              <Box sx={{ display: 'flex', justifyContent: 'space-between', mb: 0.5 }}>
                <Typography variant="caption" color="text.secondary">
                  Client #{cb.client_id}
                </Typography>
                <Typography
                  variant="caption"
                  sx={{
                    color: cb.buffer_pct > 50 ? 'error.main' : cb.buffer_pct > 10 ? 'warning.main' : 'success.main',
                    fontWeight: 'bold',
                  }}
                >
                  {cb.buffer_used} / {cb.buffer_size} ({cb.buffer_pct}%)
                </Typography>
              </Box>
              <LinearProgress
                variant="determinate"
                value={cb.buffer_pct}
                color={cb.buffer_pct > 50 ? 'error' : cb.buffer_pct > 10 ? 'warning' : 'success'}
                sx={{ height: 4, borderRadius: 2 }}
              />
            </Box>
          ))}
        </Box>
      )}
    </Paper>
  )
}

// SubTab is one of four lenses on the same underlying dataset:
//   overview  – aggregate stats + GPU panel (was the "Aggregate View" cards)
//   hosts     – the list of runner hosts themselves, one card per runner
//               showing status + profile assignment
//   sandboxes – user-facing dev containers (was "Dev Containers")
//   profiles  – inference-profile config (was the standalone Runner Profiles
//               admin tab; absorbed here because Profiles only make sense
//               in the context of the runners that consume them)
type SubTab = 'overview' | 'hosts' | 'sandboxes' | 'profiles'

const SUBTAB_LS_KEY = 'admin-runners-subtab'

function isSubTab(v: string | null): v is SubTab {
  return v === 'overview' || v === 'hosts' || v === 'sandboxes' || v === 'profiles'
}

const Runners: FC = () => {
  const api = useApi()
  const apiClient = api.getApiClient()
  const queryClient = useQueryClient()
  const [stoppingIds, setStoppingIds] = useState<Set<string>>(new Set())

  // Sub-tab state with localStorage persistence. URL-syncing would be nicer
  // for deep-linking but the admin shell controls the top-level `dialog_tab`
  // param and doesn't currently expose a nested-param hook; localStorage gets
  // us 90% of the way (same operator returns to the same lens) without
  // touching the shell.
  const [subTab, setSubTab] = useState<SubTab>(() => {
    if (typeof window === 'undefined') return 'overview'
    const stored = window.localStorage.getItem(SUBTAB_LS_KEY)
    return isSubTab(stored) ? stored : 'overview'
  })
  useEffect(() => {
    if (typeof window === 'undefined') return
    window.localStorage.setItem(SUBTAB_LS_KEY, subTab)
  }, [subTab])

  // Inference profile catalogue. Polled on the same 5s cadence as the debug
  // query below so the Profiles sub-tab tracks additions / deletions made
  // elsewhere without manual refresh.
  const {
    data: runnerProfiles,
    isLoading: isLoadingRunnerProfiles,
  } = useListRunnerProfiles({ refetchInterval: 5000 })

  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['agent-sandboxes-debug'],
    queryFn: async () => {
      const response = await apiClient.v1AdminAgentSandboxesDebugList({})
      return response.data as AgentSandboxesDebugResponse
    },
    refetchInterval: 5000,
  })

  // Stop session mutation - kills a dev container.
  const stopMutation = useMutation({
    mutationFn: async (sessionId: string) => {
      setStoppingIds((prev) => new Set(prev).add(sessionId))
      await apiClient.v1SessionsStopExternalAgentDelete(sessionId)
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['agent-sandboxes-debug'] })
    },
    onSettled: (_data, _error, sessionId) => {
      setStoppingIds((prev) => {
        const next = new Set(prev)
        next.delete(sessionId)
        return next
      })
    },
  })

  const handleStopContainer = (sessionId: string) => {
    stopMutation.mutate(sessionId)
  }

  const gpus = data?.gpus || []
  const sandboxes = data?.sandboxes || []
  const devContainers = data?.dev_containers || []

  // Sort containers by session_id for stable rendering across refetches.
  const sortedContainers = [...devContainers].sort((a, b) =>
    a.session_id.localeCompare(b.session_id)
  )

  // Decide whether the Profiles assignment panel inside Hosts should render.
  // Visible when at least one Runner Profile is configured OR when any runner
  // already has a profile assigned (so deleting the last catalogue entry
  // mid-interaction doesn't hide a "Clear" button from the operator).
  const anySandboxHasProfileAssigned = sandboxes.some((s) => !!s.active_profile_id)
  const hasRunnerProfiles = (runnerProfiles?.length ?? 0) > 0
  const showInferenceProfilesPanel =
    isLoadingRunnerProfiles ||
    hasRunnerProfiles ||
    anySandboxHasProfileAssigned

  // Aggregate stats - "online" not "running" (sandbox status enum).
  const runningRunners = sandboxes.filter((s) => s.status === 'online').length
  const runningContainers = sortedContainers.filter((c) => c.status === 'running').length
  const totalClients = sortedContainers.reduce((sum, c) => sum + (c.clients?.length || 0), 0)

  return (
    <Box sx={{ p: 3 }}>
      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 2 }}>
        <Box>
          <Typography variant="h5">Runners</Typography>
          <Typography variant="body2" color="text.secondary">
            Aggregate view of all Helix Runners
          </Typography>
        </Box>
        <IconButton onClick={() => refetch()} disabled={isLoading} sx={{ color: 'primary.main' }}>
          {isLoading ? <CircularProgress size={24} /> : <RefreshIcon />}
        </IconButton>
      </Box>

      {error && (
        <Alert severity="error" sx={{ mb: 3 }}>
          {error instanceof Error ? error.message : 'Failed to fetch runner data'}
        </Alert>
      )}

      <Tabs
        value={subTab}
        onChange={(_, v) => setSubTab(v as SubTab)}
        sx={{ borderBottom: 1, borderColor: 'divider', mb: 3 }}
      >
        <Tab value="overview" label="Overview" />
        <Tab value="hosts" label="Hosts" />
        <Tab value="sandboxes" label="Sandboxes" />
        <Tab value="profiles" label="Profiles" />
      </Tabs>

      {subTab === 'overview' && (
        <Box>
          <Grid container spacing={2} sx={{ mb: 3 }}>
            <Grid item xs={12} sm={4}>
              <Paper sx={{ p: 2, textAlign: 'center' }}>
                <ComputerIcon sx={{ fontSize: 40, color: 'primary.main', mb: 1 }} />
                <Typography variant="h4">{runningRunners}</Typography>
                <Typography variant="body2" color="text.secondary">
                  Online Runners
                </Typography>
              </Paper>
            </Grid>
            <Grid item xs={12} sm={4}>
              <Paper sx={{ p: 2, textAlign: 'center' }}>
                <DesktopWindowsIcon sx={{ fontSize: 40, color: 'success.main', mb: 1 }} />
                <Typography variant="h4">{runningContainers}</Typography>
                <Typography variant="body2" color="text.secondary">
                  Active Sandboxes
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

          {gpus.length > 0 && <GPUStatsCard gpus={gpus} />}
          {gpus.length === 0 && (
            <Box sx={{ textAlign: 'center', py: 4 }}>
              <Typography variant="body2" color="text.secondary">
                No GPU stats reported. Bring up a runner to see GPU telemetry.
              </Typography>
            </Box>
          )}
        </Box>
      )}

      {subTab === 'hosts' && (
        <Box>
          {showInferenceProfilesPanel && sandboxes.length > 0 ? (
            <Grid container spacing={2}>
              {sandboxes.map((sb) => (
                <Grid item xs={12} md={6} lg={4} key={sb.id}>
                  <SandboxProfileCard sandbox={sb} />
                </Grid>
              ))}
            </Grid>
          ) : sandboxes.length === 0 ? (
            <Box sx={{ textAlign: 'center', py: 6 }}>
              <ComputerIcon sx={{ fontSize: 60, color: 'text.secondary', mb: 2 }} />
              <Typography variant="body1" color="text.secondary">
                No runners registered
              </Typography>
              <Typography variant="body2" color="text.secondary">
                Provision one via YellowDog, or self-register a runner pointing at this control plane.
              </Typography>
            </Box>
          ) : (
            <Grid container spacing={2}>
              {sandboxes.map((sb) => (
                <Grid item xs={12} md={6} lg={4} key={sb.id}>
                  <SandboxProfileCard sandbox={sb} />
                </Grid>
              ))}
            </Grid>
          )}
        </Box>
      )}

      {subTab === 'sandboxes' && (
        <Box>
          {sortedContainers.length > 0 ? (
            <Grid container spacing={2}>
              {sortedContainers.map((container) => (
                <Grid item xs={12} md={6} lg={4} key={`${container.sandbox_id}-${container.session_id}`}>
                  <DevContainerCard
                    container={container}
                    onStop={handleStopContainer}
                    isStopping={stoppingIds.has(container.session_id)}
                  />
                </Grid>
              ))}
            </Grid>
          ) : (
            <Box sx={{ textAlign: 'center', py: 6 }}>
              <DesktopWindowsIcon sx={{ fontSize: 60, color: 'text.secondary', mb: 2 }} />
              <Typography variant="body1" color="text.secondary">
                No dev containers running
              </Typography>
              <Typography variant="body2" color="text.secondary">
                Start a task to launch a desktop sandbox.
              </Typography>
            </Box>
          )}
        </Box>
      )}

      {subTab === 'profiles' && (
        <Box>
          <RunnerProfilesTable />
        </Box>
      )}
    </Box>
  )
}

export default Runners
