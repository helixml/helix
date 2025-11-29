import React, { FC, useEffect, useState } from 'react'
import {
  Box,
  Paper,
  Typography,
  Card,
  CardContent,
  CardHeader,
  Chip,
  IconButton,
  Alert,
  CircularProgress,
  Grid,
  Tooltip,
} from '@mui/material'
import RefreshIcon from '@mui/icons-material/Refresh'
import VideocamIcon from '@mui/icons-material/Videocam'
import PlayArrowIcon from '@mui/icons-material/PlayArrow'
import PauseIcon from '@mui/icons-material/Pause'
import ErrorIcon from '@mui/icons-material/Error'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import useApi from '../../hooks/useApi'

interface MoonlightSession {
  session_id: string
  client_unique_id: string | null
  mode: string
  has_websocket: boolean
  streamer_pid: number | null
  streamer_alive: boolean
}

interface MoonlightClient {
  client_unique_id: string
  has_certificate: boolean
}

interface MoonlightStatusResponse {
  total_clients: number
  total_sessions: number
  active_websockets: number
  clients: MoonlightClient[]
  sessions: MoonlightSession[]
}

interface WolfLobbyMemory {
  lobby_id: string
  lobby_name: string
  resolution: string
  client_count: number
  memory_bytes: number
}

interface WolfAppMemory {
  app_id: string
  app_name: string
  resolution: string
  client_count: number
  memory_bytes: number
}

interface WolfClientConnection {
  session_id: string
  client_ip: string
  resolution: string
  lobby_id?: string
  app_id?: string
  memory_bytes: number
}

interface WolfMemoryData {
  success: boolean
  apps: WolfAppMemory[]
  lobbies: WolfLobbyMemory[]
  clients: WolfClientConnection[]
}

// Wolf streaming session with ENET activity tracking
interface WolfSession {
  session_id: string
  client_unique_id?: string
  client_ip: string
  app_id: string
  lobby_id?: string
  idle_seconds: number  // Seconds since last ENET packet (session timeout at 60s)
  display_mode: {
    width: number
    height: number
    refresh_rate: number
    hevc_supported: boolean
    av1_supported: boolean
  }
}

interface MoonlightMonitorProps {
  sandboxInstanceId: string;
}

const MoonlightMonitor: FC<MoonlightMonitorProps> = ({ sandboxInstanceId }) => {
  const api = useApi()
  const apiClient = api.getApiClient()
  const queryClient = useQueryClient()
  const [wolfStates, setWolfStates] = useState<Record<string, string>>({})

  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['moonlight-status', sandboxInstanceId],
    queryFn: async () => {
      if (!sandboxInstanceId) return null
      try {
        const response = await api.get<MoonlightStatusResponse>(`/api/v1/moonlight/status?wolf_instance_id=${sandboxInstanceId}`)
        // Handle both direct data and wrapped response
        if (response && response.data) {
          return response.data
        } else if (response) {
          return response as MoonlightStatusResponse
        }
        throw new Error('No data returned from moonlight status endpoint')
      } catch (err: any) {
        console.error('Moonlight status fetch error:', err)
        throw err
      }
    },
    enabled: !!sandboxInstanceId,
    refetchInterval: 3000, // Poll every 3 seconds
    retry: 1,
  })

  // Fetch Wolf live state using generated client + React Query
  const { data: wolfDebugData } = useQuery({
    queryKey: ['wolf-debug', sandboxInstanceId],
    queryFn: async () => {
      if (!sandboxInstanceId) return null
      const response = await apiClient.v1AdminAgentSandboxesDebugList({ wolf_instance_id: sandboxInstanceId })
      return response.data
    },
    enabled: !!sandboxInstanceId,
    refetchInterval: 3000,
    retry: 1,
  })

  const wolfMemory = wolfDebugData?.memory || null

  // Extract Helix session ID from Moonlight session_id
  // Format: "agent-ses_01k9g7cx800pd7p3m5sw9r2301-{instance_id}"
  const extractHelixSessionId = (moonlightSessionId: string): string | null => {
    const match = moonlightSessionId.match(/agent-(ses_[a-z0-9]+)-/)
    return match ? match[1] : null
  }

  // Fetch Wolf app state for all sessions
  useEffect(() => {
    if (!data?.sessions) return

    const fetchWolfStates = async () => {
      const states: Record<string, string> = {}

      for (const session of data.sessions) {
        const helixSessionId = extractHelixSessionId(session.session_id)
        if (!helixSessionId) continue

        try {
          const response = await api.get(`/api/v1/sessions/${helixSessionId}/wolf-app-state`)
          if (response && response.data) {
            states[session.session_id] = response.data.state || 'unknown'
          } else {
            states[session.session_id] = 'unknown'
          }
        } catch (err) {
          states[session.session_id] = 'error'
        }
      }

      setWolfStates(states)
    }

    fetchWolfStates()
  }, [data?.sessions])

  const handleRefresh = () => {
    queryClient.invalidateQueries({ queryKey: ['moonlight-status', sandboxInstanceId] })
    queryClient.invalidateQueries({ queryKey: ['wolf-debug', sandboxInstanceId] })
  }

  // Show message if no sandbox selected
  if (!sandboxInstanceId) {
    return (
      <Paper sx={{ p: 3 }}>
        <Alert severity="info">
          Select an agent sandbox from the dropdown above to view Moonlight status.
        </Alert>
      </Paper>
    )
  }

  if (isLoading) {
    return (
      <Box sx={{ display: 'flex', justifyContent: 'center', p: 4 }}>
        <CircularProgress />
      </Box>
    )
  }

  if (error) {
    const errorMessage = error instanceof Error
      ? error.message
      : JSON.stringify(error)

    return (
      <Alert severity="error">
        <Typography variant="subtitle2">Failed to load Moonlight status</Typography>
        <Typography variant="caption" display="block" sx={{ fontFamily: 'monospace', mt: 1 }}>
          {errorMessage}
        </Typography>
      </Alert>
    )
  }

  // Handle undefined data gracefully
  if (!data) {
    return (
      <Paper sx={{ p: 3 }}>
        <Alert severity="info">No Moonlight status data available. Moonlight-web may be starting up.</Alert>
      </Paper>
    )
  }

  const sessions = data.sessions || []
  const clients = data.clients || []

  // Calculate health metrics
  const zombieSessions = sessions.filter(s => !s.streamer_alive)
  const orphanedSessions = sessions.filter(s => !s.has_websocket && s.streamer_alive)
  const healthySessions = sessions.filter(s => s.has_websocket && s.streamer_alive)

  return (
    <Paper sx={{ p: 3 }}>
      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 3 }}>
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 2 }}>
          <VideocamIcon color="primary" fontSize="large" />
          <Typography variant="h5">Moonlight Web Streaming Status</Typography>
        </Box>
        <IconButton onClick={handleRefresh}>
          <RefreshIcon />
        </IconButton>
      </Box>

      {/* Health Alerts */}
      {zombieSessions.length > 0 && (
        <Alert severity="error" sx={{ mb: 2 }}>
          {zombieSessions.length} zombie session(s) detected - streamer process dead but session not cleaned up
        </Alert>
      )}
      {orphanedSessions.length > 5 && (
        <Alert severity="warning" sx={{ mb: 2 }}>
          {orphanedSessions.length} orphaned session(s) - no WebSocket but process still running (may be normal for keepalive)
        </Alert>
      )}

      {/* Summary Stats */}
      <Grid container spacing={2} sx={{ mb: 3 }}>
        <Grid item xs={12} md={2.4}>
          <Card variant="outlined">
            <CardContent>
              <Typography color="textSecondary" gutterBottom>
                Total Clients
              </Typography>
              <Typography variant="h4">{data?.total_clients || 0}</Typography>
              <Typography variant="caption" color="textSecondary">
                Certificates cached
              </Typography>
            </CardContent>
          </Card>
        </Grid>
        <Grid item xs={12} md={2.4}>
          <Card variant="outlined">
            <CardContent>
              <Typography color="textSecondary" gutterBottom>
                Total Sessions
              </Typography>
              <Typography variant="h4">{data?.total_sessions || 0}</Typography>
              <Typography variant="caption" color="textSecondary">
                Streamer processes
              </Typography>
            </CardContent>
          </Card>
        </Grid>
        <Grid item xs={12} md={2.4}>
          <Card variant="outlined" sx={{ borderColor: 'success.main' }}>
            <CardContent>
              <Typography color="success.main" gutterBottom sx={{ fontWeight: 600 }}>
                Healthy
              </Typography>
              <Typography variant="h4" color="success.main">{healthySessions.length}</Typography>
              <Typography variant="caption" color="textSecondary">
                WS + Process alive
              </Typography>
            </CardContent>
          </Card>
        </Grid>
        <Grid item xs={12} md={2.4}>
          <Card variant="outlined" sx={{ borderColor: 'warning.main' }}>
            <CardContent>
              <Typography color="warning.main" gutterBottom sx={{ fontWeight: 600 }}>
                Orphaned
              </Typography>
              <Typography variant="h4" color="warning.main">{orphanedSessions.length}</Typography>
              <Typography variant="caption" color="textSecondary">
                No WS, process alive
              </Typography>
            </CardContent>
          </Card>
        </Grid>
        <Grid item xs={12} md={2.4}>
          <Card variant="outlined" sx={{ borderColor: 'error.main' }}>
            <CardContent>
              <Typography color="error.main" gutterBottom sx={{ fontWeight: 600 }}>
                Zombie
              </Typography>
              <Typography variant="h4" color="error.main">{zombieSessions.length}</Typography>
              <Typography variant="caption" color="textSecondary">
                Process dead
              </Typography>
            </CardContent>
          </Card>
        </Grid>
      </Grid>

      {/* Moonlight-web vs Wolf Comparison */}
      <Box sx={{ mt: 4, mb: 4 }}>
        <Typography variant="h6" gutterBottom>
          Moonlight-web vs Wolf State Comparison
        </Typography>
        <Typography variant="body2" color="textSecondary" sx={{ mb: 2 }}>
          Verify consistency between what Moonlight-web knows and what Wolf reports
        </Typography>

        <Grid container spacing={2}>
          <Grid item xs={12} md={6}>
            <Paper variant="outlined" sx={{ p: 2 }}>
              <Typography variant="subtitle1" sx={{ fontWeight: 600, mb: 2 }}>
                Moonlight-web's View
              </Typography>
              <Typography variant="body2" color="textSecondary" sx={{ mb: 1 }}>
                Active Sessions: {data?.total_sessions || 0}
              </Typography>
              <Typography variant="body2" color="textSecondary" sx={{ mb: 1 }}>
                Cached Clients: {data?.total_clients || 0}
              </Typography>
              <Typography variant="body2" color="textSecondary" sx={{ mb: 2 }}>
                Active WebSockets: {data?.active_websockets || 0}
              </Typography>

              {sessions.length > 0 ? (
                <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1 }}>
                  {sessions.map((session) => {
                    const helixSessionId = extractHelixSessionId(session.session_id)
                    const wolfState = session.session_id ? (wolfStates[session.session_id] || 'loading') : 'unknown'

                    // Find Wolf client connection for this Moonlight session
                    // Wolf clients are identified by session_id (which is Wolf's internal client ID)
                    const wolfClient = wolfMemory?.clients?.find(c =>
                      // Try to match by checking if this could be the same session
                      // This is tricky - need to understand the mapping better
                      session.client_unique_id?.includes(helixSessionId || '')
                    )

                    // Find lobby this session is targeting
                    const targetLobbyId = wolfClient?.lobby_id
                    const targetLobby = targetLobbyId ? wolfMemory?.lobbies?.find(l => l.lobby_id === targetLobbyId) : null

                    return (
                      <Box key={session.session_id} sx={{ p: 1, bgcolor: 'background.default', borderRadius: 1 }}>
                        <Typography variant="caption" sx={{ fontFamily: 'monospace', display: 'block', mb: 0.5 }}>
                          Session: {helixSessionId?.slice(-8) || 'unknown'}
                        </Typography>
                        <Typography variant="caption" color="textSecondary" sx={{ display: 'block', mb: 0.5, fontSize: '0.7rem' }}>
                          Moonlight session: {session.session_id.slice(0, 30)}...
                        </Typography>
                        {targetLobby && (
                          <Typography variant="caption" color="success.main" sx={{ display: 'block', mb: 0.5, fontSize: '0.7rem' }}>
                            → Wolf lobby: {targetLobby.lobby_name} ({targetLobby.lobby_id.slice(0, 8)}...)
                          </Typography>
                        )}
                        <Box sx={{ display: 'flex', gap: 1, mt: 0.5, flexWrap: 'wrap' }}>
                          {session.has_websocket ? (
                            <Chip label="Streaming" size="small" color="success" />
                          ) : (
                            <Chip label="Idle" size="small" color="default" />
                          )}
                          <Chip label={session.mode.toUpperCase()} size="small" variant="outlined" />
                          {wolfState === 'running' && <Chip label="Wolf: Streaming" size="small" color="success" variant="outlined" />}
                          {wolfState === 'resumable' && <Chip label="Wolf: Ready" size="small" color="info" variant="outlined" />}
                        </Box>
                      </Box>
                    )
                  })}
                </Box>
              ) : (
                <Typography variant="caption" color="textSecondary">No active sessions</Typography>
              )}
            </Paper>
          </Grid>

          <Grid item xs={12} md={6}>
            <Paper variant="outlined" sx={{ p: 2 }}>
              <Typography variant="subtitle1" sx={{ fontWeight: 600, mb: 2 }}>
                Wolf's View (Live)
              </Typography>
              <Typography variant="body2" color="textSecondary" sx={{ mb: 1 }}>
                Apps: {wolfMemory?.apps?.length || 0} | Lobbies: {wolfMemory?.lobbies?.length || 0}
              </Typography>
              <Typography variant="body2" color="textSecondary" sx={{ mb: 2 }}>
                Connected Clients: {wolfMemory?.clients?.length || 0}
              </Typography>

              {/* Wolf Apps */}
              {wolfMemory?.apps && wolfMemory.apps.length > 0 && (
                <Box sx={{ mb: 2 }}>
                  <Typography variant="caption" sx={{ fontWeight: 600, display: 'block', mb: 1 }}>
                    Wolf Apps:
                  </Typography>
                  <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1 }}>
                    {wolfMemory.apps.map((app) => {
                      // Find clients connected to this app
                      const appClients = wolfMemory.clients?.filter(c => c.app_id === app.app_id) || []

                      return (
                        <Box key={app.app_id} sx={{ p: 1, bgcolor: 'background.default', borderRadius: 1, border: '1px solid', borderColor: 'primary.main' }}>
                          <Typography variant="caption" sx={{ fontFamily: 'monospace', display: 'block', mb: 0.5, fontWeight: 600 }}>
                            {app.app_name}
                          </Typography>
                          <Typography variant="caption" color="textSecondary" sx={{ display: 'block', mb: 0.5, fontSize: '0.7rem' }}>
                            App ID: {app.app_id}
                          </Typography>
                          <Box sx={{ display: 'flex', gap: 1, mt: 0.5, flexWrap: 'wrap' }}>
                            <Chip
                              label={`${app.client_count} client${app.client_count !== 1 ? 's' : ''}`}
                              size="small"
                              color={app.client_count > 0 ? "success" : "default"}
                            />
                            {app.resolution !== 'N/A' && (
                              <Chip label={app.resolution} size="small" variant="outlined" />
                            )}
                          </Box>
                          {/* Show which Wolf clients are connected to this app */}
                          {appClients.length > 0 && (
                            <Box sx={{ mt: 1, pl: 1, borderLeft: '2px solid', borderColor: 'success.main' }}>
                              <Typography variant="caption" sx={{ fontSize: '0.65rem', fontWeight: 600, display: 'block', mb: 0.5 }}>
                                Connected Wolf Clients:
                              </Typography>
                              {appClients.map(client => (
                                <Box key={client.session_id} sx={{ mb: 0.5 }}>
                                  <Typography variant="caption" sx={{ display: 'block', fontSize: '0.65rem', fontFamily: 'monospace' }}>
                                    Session: {client.session_id}
                                  </Typography>
                                  <Typography variant="caption" color="textSecondary" sx={{ fontSize: '0.6rem' }}>
                                    {client.resolution} | {client.client_ip}
                                  </Typography>
                                </Box>
                              ))}
                            </Box>
                          )}
                        </Box>
                      )
                    })}
                  </Box>
                </Box>
              )}

              {/* Wolf Lobbies */}
              {wolfMemory?.lobbies && wolfMemory.lobbies.length > 0 && (
                <Box>
                  <Typography variant="caption" sx={{ fontWeight: 600, display: 'block', mb: 1 }}>
                    Wolf Lobbies:
                  </Typography>
                  <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1 }}>
                    {wolfMemory.lobbies.map((lobby) => {
                      // Find clients connected to this lobby
                      const connectedClients = wolfMemory.clients?.filter(c => c.lobby_id === lobby.lobby_id) || []

                      return (
                        <Box key={lobby.lobby_id} sx={{ p: 1, bgcolor: 'background.default', borderRadius: 1 }}>
                          <Typography variant="caption" sx={{ fontFamily: 'monospace', display: 'block', mb: 0.5 }}>
                            {lobby.lobby_name}
                          </Typography>
                          <Typography variant="caption" color="textSecondary" sx={{ display: 'block', mb: 0.5, fontSize: '0.7rem' }}>
                            Lobby ID: {lobby.lobby_id.slice(0, 8)}...
                          </Typography>
                          <Box sx={{ display: 'flex', gap: 1, mt: 0.5, flexWrap: 'wrap' }}>
                            <Chip
                              label={`${lobby.client_count} client${lobby.client_count !== 1 ? 's' : ''}`}
                              size="small"
                              color={lobby.client_count > 0 ? "success" : "default"}
                            />
                            {lobby.resolution !== 'N/A' && (
                              <Chip label={lobby.resolution} size="small" variant="outlined" />
                            )}
                          </Box>
                          {/* Show which Wolf clients are connected to this lobby */}
                          {connectedClients.length > 0 && (
                            <Box sx={{ mt: 1, pl: 1, borderLeft: '2px solid', borderColor: 'primary.main' }}>
                              {connectedClients.map(client => (
                                <Typography key={client.session_id} variant="caption" sx={{ display: 'block', fontSize: '0.65rem', fontFamily: 'monospace' }}>
                                  → Wolf session: {client.session_id}
                                </Typography>
                              ))}
                            </Box>
                          )}
                        </Box>
                      )
                    })}
                  </Box>
                </Box>
              )}

              {!wolfMemory?.apps?.length && !wolfMemory?.lobbies?.length && (
                <Typography variant="caption" color="textSecondary">No active apps or lobbies</Typography>
              )}
            </Paper>
          </Grid>
        </Grid>

        {/* Mismatch detection - only warn if streaming to non-existent lobbies */}
        {(() => {
          const moonlightSessionCount = sessions.length
          const wolfLobbyCount = wolfMemory?.lobbies?.length || 0
          const wolfClientCount = wolfMemory?.clients?.length || 0

          // PROBLEM: More Moonlight sessions than Wolf lobbies (streaming to ghost lobby)
          if (moonlightSessionCount > wolfLobbyCount) {
            return (
              <Alert severity="error" sx={{ mt: 2 }}>
                ⚠️ Critical: Moonlight has {moonlightSessionCount} session(s) but Wolf only has {wolfLobbyCount} lobby/lobbies - sessions streaming to non-existent lobbies!
              </Alert>
            )
          }

          // INFO: More lobbies than sessions (normal - agents running but not being viewed)
          if (wolfLobbyCount > moonlightSessionCount && moonlightSessionCount === 0) {
            return (
              <Alert severity="info" sx={{ mt: 2 }}>
                {wolfLobbyCount} agent(s) running but no browser connections
              </Alert>
            )
          }

          return null
        })()}
      </Box>

      {/* Client-Centric State Table */}
      <Box sx={{ mt: 4, mb: 4 }}>
        <Typography variant="h6" gutterBottom>
          Per-Client State: Moonlight → Wolf (Live)
        </Typography>
        <Typography variant="body2" color="textSecondary" sx={{ mb: 2 }}>
          Each Moonlight client can only stream one session at a time. Shows Wolf UI app state and current lobby.
        </Typography>

        <Paper variant="outlined" sx={{ overflowX: 'auto' }}>
          <Box component="table" sx={{ width: '100%', borderCollapse: 'collapse' }}>
            <Box component="thead">
              <Box component="tr" sx={{ bgcolor: 'action.hover' }}>
                <Box component="th" sx={{ p: 2, textAlign: 'left', fontWeight: 600, borderBottom: '2px solid', borderColor: 'divider', minWidth: 200 }}>
                  <Typography variant="subtitle2">Moonlight Client</Typography>
                </Box>
                <Box component="th" sx={{ p: 2, textAlign: 'left', fontWeight: 600, borderBottom: '2px solid', borderColor: 'divider' }}>
                  <Typography variant="subtitle2">Wolf UI App State</Typography>
                </Box>
                <Box component="th" sx={{ p: 2, textAlign: 'left', fontWeight: 600, borderBottom: '2px solid', borderColor: 'divider' }}>
                  <Typography variant="subtitle2">Current Lobby</Typography>
                </Box>
                <Box component="th" sx={{ p: 2, textAlign: 'left', fontWeight: 600, borderBottom: '2px solid', borderColor: 'divider' }}>
                  <Typography variant="subtitle2">ENET Idle</Typography>
                </Box>
                <Box component="th" sx={{ p: 2, textAlign: 'left', fontWeight: 600, borderBottom: '2px solid', borderColor: 'divider' }}>
                  <Typography variant="subtitle2">Moonlight Session</Typography>
                </Box>
              </Box>
            </Box>
            <Box component="tbody">
              {clients.map((client) => {
                const clientSessions = sessions.filter(s => s.client_unique_id === client.client_unique_id)
                const activeSession = clientSessions.find(s => s.has_websocket)
                const helixSessionId = activeSession ? extractHelixSessionId(activeSession.session_id) : null

                // Find Wolf session for this Moonlight client using client_unique_id correlation
                const wolfSession = wolfDebugData?.sessions?.find(s =>
                  s.client_unique_id === client.client_unique_id
                )

                // Get lobby_id from Wolf memory clients (has the actual lobby_id from Wolf)
                const wolfClient = wolfSession ? wolfMemory?.clients?.find(c =>
                  c.session_id === wolfSession.session_id
                ) : null

                // Check if this client has a Wolf UI app session
                // Wolf UI app = app_id 134906179
                const hasWolfUISession = wolfSession && wolfSession.app_id === '134906179'

                // Check if this client is in a lobby (from Wolf's live data)
                const currentLobbyId = wolfClient?.lobby_id
                const currentLobby = currentLobbyId ? wolfMemory?.lobbies?.find(l => l.lobby_id === currentLobbyId) : null

                // Determine Wolf UI app state from this client's perspective
                let wolfUIState = 'stopped'
                if (hasWolfUISession) {
                  // Has Wolf UI session - check if actively streaming or just resumable
                  wolfUIState = activeSession ? 'connected' : 'resumable'
                }

                return (
                  <Box component="tr" key={client.client_unique_id} sx={{ '&:hover': { bgcolor: 'action.hover' } }}>
                    <Box component="td" sx={{ p: 2, borderBottom: '1px solid', borderColor: 'divider', verticalAlign: 'top' }}>
                      <Typography variant="caption" sx={{ fontFamily: 'monospace', display: 'block', fontSize: '0.7rem', wordBreak: 'break-all' }}>
                        {client.client_unique_id}
                      </Typography>
                      <Typography variant="caption" color="textSecondary" sx={{ fontSize: '0.65rem', display: 'block', mt: 0.5 }}>
                        Helix: {helixSessionId?.slice(-8) || 'N/A'}
                      </Typography>
                    </Box>
                    <Box component="td" sx={{ p: 2, borderBottom: '1px solid', borderColor: 'divider', verticalAlign: 'top' }}>
                      {wolfUIState === 'connected' ? (
                        <Box>
                          <Chip icon={<PlayArrowIcon />} label="CONNECTED" size="small" color="success" />
                          <Typography variant="caption" color="textSecondary" sx={{ display: 'block', mt: 0.5, fontSize: '0.65rem' }}>
                            Actively streaming
                          </Typography>
                        </Box>
                      ) : wolfUIState === 'resumable' ? (
                        <Box>
                          <Chip icon={<PauseIcon />} label="RESUMABLE" size="small" color="info" />
                          <Typography variant="caption" color="textSecondary" sx={{ display: 'block', mt: 0.5, fontSize: '0.65rem' }}>
                            Container running (shows play/stop)
                          </Typography>
                        </Box>
                      ) : (
                        <Box>
                          <Chip label="STOPPED" size="small" variant="outlined" />
                          <Typography variant="caption" color="textSecondary" sx={{ display: 'block', mt: 0.5, fontSize: '0.65rem' }}>
                            No session (click starts fresh)
                          </Typography>
                        </Box>
                      )}
                    </Box>
                    <Box component="td" sx={{ p: 2, borderBottom: '1px solid', borderColor: 'divider', verticalAlign: 'top' }}>
                      {currentLobby ? (
                        <Box>
                          <Typography variant="caption" sx={{ fontFamily: 'monospace', display: 'block', fontWeight: 600 }}>
                            {currentLobby.lobby_name}
                          </Typography>
                          <Typography variant="caption" color="textSecondary" sx={{ fontSize: '0.65rem' }}>
                            {currentLobby.client_count} client(s) | Lobby {currentLobby.lobby_id.slice(0, 8)}
                          </Typography>
                        </Box>
                      ) : (
                        <Typography variant="caption" color="textSecondary">
                          {hasWolfUISession ? 'In lobby selector' : 'Not connected'}
                        </Typography>
                      )}
                    </Box>
                    <Box component="td" sx={{ p: 2, borderBottom: '1px solid', borderColor: 'divider', verticalAlign: 'top' }}>
                      {/* ENET Idle Time - shows seconds since last packet from client */}
                      {wolfSession ? (
                        <Box>
                          {(() => {
                            const idleSeconds = (wolfSession as WolfSession).idle_seconds || 0
                            // Color coding: green <30s, yellow 30-50s, red >50s (timeout at 60s)
                            const color = idleSeconds < 30 ? 'success' : idleSeconds < 50 ? 'warning' : 'error'
                            return (
                              <>
                                <Chip
                                  label={`${idleSeconds}s`}
                                  size="small"
                                  color={color}
                                  variant={idleSeconds < 30 ? 'outlined' : 'filled'}
                                />
                                {idleSeconds > 50 && (
                                  <Typography variant="caption" color="error" sx={{ display: 'block', mt: 0.5, fontSize: '0.6rem' }}>
                                    Timeout in {60 - idleSeconds}s!
                                  </Typography>
                                )}
                              </>
                            )
                          })()}
                        </Box>
                      ) : (
                        <Typography variant="caption" color="textSecondary">N/A</Typography>
                      )}
                    </Box>
                    <Box component="td" sx={{ p: 2, borderBottom: '1px solid', borderColor: 'divider', verticalAlign: 'top' }}>
                      {activeSession ? (
                        <Box>
                          <Chip label="STREAMING" size="small" color="success" />
                          <Typography variant="caption" sx={{ display: 'block', mt: 0.5, fontSize: '0.65rem', fontFamily: 'monospace' }}>
                            {activeSession.mode.toUpperCase()}
                          </Typography>
                        </Box>
                      ) : clientSessions.length > 0 ? (
                        <Chip label="Idle" size="small" color="default" />
                      ) : (
                        <Chip label="Cert Only" size="small" variant="outlined" />
                      )}
                    </Box>
                  </Box>
                )
              })}
            </Box>
          </Box>
        </Paper>
      </Box>

      {/* Clients and Their Sessions */}
      <Typography variant="h6" gutterBottom>
        Moonlight Clients and Their Sessions (Detailed)
      </Typography>
      <Typography variant="body2" color="textSecondary" sx={{ mb: 2 }}>
        Grouped by unique client (browser tab). Each client can have multiple sessions over time.
      </Typography>
      {clients.length === 0 ? (
        <Alert severity="info">No Moonlight clients</Alert>
      ) : (
        <Box sx={{ display: 'flex', flexDirection: 'column', gap: 3 }}>
          {clients.map((client) => {
            // Find all sessions for this client
            const clientSessions = sessions.filter(s => s.client_unique_id === client.client_unique_id)
            const hasActiveSessions = clientSessions.length > 0

            return (
              <Card key={client.client_unique_id} variant="outlined">
                <CardHeader
                  title={
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, flexWrap: 'wrap' }}>
                      <Typography variant="subtitle2" sx={{ fontFamily: 'monospace', fontSize: '0.85rem', flex: 1, wordBreak: 'break-all' }}>
                        Client: {client.client_unique_id}
                      </Typography>
                      <Chip
                        label={`${clientSessions.length} SESSION${clientSessions.length !== 1 ? 'S' : ''}`}
                        size="small"
                        color={hasActiveSessions ? 'primary' : 'default'}
                      />
                    </Box>
                  }
                />
                <CardContent>
                  {clientSessions.length === 0 ? (
                    <Typography variant="caption" color="textSecondary">
                      No active sessions (certificate cached for resume)
                    </Typography>
                  ) : (
                    <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
                      {clientSessions.map((session) => {
                        const helixSessionId = extractHelixSessionId(session.session_id)
                        const wolfState = wolfStates[session.session_id] || 'loading'

                        const getWolfStateChip = () => {
                          switch (wolfState) {
                            case 'running':
                              return (
                                <Tooltip title="Actively streaming to client">
                                  <Chip icon={<PlayArrowIcon />} label="STREAMING" size="small" color="success" />
                                </Tooltip>
                              )
                            case 'resumable':
                              return (
                                <Tooltip title="Container running, ready to connect">
                                  <Chip label="READY" size="small" color="info" />
                                </Tooltip>
                              )
                            case 'absent':
                              return (
                                <Tooltip title="Container stopped">
                                  <Chip icon={<ErrorIcon />} label="STOPPED" size="small" color="error" />
                                </Tooltip>
                              )
                            case 'loading':
                              return <Chip label="..." size="small" variant="outlined" />
                            default:
                              return <Chip label={wolfState.toUpperCase()} size="small" variant="outlined" />
                          }
                        }

                        // In lobbies mode, mode should always be "create"
                        // If it's not, that's unexpected and may indicate a bug
                        const isNormalMode = session.mode === 'create'
                        const modeLabel = session.mode.toUpperCase()

                        return (
                          <Box key={session.session_id} sx={{ p: 2, backgroundColor: 'background.paper', borderRadius: 1, border: '1px solid', borderColor: 'divider' }}>
                            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, flexWrap: 'wrap', mb: 1 }}>
                              <Typography variant="caption" sx={{ fontFamily: 'monospace', fontSize: '0.8rem' }}>
                                {session.session_id}
                              </Typography>
                              {isNormalMode ? (
                                <Chip
                                  label="CREATE"
                                  size="small"
                                  color="default"
                                  variant="outlined"
                                />
                              ) : (
                                <Tooltip title={`Unexpected mode in lobbies: ${modeLabel} (should be CREATE)`}>
                                  <Chip
                                    label={`⚠️ ${modeLabel}`}
                                    size="small"
                                    color="warning"
                                  />
                                </Tooltip>
                              )}
                              {session.has_websocket ? (
                                <Chip label="WS ACTIVE" size="small" color="success" />
                              ) : (
                                <Chip label="WS IDLE" size="small" color="default" />
                              )}
                              {getWolfStateChip()}
                            </Box>
                            <Box sx={{ display: 'flex', gap: 2, flexWrap: 'wrap' }}>
                              <Typography variant="caption" color="textSecondary" sx={{ fontFamily: 'monospace' }}>
                                Helix: {helixSessionId || 'N/A'}
                              </Typography>
                              <Typography variant="caption" color="textSecondary" sx={{ fontFamily: 'monospace' }}>
                                PID: {session.streamer_pid || 'N/A'}
                              </Typography>
                              {session.streamer_alive ? (
                                <Chip label="ALIVE" size="small" color="success" variant="outlined" sx={{ height: 20 }} />
                              ) : (
                                <Chip label="DEAD" size="small" color="error" icon={<ErrorIcon fontSize="small" />} sx={{ height: 20 }} />
                              )}
                            </Box>
                          </Box>
                        )
                      })}
                    </Box>
                  )}
                </CardContent>
              </Card>
            )
          })}
        </Box>
      )}
    </Paper>
  )
}

export default MoonlightMonitor
