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
  Divider,
  Tooltip,
} from '@mui/material'
import RefreshIcon from '@mui/icons-material/Refresh'
import MemoryIcon from '@mui/icons-material/Memory'
import VideocamIcon from '@mui/icons-material/Videocam'
import TimelineIcon from '@mui/icons-material/Timeline'
import AccountTreeIcon from '@mui/icons-material/AccountTree'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import useApi from '../../hooks/useApi'





const formatBytes = (bytesStr: string): string => {
  const bytes = parseInt(bytesStr, 10)
  if (isNaN(bytes) || bytes === 0) return '0 Bytes'
  const k = 1024
  const sizes = ['Bytes', 'KB', 'MB', 'GB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return Math.round((bytes / Math.pow(k, i)) * 100) / 100 + ' ' + sizes[i]
}

const truncateName = (name: string, maxLength: number = 20): string => {
  if (name.length <= maxLength) return name
  return name.substring(0, maxLength - 3) + '...'
}

// GStreamer Pipeline Network Visualization Component
const PipelineNetworkVisualization: FC<{ data: AgentSandboxesDebugResponse }> = ({ data }) => {
  // Support both apps and lobbies modes
  const apps = data.apps || []
  const lobbies = data.lobbies || []
  const isAppsMode = data.wolf_mode === 'apps'  // Use explicit wolf_mode field
  const containers = isAppsMode ? apps : lobbies // Apps or Lobbies

  const sessions = data.sessions || []
  const moonlightClients = data.moonlight_clients || []  // NEW: moonlight-web clients
  const memoryData = data.memory
  const containerMemory = isAppsMode ? (memoryData?.apps || []) : (memoryData?.lobbies || [])
  const clientConnections = memoryData?.clients || []

  // Build connection map: session_id -> container_id (app_id or lobby_id)
  const connectionMap = new Map<string, string>()
  clientConnections.forEach((client) => {
    const containerId = client.app_id || client.lobby_id
    if (containerId) {
      connectionMap.set(client.session_id, containerId)
    }
  })

  // Layout parameters (3-tier architecture)
  const svgWidth = 1000
  const svgHeight = 700  // Increased for 3 layers
  const containerRadius = 60
  const sessionRadius = 30
  const clientRadius = 25
  const containerY = 120  // Apps/Lobbies at top
  const sessionY = 350    // Wolf sessions in middle
  const clientY = 580     // Moonlight-web clients at bottom

  // Position containers (apps or lobbies) horizontally
  const containerPositions = new Map<string, { x: number; y: number }>()
  const containerSpacing = svgWidth / (containers.length + 1)
  containers.forEach((container, idx) => {
    const containerId = 'id' in container ? container.id : container.id
    containerPositions.set(containerId, {
      x: containerSpacing * (idx + 1),
      y: containerY,
    })
  })

  // Position sessions horizontally
  // Use session_id + app_id as unique key since same client can connect to multiple apps
  const sessionPositions = new Map<string, { x: number; y: number }>()
  const sessionSpacing = svgWidth / (sessions.length + 1)
  sessions.forEach((session, idx) => {
    const uniqueKey = `${session.session_id}-${session.app_id}`
    sessionPositions.set(uniqueKey, {
      x: sessionSpacing * (idx + 1),
      y: sessionY,
    })
  })

  // Position moonlight-web clients horizontally
  const clientPositions = new Map<string, { x: number; y: number }>()
  const clientSpacing = svgWidth / (moonlightClients.length + 1)
  moonlightClients.forEach((client, idx) => {
    clientPositions.set(client.session_id, {
      x: clientSpacing * (idx + 1),
      y: clientY,
    })
  })

  // Find memory for each container (app or lobby)
  const getContainerMemory = (containerId: string): string => {
    if (isAppsMode) {
      const mem = containerMemory.find((am: any) => am.app_id === containerId)
      return mem ? formatBytes(mem.memory_bytes) : '0 B'
    } else {
      const mem = containerMemory.find((lm: any) => lm.lobby_id === containerId)
      return mem ? formatBytes(mem.memory_bytes) : '0 B'
    }
  }

  // Find client count for each container
  const getContainerClientCount = (containerId: string): number => {
    if (isAppsMode) {
      return clientConnections.filter((c) => c.app_id === containerId).length
    } else {
      return clientConnections.filter((c) => c.lobby_id === containerId).length
    }
  }

  // Get container name
  const getContainerName = (container: any): string => {
    return container.title || container.name || container.id
  }

  // Find session memory
  const getSessionMemory = (sessionId: string): string => {
    const client = clientConnections.find((c) => c.session_id === sessionId)
    return client ? formatBytes(client.memory_bytes) : '0 B'
  }

  return (
    <Card sx={{ height: '100%' }}>
      <CardHeader
        avatar={<AccountTreeIcon />}
        title="Streaming Pipeline Architecture"
        subheader={`${isAppsMode ? 'Apps (1:1 direct)' : 'Lobbies (interpipe)'} → Wolf Sessions → Moonlight-web Clients`}
      />
      <CardContent>
        <svg width={svgWidth} height={svgHeight} style={{ border: '1px solid rgba(255,255,255,0.1)' }}>
          {/* Draw container-to-session connection lines */}
          {sessions.map((session) => {
            // In apps mode, session.app_id directly identifies the connected container
            const connectedContainerId = session.app_id
            if (!connectedContainerId) return null

            const uniqueKey = `${session.session_id}-${session.app_id}`
            const sessionPos = sessionPositions.get(uniqueKey)
            const containerPos = containerPositions.get(connectedContainerId)
            if (!sessionPos || !containerPos) return null

            return (
              <g key={`connection-${uniqueKey}`}>
                <line
                  x1={sessionPos.x}
                  y1={sessionPos.y - sessionRadius}
                  x2={containerPos.x}
                  y2={containerPos.y + containerRadius}
                  stroke="#00c8ff"
                  strokeWidth="2"
                  strokeDasharray="5,5"
                  opacity="0.6"
                />
                <text
                  x={(sessionPos.x + containerPos.x) / 2}
                  y={(sessionPos.y + containerPos.y) / 2}
                  fill="rgba(255,255,255,0.5)"
                  fontSize="10"
                  textAnchor="middle"
                >
                  {isAppsMode ? 'direct' : 'interpipe'}
                </text>
              </g>
            )
          })}

          {/* Draw session-to-client connection lines */}
          {moonlightClients.map((client) => {
            // Extract Helix session ID from moonlight client session ID (format: "agent-{sessionId}")
            const helixSessionId = client.session_id.replace(/^agent-/, '')

            // Find the app this client corresponds to by matching title
            // Title format changed to "Agent {last4}" for compact display
            const shortId = helixSessionId.slice(-4)
            const expectedAppTitle = `Agent ${shortId}`
            const clientApp = apps.find(app => app.title === expectedAppTitle)
            if (!clientApp) {
              console.warn(`[Dashboard] No app found for client ${client.session_id}`, {
                expectedTitle: expectedAppTitle,
                availableApps: apps.map(a => ({ id: a.id, title: a.title }))
              })
              return null
            }
            console.log(`[Dashboard] Found app for client ${client.session_id}:`, clientApp)

            // Find Wolf session connected to this app
            // Wolf sessions have app_id field directly
            const matchingSession = sessions.find(s => s.app_id === clientApp.id)

            if (!matchingSession) {
              console.warn(`[Dashboard] No Wolf session found for app ${clientApp.id}`, {
                clientSessionId: client.session_id,
                appId: clientApp.id,
                appTitle: clientApp.title,
                sessionsCount: sessions.length,
                firstFewSessions: sessions.slice(0, 3).map(s => ({ session_id: s.session_id, app_id: s.app_id }))
              })
              return null
            }
            console.log(`[Dashboard] Found Wolf session for app ${clientApp.id}:`, matchingSession)

            const clientPos = clientPositions.get(client.session_id)
            const sessionUniqueKey = `${matchingSession.session_id}-${matchingSession.app_id}`
            const sessionPos = sessionPositions.get(sessionUniqueKey)

            console.log(`[Dashboard] Checking positions for client ${client.session_id}:`, {
              clientPos,
              sessionUniqueKey,
              sessionPos,
              hasClient: !!clientPos,
              hasSession: !!sessionPos
            })

            if (!clientPos || !sessionPos) {
              console.warn(`[Dashboard] Missing position for connection line`, {
                client: client.session_id,
                sessionKey: sessionUniqueKey,
                hasClientPos: !!clientPos,
                hasSessionPos: !!sessionPos
              })
              return null
            }

            console.log(`[Dashboard] Drawing connection line from client ${client.session_id} to session ${sessionUniqueKey}`)

            return (
              <g key={`client-connection-${client.session_id}`}>
                <line
                  x1={clientPos.x}
                  y1={clientPos.y - clientRadius}
                  x2={sessionPos.x}
                  y2={sessionPos.y + sessionRadius}
                  stroke={client.has_websocket ? "#4caf50" : "#ffc107"}
                  strokeWidth="2"
                  strokeDasharray={client.has_websocket ? "0" : "5,5"}
                  opacity="0.7"
                />
                <text
                  x={(clientPos.x + sessionPos.x) / 2}
                  y={(clientPos.y + sessionPos.y) / 2}
                  fill="rgba(255,255,255,0.5)"
                  fontSize="10"
                  textAnchor="middle"
                >
                  {client.has_websocket ? 'WebRTC' : 'headless'}
                </text>
              </g>
            )
          })}

          {/* Draw containers (apps or lobbies - producer pipelines) */}
          {containers.map((container) => {
            const containerId = 'id' in container ? container.id : container.id
            const containerName = getContainerName(container)
            const pos = containerPositions.get(containerId)
            if (!pos) return null

            const clientCount = getContainerClientCount(containerId)
            const hasClients = clientCount > 0

            return (
              <g key={`container-${containerId}`}>
                <Tooltip title={`${isAppsMode ? 'App' : 'Lobby'}: ${containerName}`} arrow>
                  <circle
                    cx={pos.x}
                    cy={pos.y}
                    r={containerRadius}
                    fill={hasClients ? 'rgba(76, 175, 80, 0.2)' : 'rgba(255, 193, 7, 0.2)'}
                    stroke={hasClients ? '#4caf50' : '#ffc107'}
                    strokeWidth="3"
                  />
                </Tooltip>
                <text
                  x={pos.x}
                  y={pos.y - 10}
                  textAnchor="middle"
                  fill="white"
                  fontSize="12"
                  fontWeight="bold"
                >
                  {truncateName(containerName, 15)}
                </text>
                <text x={pos.x} y={pos.y + 5} textAnchor="middle" fill="rgba(255,255,255,0.7)" fontSize="10">
                  {getContainerMemory(containerId)}
                </text>
                <text x={pos.x} y={pos.y + 20} textAnchor="middle" fill="rgba(255,255,255,0.5)" fontSize="9">
                  {clientCount} client{clientCount !== 1 ? 's' : ''}
                </text>
                {/* Producer pipeline indicator */}
                {!isAppsMode && (
                  <>
                    <rect
                      x={pos.x - 30}
                      y={pos.y - containerRadius - 20}
                      width="60"
                      height="15"
                      fill="rgba(103, 58, 183, 0.3)"
                      stroke="#673ab7"
                      strokeWidth="1"
                      rx="3"
                    />
                    <text
                      x={pos.x}
                      y={pos.y - containerRadius - 9}
                      textAnchor="middle"
                      fill="white"
                      fontSize="8"
                    >
                      {isAppsMode ? 'direct pipeline' : 'interpipesink'}
                    </text>
                  </>
                )}
              </g>
            )
          })}

          {/* Draw sessions (consumer pipelines) */}
          {sessions.map((session) => {
            const uniqueKey = `${session.session_id}-${session.app_id}`
            const pos = sessionPositions.get(uniqueKey)
            if (!pos) return null

            // In apps mode, session.app_id is the direct connection (no interpipe needed)
            // In lobbies mode, session.lobby_id shows which lobby the session is connected to
            const connectedContainerId = isAppsMode ? session.app_id : (session.lobby_id || connectionMap.get(session.session_id))
            const isOrphaned = !connectedContainerId

            return (
              <g key={`session-${uniqueKey}`}>
                <Tooltip title={
                  `Wolf-UI Session: ${session.session_id.slice(-8)}\n` +
                  `Client ID: ${session.client_unique_id || 'N/A'}\n` +
                  `Connected to: ${session.lobby_id ? `Lobby ${session.lobby_id.slice(-8)}` : session.app_id ? `App ${session.app_id}` : 'None (orphaned)'}\n` +
                  `IP: ${session.client_ip}`
                } arrow>
                  <circle
                    cx={pos.x}
                    cy={pos.y}
                    r={sessionRadius}
                    fill={isOrphaned ? 'rgba(244, 67, 54, 0.2)' : 'rgba(33, 150, 243, 0.2)'}
                    stroke={isOrphaned ? '#f44336' : '#2196f3'}
                    strokeWidth="2"
                  />
                </Tooltip>
                <text
                  x={pos.x}
                  y={pos.y - 5}
                  textAnchor="middle"
                  fill="white"
                  fontSize="10"
                >
                  {session.client_unique_id ? session.client_unique_id.slice(-12) : session.session_id.slice(-4)}
                </text>
                <text
                  x={pos.x}
                  y={pos.y + 8}
                  textAnchor="middle"
                  fill="rgba(255,255,255,0.6)"
                  fontSize="8"
                >
                  {getSessionMemory(session.session_id)}
                </text>
                {/* Consumer pipeline indicator */}
                {!isAppsMode && (
                  <>
                    <rect
                      x={pos.x - 30}
                      y={pos.y + sessionRadius + 5}
                      width="60"
                      height="15"
                      fill="rgba(63, 81, 181, 0.3)"
                      stroke="#3f51b5"
                      strokeWidth="1"
                      rx="3"
                    />
                    <text
                      x={pos.x}
                      y={pos.y + sessionRadius + 16}
                      textAnchor="middle"
                      fill="white"
                      fontSize="8"
                    >
                      {isAppsMode ? 'direct pipeline' : 'interpipesrc'}
                    </text>
                  </>
                )}
              </g>
            )
          })}

          {/* Draw moonlight-web clients (WebRTC consumers) */}
          {moonlightClients.map((client) => {
            const pos = clientPositions.get(client.session_id)
            if (!pos) return null

            const hasWebRTC = client.has_websocket
            const modeColor = client.mode === 'keepalive' ? '#ffc107' : client.mode === 'join' ? '#2196f3' : '#9c27b0'
            const clientDisplay = client.client_unique_id || client.session_id.slice(-8)

            return (
              <g key={`client-${client.session_id}`}>
                <Tooltip title={`Client: ${client.session_id} | Moonlight ID: ${client.client_unique_id || 'default'} | Mode: ${client.mode} | WebRTC: ${hasWebRTC ? 'Yes' : 'No'}`} arrow>
                  <circle
                    cx={pos.x}
                    cy={pos.y}
                    r={clientRadius}
                    fill={hasWebRTC ? 'rgba(76, 175, 80, 0.2)' : 'rgba(255, 193, 7, 0.2)'}
                    stroke={hasWebRTC ? '#4caf50' : '#ffc107'}
                    strokeWidth="2"
                  />
                </Tooltip>
                <text
                  x={pos.x}
                  y={pos.y - 8}
                  textAnchor="middle"
                  fill="white"
                  fontSize="9"
                >
                  {client.mode}
                </text>
                {client.client_unique_id && (
                  <text
                    x={pos.x}
                    y={pos.y + 2}
                    textAnchor="middle"
                    fill="rgba(255,255,255,0.7)"
                    fontSize="7"
                    fontWeight="bold"
                  >
                    {client.client_unique_id.slice(-12)}
                  </text>
                )}
                <text
                  x={pos.x}
                  y={pos.y + (client.client_unique_id ? 12 : 7)}
                  textAnchor="middle"
                  fill="rgba(255,255,255,0.5)"
                  fontSize="6"
                >
                  {hasWebRTC ? 'WebRTC' : 'headless'}
                </text>
                {/* Client type indicator */}
                <rect
                  x={pos.x - 25}
                  y={pos.y + clientRadius + 5}
                  width="50"
                  height="12"
                  fill="rgba(156, 39, 176, 0.3)"
                  stroke="#9c27b0"
                  strokeWidth="1"
                  rx="3"
                />
                <text
                  x={pos.x}
                  y={pos.y + clientRadius + 14}
                  textAnchor="middle"
                  fill="white"
                  fontSize="7"
                >
                  moonlight-web
                </text>
              </g>
            )
          })}

          {/* Legend */}
          <g transform="translate(20, 20)">
            <text x="0" y="0" fill="white" fontSize="12" fontWeight="bold">
              Legend:
            </text>
            <circle cx="10" cy="20" r="8" fill="rgba(76, 175, 80, 0.2)" stroke="#4caf50" strokeWidth="2" />
            <text x="25" y="25" fill="rgba(255,255,255,0.8)" fontSize="10">
              Active {isAppsMode ? 'App' : 'Lobby'} (has clients)
            </text>
            <circle cx="10" cy="40" r="8" fill="rgba(255, 193, 7, 0.2)" stroke="#ffc107" strokeWidth="2" />
            <text x="25" y="45" fill="rgba(255,255,255,0.8)" fontSize="10">
              Empty {isAppsMode ? 'App' : 'Lobby'}
            </text>
            <circle cx="10" cy="60" r="8" fill="rgba(33, 150, 243, 0.2)" stroke="#2196f3" strokeWidth="2" />
            <text x="25" y="65" fill="rgba(255,255,255,0.8)" fontSize="10">
              Connected Session
            </text>
            <circle cx="10" cy="80" r="8" fill="rgba(244, 67, 54, 0.2)" stroke="#f44336" strokeWidth="2" />
            <text x="25" y="85" fill="rgba(255,255,255,0.8)" fontSize="10">
              Orphaned Session
            </text>
            <circle cx="10" cy="100" r="8" fill="rgba(76, 175, 80, 0.2)" stroke="#4caf50" strokeWidth="2" />
            <text x="25" y="105" fill="rgba(255,255,255,0.8)" fontSize="10">
              WebRTC Client (active)
            </text>
            <circle cx="10" cy="120" r="8" fill="rgba(255, 193, 7, 0.2)" stroke="#ffc107" strokeWidth="2" />
            <text x="25" y="125" fill="rgba(255,255,255,0.8)" fontSize="10">
              Headless Client (keepalive)
            </text>
          </g>
        </svg>

        <Box sx={{ mt: 2 }}>
          <Typography variant="caption" color="text.secondary">
            <strong>3-Tier Architecture:</strong> {isAppsMode
              ? 'Apps mode - Direct 1:1 connections. Each app has its own GStreamer pipeline directly connected to one Wolf session.'
              : 'Lobbies mode - Dynamic interpipe switching. Lobbies run interpipesink producers, sessions consume via interpipesrc and can dynamically switch between lobbies.'
            } Moonlight-web Clients (bottom) connect to Wolf sessions via WebRTC (solid green = active WebRTC, dashed yellow = headless keepalive).
          </Typography>
        </Box>
      </CardContent>
    </Card>
  )
}

const AgentSandboxes: FC = () => {
  const api = useApi()
  const apiClient = api.getApiClient()
  const queryClient = useQueryClient()

  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['agent-sandboxes-debug'],
    queryFn: async () => {
      const response = await apiClient.v1AdminAgentSandboxesDebugList()
      return response.data
    },
    refetchInterval: 5000,
  })

  const memoryData = data?.memory
  const apps = data?.apps || []
  const lobbies = data?.lobbies || []
  const isAppsMode = data?.wolf_mode === 'apps'  // Use explicit wolf_mode field
  const containers = isAppsMode ? apps : lobbies
  const sessions = data?.sessions || []

  return (
    <Box sx={{ p: 3 }}>
      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 3 }}>
        <Typography variant="h4">Agent Sandboxes Dashboard</Typography>
        <IconButton onClick={() => refetch()} disabled={isLoading} sx={{ color: 'primary.main' }}>
          {isLoading ? <CircularProgress size={24} /> : <RefreshIcon />}
        </IconButton>
      </Box>

      <Typography variant="body2" color="text.secondary" sx={{ mb: 3 }}>
        Monitor Wolf streaming infrastructure: GStreamer pipelines, {isAppsMode ? 'direct connections' : 'interpipe connections'}, memory usage, and
        real-time streaming sessions
      </Typography>

      {error && (
        <Alert severity="error" sx={{ mb: 2 }}>
          {error instanceof Error ? error.message : 'Failed to fetch Wolf debugging data'}
        </Alert>
      )}

      <Grid container spacing={3}>
        {/* GPU Stats Panel - Only show if GPU is available */}
        {data?.gpu_stats?.available && (
          <Grid item xs={12}>
            <Card>
              <CardHeader
                avatar={<MemoryIcon />}
                title="GPU Encoder Stats"
                subheader={
                  <>
                    {data.gpu_stats.gpu_name || 'NVIDIA GPU'}
                    {data.gstreamer_pipelines && (
                      <>
                        {' • '}
                        {data.gstreamer_pipelines.total_pipelines} GStreamer Pipelines
                        {' ('}
                        {data.gstreamer_pipelines.producer_pipelines} producers + {data.gstreamer_pipelines.consumer_pipelines} consumers)
                      </>
                    )}
                    {data.gpu_stats.query_duration_ms > 0 && (
                      <>
                        {' • '}
                        nvidia-smi: {data.gpu_stats.query_duration_ms}ms (cached 2s)
                      </>
                    )}
                  </>
                }
                action={
                  <Chip
                    label="Available"
                    color="success"
                    size="small"
                  />
                }
              />
              <CardContent>
                <Grid container spacing={2}>
                    <Grid item xs={12} sm={6} md={3}>
                      <Box>
                        <Typography variant="body2" color="text.secondary">
                          NVENC Sessions
                        </Typography>
                        <Typography variant="h4">
                          {data.gpu_stats.encoder_session_count}
                        </Typography>
                      </Box>
                    </Grid>
                    <Grid item xs={12} sm={6} md={3}>
                      <Box>
                        <Typography variant="body2" color="text.secondary">
                          Encoder FPS
                        </Typography>
                        <Typography variant="h4">
                          {data.gpu_stats.encoder_average_fps.toFixed(0)}
                        </Typography>
                      </Box>
                    </Grid>
                    <Grid item xs={12} sm={6} md={3}>
                      <Box>
                        <Typography variant="body2" color="text.secondary">
                          Encoder Latency
                        </Typography>
                        <Typography variant="h4">
                          {data.gpu_stats.encoder_average_latency_us}µs
                        </Typography>
                      </Box>
                    </Grid>
                    <Grid item xs={12} sm={6} md={3}>
                      <Box>
                        <Typography variant="body2" color="text.secondary">
                          GPU Temperature
                        </Typography>
                        <Typography variant="h4">
                          {data.gpu_stats.temperature_celsius}°C
                        </Typography>
                      </Box>
                    </Grid>
                    <Grid item xs={12} sm={6} md={4}>
                      <Box>
                        <Typography variant="body2" color="text.secondary">
                          Encoder Utilization
                        </Typography>
                        <Typography variant="h6">
                          {data.gpu_stats.encoder_utilization_percent}%
                        </Typography>
                      </Box>
                    </Grid>
                    <Grid item xs={12} sm={6} md={4}>
                      <Box>
                        <Typography variant="body2" color="text.secondary">
                          GPU Utilization
                        </Typography>
                        <Typography variant="h6">
                          {data.gpu_stats.gpu_utilization_percent}%
                        </Typography>
                      </Box>
                    </Grid>
                    <Grid item xs={12} sm={6} md={4}>
                      <Box>
                        <Typography variant="body2" color="text.secondary">
                          VRAM Usage
                        </Typography>
                        <Typography variant="h6">
                          {data.gpu_stats.memory_used_mb} / {data.gpu_stats.memory_total_mb} MB
                          ({Math.round((data.gpu_stats.memory_used_mb / data.gpu_stats.memory_total_mb) * 100)}%)
                        </Typography>
                      </Box>
                    </Grid>
                  </Grid>
              </CardContent>
            </Card>
          </Grid>
        )}

        {/* Pipeline Network Visualization */}
        {data && (
          <Grid item xs={12}>
            <PipelineNetworkVisualization data={data} />
          </Grid>
        )}

        {/* Memory Usage Panel */}
        <Grid item xs={12} md={6}>
          <Card sx={{ height: '100%' }}>
            <CardHeader
              avatar={<MemoryIcon />}
              title="Memory Usage"
              subheader="Wolf process and GStreamer buffers"
            />
            <CardContent>
              {memoryData ? (
                <>
                  <Box sx={{ mb: 2 }}>
                    <Typography variant="body2" color="text.secondary">
                      Process RSS
                    </Typography>
                    <Typography variant="h6">{formatBytes(memoryData.process_rss_bytes)}</Typography>
                  </Box>
                  <Box sx={{ mb: 2 }}>
                    <Typography variant="body2" color="text.secondary">
                      GStreamer Buffers
                    </Typography>
                    <Typography variant="h6">{formatBytes(memoryData.gstreamer_buffer_bytes)}</Typography>
                  </Box>
                  <Box sx={{ mb: 2 }}>
                    <Typography variant="body2" color="text.secondary">
                      Total Memory
                    </Typography>
                    <Typography variant="h6">{formatBytes(memoryData.total_memory_bytes)}</Typography>
                  </Box>
                  <Divider sx={{ my: 2 }} />
                  <Typography variant="subtitle2" gutterBottom>
                    Per-{isAppsMode ? 'App' : 'Lobby'} Breakdown
                  </Typography>
                  {isAppsMode && memoryData.apps && memoryData.apps.length > 0 ? (
                    memoryData.apps.map((app: any) => (
                      <Box key={app.app_id} sx={{ mb: 1 }}>
                        <Typography variant="body2">
                          {app.app_name}: {formatBytes(app.memory_bytes)}
                        </Typography>
                        <Typography variant="caption" color="text.secondary">
                          {app.resolution} • {app.client_count} clients
                        </Typography>
                      </Box>
                    ))
                  ) : !isAppsMode && memoryData.lobbies && memoryData.lobbies.length > 0 ? (
                    memoryData.lobbies.map((lobby) => (
                      <Box key={lobby.lobby_id} sx={{ mb: 1 }}>
                        <Typography variant="body2">
                          {lobby.lobby_name}: {formatBytes(lobby.memory_bytes)}
                        </Typography>
                        <Typography variant="caption" color="text.secondary">
                          {lobby.resolution} • {lobby.client_count} clients
                        </Typography>
                      </Box>
                    ))
                  ) : (
                    <Typography variant="body2" color="text.secondary">
                      No {isAppsMode ? 'apps' : 'lobbies'} active
                    </Typography>
                  )}
                </>
              ) : (
                <Typography variant="body2" color="text.secondary">
                  Loading memory data...
                </Typography>
              )}
            </CardContent>
          </Card>
        </Grid>

        {/* Active Apps/Lobbies Panel */}
        <Grid item xs={12} md={6}>
          <Card sx={{ height: '100%' }}>
            <CardHeader
              avatar={<VideocamIcon />}
              title={`Active ${isAppsMode ? 'Apps' : 'Lobbies'}`}
              subheader={isAppsMode ? 'Direct GStreamer pipelines (1:1)' : 'Producer pipelines with interpipesink'}
            />
            <CardContent>
              {containers.length > 0 ? (
                <Box>
                  {containers.map((container: any) => {
                    const containerId = container.id
                    const containerName = container.title || container.name
                    return (
                      <Box
                        key={containerId}
                        sx={{
                          mb: 2,
                          p: 2,
                          border: '1px solid rgba(255,255,255,0.1)',
                          borderRadius: 1,
                        }}
                      >
                        <Typography variant="subtitle1" fontWeight="bold">
                          <Tooltip title={containerName} placement="top">
                            <span>{truncateName(containerName)}</span>
                          </Tooltip>
                        </Typography>
                        <Typography variant="caption" sx={{ fontFamily: 'monospace', color: 'text.secondary' }}>
                          {containerId}
                        </Typography>
                        {!isAppsMode && (
                          <Box sx={{ mt: 1, display: 'flex', gap: 1, flexWrap: 'wrap' }}>
                            <Chip
                              label={container.multi_user ? 'Multi-User' : 'Single-User'}
                              size="small"
                              color={container.multi_user ? 'success' : 'default'}
                            />
                            {container.stop_when_everyone_leaves && (
                              <Chip label="Auto-Stop" size="small" color="warning" />
                            )}
                          </Box>
                        )}
                        {!isAppsMode && (
                          <Typography variant="caption" color="text.secondary" sx={{ mt: 1, display: 'block' }}>
                            GStreamer: interpipesink name="{containerId}_video" | interpipesink name="{containerId}_audio"
                          </Typography>
                        )}
                      </Box>
                    )
                  })}
                </Box>
              ) : (
                <Typography variant="body2" color="text.secondary">
                  No active {isAppsMode ? 'apps' : 'lobbies'}
                </Typography>
              )}
            </CardContent>
          </Card>
        </Grid>

        {/* Stream Sessions Panel */}
        <Grid item xs={12}>
          <Card>
            <CardHeader
              avatar={<TimelineIcon />}
              title="Stream Sessions"
              subheader={isAppsMode ? 'Wolf sessions (direct connection)' : 'Consumer pipelines with interpipesrc'}
            />
            <CardContent>
              {sessions.length > 0 ? (
                <Grid container spacing={2}>
                  {sessions.map((session) => {
                    const clientConn = memoryData?.clients.find((c) => c.session_id === session.session_id)
                    const connectedContainerId = clientConn?.app_id || clientConn?.lobby_id
                    const connectedContainer = connectedContainerId
                      ? containers.find((c: any) => c.id === connectedContainerId)
                      : null
                    const containerName = connectedContainer ? (connectedContainer.title || connectedContainer.name) : null
                    const uniqueKey = `${session.session_id}-${session.app_id}`

                    return (
                      <Grid item xs={12} md={6} key={uniqueKey}>
                        <Box
                          sx={{
                            p: 2,
                            border: connectedContainer
                              ? '1px solid rgba(33, 150, 243, 0.5)'
                              : '1px solid rgba(244, 67, 54, 0.5)',
                            borderRadius: 1,
                            backgroundColor: connectedContainer
                              ? 'rgba(33, 150, 243, 0.05)'
                              : 'rgba(244, 67, 54, 0.05)',
                          }}
                        >
                          <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                            <Box>
                              <Typography variant="subtitle2">{session.session_id}</Typography>
                              <Typography variant="caption" color="text.secondary">
                                IP: {session.client_ip}
                              </Typography>
                            </Box>
                            {containerName ? (
                              <Chip label={`→ ${containerName}`} size="small" color="primary" />
                            ) : (
                              <Chip label="Orphaned" size="small" color="error" />
                            )}
                          </Box>
                          <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 1 }}>
                            {session.display_mode.width}x{session.display_mode.height}@
                            {session.display_mode.refresh_rate} •{' '}
                            {session.display_mode.av1_supported
                              ? 'AV1'
                              : session.display_mode.hevc_supported
                              ? 'HEVC'
                              : 'H264'}
                          </Typography>
                          {!isAppsMode && (
                            <Typography
                              variant="caption"
                              color="text.secondary"
                              sx={{ display: 'block', mt: 1, fontFamily: 'monospace' }}
                            >
                              interpipesrc listen-to="{connectedContainerId || session.session_id}_video"
                            </Typography>
                          )}
                          {clientConn && (
                            <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 0.5 }}>
                              Memory: {formatBytes(clientConn.memory_bytes)}
                            </Typography>
                          )}
                        </Box>
                      </Grid>
                    )
                  })}
                </Grid>
              ) : (
                <Typography variant="body2" color="text.secondary">
                  No active streaming sessions
                </Typography>
              )}
            </CardContent>
          </Card>
        </Grid>
      </Grid>
    </Box>
  )
}

export default AgentSandboxes
