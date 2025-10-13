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

interface WolfLobbyMemory {
  lobby_id: string
  lobby_name: string
  resolution: string
  client_count: string
  memory_bytes: string
}

interface WolfClientConnection {
  session_id: string
  client_ip: string
  resolution: string
  lobby_id: string | null
  memory_bytes: string
}

interface WolfSystemMemory {
  success: boolean
  process_rss_bytes: string
  gstreamer_buffer_bytes: string
  total_memory_bytes: string
  lobbies: WolfLobbyMemory[]
  clients: WolfClientConnection[]
}

interface WolfLobbyInfo {
  id: string
  name: string
  started_by_profile_id: string
  multi_user: boolean
  stop_when_everyone_leaves: boolean
  pin?: number[]
}

interface WolfSessionInfo {
  session_id: string
  client_ip: string
  app_id: string
  display_mode: {
    width: number
    height: number
    refresh_rate: number
    hevc_supported: boolean
    av1_supported: boolean
  }
}

interface AgentSandboxesDebugResponse {
  memory: WolfSystemMemory | null
  lobbies: WolfLobbyInfo[]
  sessions: WolfSessionInfo[]
}

const formatBytes = (bytesStr: string): string => {
  const bytes = parseInt(bytesStr, 10)
  if (isNaN(bytes) || bytes === 0) return '0 Bytes'
  const k = 1024
  const sizes = ['Bytes', 'KB', 'MB', 'GB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return Math.round((bytes / Math.pow(k, i)) * 100) / 100 + ' ' + sizes[i]
}

// GStreamer Pipeline Network Visualization Component
const PipelineNetworkVisualization: FC<{ data: AgentSandboxesDebugResponse }> = ({ data }) => {
  const lobbies = data.lobbies || []
  const sessions = data.sessions || []
  const memoryData = data.memory
  const lobbyMemory = memoryData?.lobbies || []
  const clientConnections = memoryData?.clients || []

  // Build connection map: session_id -> lobby_id
  const connectionMap = new Map<string, string>()
  clientConnections.forEach((client) => {
    if (client.lobby_id) {
      connectionMap.set(client.session_id, client.lobby_id)
    }
  })

  // Layout parameters
  const svgWidth = 1000
  const svgHeight = 600
  const lobbyRadius = 60
  const sessionRadius = 30
  const lobbyY = 150
  const sessionY = 450

  // Position lobbies horizontally
  const lobbyPositions = new Map<string, { x: number; y: number }>()
  const lobbySpacing = svgWidth / (lobbies.length + 1)
  lobbies.forEach((lobby, idx) => {
    lobbyPositions.set(lobby.id, {
      x: lobbySpacing * (idx + 1),
      y: lobbyY,
    })
  })

  // Position sessions horizontally
  const sessionPositions = new Map<string, { x: number; y: number }>()
  const sessionSpacing = svgWidth / (sessions.length + 1)
  sessions.forEach((session, idx) => {
    sessionPositions.set(session.session_id, {
      x: sessionSpacing * (idx + 1),
      y: sessionY,
    })
  })

  // Find memory for each lobby
  const getLobbyMemory = (lobbyId: string): string => {
    const mem = lobbyMemory.find((lm) => lm.lobby_id === lobbyId)
    return mem ? formatBytes(mem.memory_bytes) : '0 B'
  }

  // Find client count for each lobby
  const getLobbyClientCount = (lobbyId: string): number => {
    return clientConnections.filter((c) => c.lobby_id === lobbyId).length
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
        title="GStreamer Pipeline Network"
        subheader="Interpipe connections between lobbies (producers) and sessions (consumers)"
      />
      <CardContent>
        <svg width={svgWidth} height={svgHeight} style={{ border: '1px solid rgba(255,255,255,0.1)' }}>
          {/* Draw connection lines */}
          {sessions.map((session) => {
            const connectedLobbyId = connectionMap.get(session.session_id)
            if (!connectedLobbyId) return null

            const sessionPos = sessionPositions.get(session.session_id)
            const lobbyPos = lobbyPositions.get(connectedLobbyId)
            if (!sessionPos || !lobbyPos) return null

            return (
              <g key={`connection-${session.session_id}`}>
                <line
                  x1={sessionPos.x}
                  y1={sessionPos.y - sessionRadius}
                  x2={lobbyPos.x}
                  y2={lobbyPos.y + lobbyRadius}
                  stroke="#00c8ff"
                  strokeWidth="2"
                  strokeDasharray="5,5"
                  opacity="0.6"
                />
                <text
                  x={(sessionPos.x + lobbyPos.x) / 2}
                  y={(sessionPos.y + lobbyPos.y) / 2}
                  fill="rgba(255,255,255,0.5)"
                  fontSize="10"
                  textAnchor="middle"
                >
                  interpipe
                </text>
              </g>
            )
          })}

          {/* Draw lobbies (producer pipelines) */}
          {lobbies.map((lobby) => {
            const pos = lobbyPositions.get(lobby.id)
            if (!pos) return null

            const clientCount = getLobbyClientCount(lobby.id)
            const hasClients = clientCount > 0

            return (
              <g key={`lobby-${lobby.id}`}>
                <Tooltip title={`Lobby: ${lobby.name}`} arrow>
                  <circle
                    cx={pos.x}
                    cy={pos.y}
                    r={lobbyRadius}
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
                  {lobby.name}
                </text>
                <text x={pos.x} y={pos.y + 5} textAnchor="middle" fill="rgba(255,255,255,0.7)" fontSize="10">
                  {getLobbyMemory(lobby.id)}
                </text>
                <text x={pos.x} y={pos.y + 20} textAnchor="middle" fill="rgba(255,255,255,0.5)" fontSize="9">
                  {clientCount} client{clientCount !== 1 ? 's' : ''}
                </text>
                {/* Producer pipeline indicator */}
                <rect
                  x={pos.x - 30}
                  y={pos.y - lobbyRadius - 20}
                  width="60"
                  height="15"
                  fill="rgba(103, 58, 183, 0.3)"
                  stroke="#673ab7"
                  strokeWidth="1"
                  rx="3"
                />
                <text
                  x={pos.x}
                  y={pos.y - lobbyRadius - 9}
                  textAnchor="middle"
                  fill="white"
                  fontSize="8"
                >
                  interpipesink
                </text>
              </g>
            )
          })}

          {/* Draw sessions (consumer pipelines) */}
          {sessions.map((session) => {
            const pos = sessionPositions.get(session.session_id)
            if (!pos) return null

            const connectedLobby = connectionMap.get(session.session_id)
            const isOrphaned = !connectedLobby

            return (
              <g key={`session-${session.session_id}`}>
                <Tooltip title={`Session: ${session.session_id} | IP: ${session.client_ip}`} arrow>
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
                  {session.session_id}
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
                  interpipesrc
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
              Active Lobby (has clients)
            </text>
            <circle cx="10" cy="40" r="8" fill="rgba(255, 193, 7, 0.2)" stroke="#ffc107" strokeWidth="2" />
            <text x="25" y="45" fill="rgba(255,255,255,0.8)" fontSize="10">
              Empty Lobby
            </text>
            <circle cx="10" cy="60" r="8" fill="rgba(33, 150, 243, 0.2)" stroke="#2196f3" strokeWidth="2" />
            <text x="25" y="65" fill="rgba(255,255,255,0.8)" fontSize="10">
              Connected Session
            </text>
            <circle cx="10" cy="80" r="8" fill="rgba(244, 67, 54, 0.2)" stroke="#f44336" strokeWidth="2" />
            <text x="25" y="85" fill="rgba(255,255,255,0.8)" fontSize="10">
              Orphaned Session
            </text>
          </g>
        </svg>

        <Box sx={{ mt: 2 }}>
          <Typography variant="caption" color="text.secondary">
            <strong>Architecture:</strong> Lobbies run GStreamer producer pipelines (interpipesink). Sessions
            consume streams via interpipesrc, which dynamically switches its listen-to property to connect to
            different lobbies. Lines show active interpipe connections.
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
      return response.data as AgentSandboxesDebugResponse
    },
    refetchInterval: 5000,
  })

  const memoryData = data?.memory
  const lobbies = data?.lobbies || []
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
        Monitor Wolf streaming infrastructure: GStreamer pipelines, interpipe connections, memory usage, and
        real-time streaming sessions
      </Typography>

      {error && (
        <Alert severity="error" sx={{ mb: 2 }}>
          {error instanceof Error ? error.message : 'Failed to fetch Wolf debugging data'}
        </Alert>
      )}

      <Grid container spacing={3}>
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
                    Per-Lobby Breakdown
                  </Typography>
                  {memoryData.lobbies && memoryData.lobbies.length > 0 ? (
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
                      No lobbies active
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

        {/* Active Lobbies Panel */}
        <Grid item xs={12} md={6}>
          <Card sx={{ height: '100%' }}>
            <CardHeader
              avatar={<VideocamIcon />}
              title="Active Lobbies"
              subheader="Producer pipelines with interpipesink"
            />
            <CardContent>
              {lobbies.length > 0 ? (
                <Box>
                  {lobbies.map((lobby) => (
                    <Box
                      key={lobby.id}
                      sx={{
                        mb: 2,
                        p: 2,
                        border: '1px solid rgba(255,255,255,0.1)',
                        borderRadius: 1,
                      }}
                    >
                      <Typography variant="subtitle1" fontWeight="bold">
                        {lobby.name}
                      </Typography>
                      <Typography variant="caption" sx={{ fontFamily: 'monospace', color: 'text.secondary' }}>
                        {lobby.id}
                      </Typography>
                      <Box sx={{ mt: 1, display: 'flex', gap: 1, flexWrap: 'wrap' }}>
                        <Chip
                          label={lobby.multi_user ? 'Multi-User' : 'Single-User'}
                          size="small"
                          color={lobby.multi_user ? 'success' : 'default'}
                        />
                        {lobby.stop_when_everyone_leaves && (
                          <Chip label="Auto-Stop" size="small" color="warning" />
                        )}
                      </Box>
                      <Typography variant="caption" color="text.secondary" sx={{ mt: 1, display: 'block' }}>
                        GStreamer: interpipesink name="{lobby.id}_video" | interpipesink name="{lobby.id}_audio"
                      </Typography>
                    </Box>
                  ))}
                </Box>
              ) : (
                <Typography variant="body2" color="text.secondary">
                  No active lobbies
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
              subheader="Consumer pipelines with interpipesrc"
            />
            <CardContent>
              {sessions.length > 0 ? (
                <Grid container spacing={2}>
                  {sessions.map((session) => {
                    const clientConn = memoryData?.clients.find((c) => c.session_id === session.session_id)
                    const connectedLobby = clientConn?.lobby_id
                      ? lobbies.find((l) => l.id === clientConn.lobby_id)
                      : null

                    return (
                      <Grid item xs={12} md={6} key={session.session_id}>
                        <Box
                          sx={{
                            p: 2,
                            border: connectedLobby
                              ? '1px solid rgba(33, 150, 243, 0.5)'
                              : '1px solid rgba(244, 67, 54, 0.5)',
                            borderRadius: 1,
                            backgroundColor: connectedLobby
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
                            {connectedLobby ? (
                              <Chip label={`→ ${connectedLobby.name}`} size="small" color="primary" />
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
                          <Typography
                            variant="caption"
                            color="text.secondary"
                            sx={{ display: 'block', mt: 1, fontFamily: 'monospace' }}
                          >
                            interpipesrc listen-to="{connectedLobby ? connectedLobby.id : session.session_id}
                            _video"
                          </Typography>
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
