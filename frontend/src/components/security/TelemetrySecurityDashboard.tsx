import React, { useEffect, useState } from 'react'
import {
  Box,
  Button,
  Card,
  CardContent,
  Chip,
  CircularProgress,
  Grid,
  IconButton,
  Paper,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Tooltip,
  Typography,
} from '@mui/material'
import RefreshIcon from '@mui/icons-material/Refresh'
import SecurityIcon from '@mui/icons-material/Security'
import BlockIcon from '@mui/icons-material/Block'
import CheckCircleIcon from '@mui/icons-material/CheckCircle'
import WarningIcon from '@mui/icons-material/Warning'
import ErrorIcon from '@mui/icons-material/Error'
import InfoIcon from '@mui/icons-material/Info'

interface TelemetryCounterRule {
  rule: string
  packets_blocked: number
  bytes_blocked: number
  agent: string
  endpoint_type: string
}

interface AgentConfig {
  name: string
  config_path: string
  telemetry_disabled: boolean
  auto_update_disabled: boolean
  config_verified: boolean
  issues: string[]
}

interface PhoneHomeAttempt {
  timestamp: string
  agent: string
  endpoint: string
  packet_count: number
  bytes_blocked: number
  severity: string
}

interface TelemetryStatus {
  timestamp: string
  telemetry_blocked: boolean
  total_blocked_packets: number
  total_blocked_bytes: number
  rules: TelemetryCounterRule[]
  agent_configurations: Record<string, AgentConfig>
  phone_home_attempts: PhoneHomeAttempt[]
  last_firewall_update: string
  security_recommendations: string[]
}

const TelemetrySecurityDashboard: React.FC = () => {
  const [status, setStatus] = useState<TelemetryStatus | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [autoRefresh, setAutoRefresh] = useState(true)

  const fetchStatus = async () => {
    try {
      const response = await fetch('/api/v1/security/telemetry-status')
      if (!response.ok) {
        throw new Error(`HTTP ${response.status}: ${response.statusText}`)
      }
      const data = await response.json()
      setStatus(data)
      setError(null)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to fetch telemetry status')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    fetchStatus()
  }, [])

  useEffect(() => {
    if (autoRefresh) {
      const interval = setInterval(fetchStatus, 30000) // Refresh every 30 seconds
      return () => clearInterval(interval)
    }
  }, [autoRefresh])

  const formatBytes = (bytes: number): string => {
    if (bytes === 0) return '0 B'
    const k = 1024
    const sizes = ['B', 'KB', 'MB', 'GB']
    const i = Math.floor(Math.log(bytes) / Math.log(k))
    return `${(bytes / Math.pow(k, i)).toFixed(2)} ${sizes[i]}`
  }

  const getSeverityColor = (severity: string) => {
    switch (severity) {
      case 'critical': return 'error'
      case 'warning': return 'warning'
      case 'info': return 'info'
      default: return 'default'
    }
  }

  const getSeverityIcon = (severity: string) => {
    switch (severity) {
      case 'critical': return <ErrorIcon color="error" />
      case 'warning': return <WarningIcon color="warning" />
      case 'info': return <InfoIcon color="info" />
      default: return <InfoIcon />
    }
  }

  const getAgentStatusIcon = (config: AgentConfig) => {
    if (config.config_verified && config.issues.length === 0) {
      return <CheckCircleIcon color="success" />
    }
    if (config.issues.length > 0) {
      return <ErrorIcon color="error" />
    }
    return <WarningIcon color="warning" />
  }

  if (loading) {
    return (
      <Box display="flex" justifyContent="center" alignItems="center" minHeight="400px">
        <CircularProgress />
      </Box>
    )
  }

  if (error) {
    return (
      <Box p={3}>
        <Typography color="error">Error: {error}</Typography>
        <Button onClick={fetchStatus} startIcon={<RefreshIcon />} sx={{ mt: 2 }}>
          Retry
        </Button>
      </Box>
    )
  }

  if (!status) return null

  return (
    <Box p={3}>
      <Box display="flex" justifyContent="space-between" alignItems="center" mb={3}>
        <Box display="flex" alignItems="center" gap={2}>
          <SecurityIcon fontSize="large" />
          <Typography variant="h4">AI Agent Security Dashboard</Typography>
        </Box>
        <Box display="flex" gap={1}>
          <Button
            variant={autoRefresh ? 'contained' : 'outlined'}
            size="small"
            onClick={() => setAutoRefresh(!autoRefresh)}
          >
            {autoRefresh ? 'Auto-refresh ON' : 'Auto-refresh OFF'}
          </Button>
          <IconButton onClick={fetchStatus}>
            <RefreshIcon />
          </IconButton>
        </Box>
      </Box>

      {/* Summary Cards */}
      <Grid container spacing={3} mb={3}>
        <Grid item xs={12} md={3}>
          <Card>
            <CardContent>
              <Box display="flex" alignItems="center" gap={1}>
                {status.telemetry_blocked ? (
                  <CheckCircleIcon color="success" />
                ) : (
                  <ErrorIcon color="error" />
                )}
                <Typography variant="subtitle2">Firewall Status</Typography>
              </Box>
              <Typography variant="h6">
                {status.telemetry_blocked ? 'ACTIVE' : 'INACTIVE'}
              </Typography>
            </CardContent>
          </Card>
        </Grid>

        <Grid item xs={12} md={3}>
          <Card>
            <CardContent>
              <Box display="flex" alignItems="center" gap={1}>
                <BlockIcon color="primary" />
                <Typography variant="subtitle2">Packets Blocked</Typography>
              </Box>
              <Typography variant="h6">{status.total_blocked_packets.toLocaleString()}</Typography>
            </CardContent>
          </Card>
        </Grid>

        <Grid item xs={12} md={3}>
          <Card>
            <CardContent>
              <Box display="flex" alignItems="center" gap={1}>
                <BlockIcon color="primary" />
                <Typography variant="subtitle2">Data Blocked</Typography>
              </Box>
              <Typography variant="h6">{formatBytes(status.total_blocked_bytes)}</Typography>
            </CardContent>
          </Card>
        </Grid>

        <Grid item xs={12} md={3}>
          <Card>
            <CardContent>
              <Box display="flex" alignItems="center" gap={1}>
                <WarningIcon color={status.phone_home_attempts.length > 0 ? 'warning' : 'disabled'} />
                <Typography variant="subtitle2">Phone-Home Attempts</Typography>
              </Box>
              <Typography variant="h6">{status.phone_home_attempts.length}</Typography>
            </CardContent>
          </Card>
        </Grid>
      </Grid>

      {/* Security Recommendations */}
      {status.security_recommendations.length > 0 && (
        <Card sx={{ mb: 3 }}>
          <CardContent>
            <Typography variant="h6" gutterBottom>
              Security Recommendations
            </Typography>
            {status.security_recommendations.map((rec, idx) => (
              <Box key={idx} display="flex" alignItems="center" gap={1} mb={1}>
                {rec.startsWith('✅') ? (
                  <CheckCircleIcon color="success" fontSize="small" />
                ) : (
                  <WarningIcon color="warning" fontSize="small" />
                )}
                <Typography variant="body2">{rec}</Typography>
              </Box>
            ))}
          </CardContent>
        </Card>
      )}

      {/* Agent Configuration Status */}
      <Card sx={{ mb: 3 }}>
        <CardContent>
          <Typography variant="h6" gutterBottom>
            AI Agent Privacy Configuration
          </Typography>
          <TableContainer>
            <Table size="small">
              <TableHead>
                <TableRow>
                  <TableCell>Agent</TableCell>
                  <TableCell>Config Path</TableCell>
                  <TableCell align="center">Status</TableCell>
                  <TableCell>Issues</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {Object.entries(status.agent_configurations).map(([key, config]) => (
                  <TableRow key={key}>
                    <TableCell>
                      <Box display="flex" alignItems="center" gap={1}>
                        {getAgentStatusIcon(config)}
                        <Typography variant="body2" fontWeight="bold">
                          {config.name}
                        </Typography>
                      </Box>
                    </TableCell>
                    <TableCell>
                      <Typography variant="caption" sx={{ fontFamily: 'monospace' }}>
                        {config.config_path}
                      </Typography>
                    </TableCell>
                    <TableCell align="center">
                      {config.config_verified ? (
                        <Chip label="SECURED" color="success" size="small" />
                      ) : (
                        <Chip label="AT RISK" color="error" size="small" />
                      )}
                    </TableCell>
                    <TableCell>
                      {config.issues.length > 0 ? (
                        <Box>
                          {config.issues.map((issue, idx) => (
                            <Typography key={idx} variant="caption" color="error" display="block">
                              • {issue}
                            </Typography>
                          ))}
                        </Box>
                      ) : (
                        <Typography variant="caption" color="success">
                          No issues
                        </Typography>
                      )}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </TableContainer>
        </CardContent>
      </Card>

      {/* Firewall Blocking Rules */}
      <Card sx={{ mb: 3 }}>
        <CardContent>
          <Typography variant="h6" gutterBottom>
            Firewall Blocking Rules ({status.rules.length} active)
          </Typography>
          {status.rules.length === 0 ? (
            <Typography variant="body2" color="warning.main">
              ⚠️ No firewall rules active - telemetry may not be blocked at OS level
            </Typography>
          ) : (
            <TableContainer component={Paper} variant="outlined">
              <Table size="small">
                <TableHead>
                  <TableRow>
                    <TableCell>Agent</TableCell>
                    <TableCell>Endpoint</TableCell>
                    <TableCell align="right">Packets Blocked</TableCell>
                    <TableCell align="right">Data Blocked</TableCell>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {status.rules.map((rule, idx) => (
                    <TableRow key={idx} sx={{ bgcolor: rule.packets_blocked > 0 ? 'warning.light' : 'inherit' }}>
                      <TableCell>
                        <Chip label={rule.agent} size="small" variant="outlined" />
                      </TableCell>
                      <TableCell>
                        <Typography variant="caption" sx={{ fontFamily: 'monospace' }}>
                          {rule.endpoint_type}
                        </Typography>
                      </TableCell>
                      <TableCell align="right">
                        <Typography
                          variant="body2"
                          fontWeight={rule.packets_blocked > 0 ? 'bold' : 'normal'}
                          color={rule.packets_blocked > 0 ? 'warning.main' : 'text.secondary'}
                        >
                          {rule.packets_blocked.toLocaleString()}
                        </Typography>
                      </TableCell>
                      <TableCell align="right">
                        <Typography variant="body2" color="text.secondary">
                          {formatBytes(rule.bytes_blocked)}
                        </Typography>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </TableContainer>
          )}
        </CardContent>
      </Card>

      {/* Phone-Home Attempts Detected */}
      {status.phone_home_attempts.length > 0 && (
        <Card sx={{ mb: 3 }}>
          <CardContent>
            <Typography variant="h6" gutterBottom color="warning.main">
              ⚠️ Detected Phone-Home Attempts ({status.phone_home_attempts.length})
            </Typography>
            <Typography variant="body2" color="text.secondary" mb={2}>
              These attempts were blocked by the OS-level firewall. Investigate agent configuration.
            </Typography>
            <TableContainer component={Paper} variant="outlined">
              <Table size="small">
                <TableHead>
                  <TableRow>
                    <TableCell>Severity</TableCell>
                    <TableCell>Agent</TableCell>
                    <TableCell>Target Endpoint</TableCell>
                    <TableCell align="right">Attempts</TableCell>
                    <TableCell align="right">Data Blocked</TableCell>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {status.phone_home_attempts.map((attempt, idx) => (
                    <TableRow key={idx}>
                      <TableCell>
                        <Box display="flex" alignItems="center" gap={1}>
                          {getSeverityIcon(attempt.severity)}
                          <Chip
                            label={attempt.severity.toUpperCase()}
                            color={getSeverityColor(attempt.severity)}
                            size="small"
                          />
                        </Box>
                      </TableCell>
                      <TableCell>
                        <Typography variant="body2" fontWeight="bold">
                          {attempt.agent}
                        </Typography>
                      </TableCell>
                      <TableCell>
                        <Typography variant="caption" sx={{ fontFamily: 'monospace' }}>
                          {attempt.endpoint}
                        </Typography>
                      </TableCell>
                      <TableCell align="right">
                        <Typography variant="body2" color="error">
                          {attempt.packet_count.toLocaleString()}
                        </Typography>
                      </TableCell>
                      <TableCell align="right">
                        <Typography variant="body2">
                          {formatBytes(attempt.bytes_blocked)}
                        </Typography>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </TableContainer>
          </CardContent>
        </Card>
      )}

      {/* Security Status Summary */}
      <Card>
        <CardContent>
          <Typography variant="h6" gutterBottom>
            Security Audit Summary
          </Typography>
          <Box>
            <Typography variant="body2" color="text.secondary" gutterBottom>
              <strong>Last Updated:</strong> {new Date(status.timestamp).toLocaleString()}
            </Typography>
            <Typography variant="body2" color="text.secondary" gutterBottom>
              <strong>Firewall Status:</strong>{' '}
              {status.telemetry_blocked ? (
                <span style={{ color: 'green' }}>✅ Active and blocking</span>
              ) : (
                <span style={{ color: 'red' }}>❌ Inactive - OS-level protection disabled</span>
              )}
            </Typography>
            <Typography variant="body2" color="text.secondary" gutterBottom>
              <strong>Total Packets Intercepted:</strong> {status.total_blocked_packets.toLocaleString()}{' '}
              {status.total_blocked_packets === 0 ? (
                <span style={{ color: 'green' }}>(No phone-home attempts detected)</span>
              ) : (
                <span style={{ color: 'orange' }}>(Agents attempted to phone home!)</span>
              )}
            </Typography>
            <Typography variant="body2" color="text.secondary">
              <strong>Defense Layers:</strong>
              <ul>
                <li>✅ Application-level configuration (settings.json files)</li>
                <li>✅ Environment variables (DISABLE_TELEMETRY, etc.)</li>
                <li>✅ OS-level iptables firewall (blocks even if configs bypassed)</li>
                <li>✅ iptables counters (detects and logs all attempts)</li>
                <li>✅ Real-time dashboard monitoring (this page)</li>
              </ul>
            </Typography>
          </Box>
        </CardContent>
      </Card>

      {/* Info Box */}
      <Box mt={3} p={2} bgcolor="info.light" borderRadius={1}>
        <Typography variant="body2">
          <strong>ℹ️ About This Dashboard:</strong> This dashboard monitors AI agent privacy
          and security. All AI coding agents (Qwen Code, Claude Code, Gemini CLI, Zed) are
          configured to disable telemetry at the application level. Additionally, OS-level
          iptables firewall rules block all known telemetry endpoints as a defense-in-depth
          measure. Any phone-home attempts are logged and displayed above.
        </Typography>
      </Box>
    </Box>
  )
}

export default TelemetrySecurityDashboard
