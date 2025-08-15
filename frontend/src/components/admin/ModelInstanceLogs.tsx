import React, { FC, useState, useEffect, useCallback } from 'react'
import {
  Box,
  Typography,
  Paper,
  Button,
  TextField,
  Select,
  MenuItem,
  FormControl,
  InputLabel,
  Chip,
  Alert,
  CircularProgress,
  List,
  ListItem,
  ListItemText,
  Accordion,
  AccordionSummary,
  AccordionDetails,
  IconButton,
  Tooltip
} from '@mui/material'
import {
  Refresh as RefreshIcon,
  ExpandMore as ExpandMoreIcon,
  Error as ErrorIcon,
  Warning as WarningIcon,
  Info as InfoIcon,
  BugReport as DebugIcon,
  Download as DownloadIcon
} from '@mui/icons-material'
import { TypesDashboardRunner } from '../../api/api'

interface LogEntry {
  timestamp: string
  level: string
  message: string
  source: string
}

interface LogMetadata {
  slot_id: string
  model_id: string
  created_at: string
  status: string
  last_error?: string
}

interface LogResponse {
  slot_id: string
  metadata: LogMetadata
  logs: LogEntry[]
  count: number
}

interface LogsSummary {
  active_instances: number
  recent_errors: number
  instances_with_errors: number
  max_lines_per_buffer: number
  error_retention_hours: number
}

const ModelInstanceLogs: FC<{ runner: TypesDashboardRunner }> = ({ runner }) => {
  const [selectedSlot, setSelectedSlot] = useState<string>('')
  const [logs, setLogs] = useState<LogEntry[]>([])
  const [metadata, setMetadata] = useState<LogMetadata | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [logLevel, setLogLevel] = useState<string>('all')
  const [maxLines, setMaxLines] = useState<number>(100)
  const [summary, setSummary] = useState<LogsSummary | null>(null)
  const [autoRefresh, setAutoRefresh] = useState(false)

  // Get runner URL - assuming it's available via the runner data
  const getRunnerURL = () => {
    // This would need to be implemented based on how runner URLs are exposed
    // For now, assume it's on a standard port
    return `http://localhost:8080` // This should be dynamic based on runner info
  }

  const fetchLogsSummary = async () => {
    try {
      const response = await fetch(`${getRunnerURL()}/api/v1/logs`)
      if (!response.ok) {
        throw new Error(`HTTP ${response.status}: ${response.statusText}`)
      }
      const data: LogsSummary = await response.json()
      setSummary(data)
    } catch (err) {
      console.error('Failed to fetch logs summary:', err)
    }
  }

  const fetchLogs = useCallback(async (slotId: string) => {
    if (!slotId) return
    
    setLoading(true)
    setError(null)
    
    try {
      const params = new URLSearchParams()
      if (maxLines > 0) params.set('lines', maxLines.toString())
      if (logLevel !== 'all') params.set('level', logLevel.toUpperCase())
      
      const url = `${getRunnerURL()}/api/v1/logs/${slotId}?${params.toString()}`
      const response = await fetch(url)
      
      if (!response.ok) {
        throw new Error(`HTTP ${response.status}: ${response.statusText}`)
      }
      
      const data: LogResponse = await response.json()
      setLogs(data.logs || [])
      setMetadata(data.metadata)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to fetch logs')
      setLogs([])
      setMetadata(null)
    } finally {
      setLoading(false)
    }
  }, [maxLines, logLevel])

  const handleRefresh = () => {
    if (selectedSlot) {
      fetchLogs(selectedSlot)
    }
    fetchLogsSummary()
  }

  const exportLogs = () => {
    if (logs.length === 0) return
    
    const content = logs.map(log => 
      `${log.timestamp} [${log.level}] ${log.message}`
    ).join('\n')
    
    const blob = new Blob([content], { type: 'text/plain' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `${selectedSlot}-logs.txt`
    document.body.appendChild(a)
    a.click()
    document.body.removeChild(a)
    URL.revokeObjectURL(url)
  }

  const getLogLevelIcon = (level: string) => {
    switch (level.toUpperCase()) {
      case 'ERROR':
        return <ErrorIcon sx={{ color: '#f44336', fontSize: 16 }} />
      case 'WARN':
        return <WarningIcon sx={{ color: '#ff9800', fontSize: 16 }} />
      case 'INFO':
        return <InfoIcon sx={{ color: '#2196f3', fontSize: 16 }} />
      case 'DEBUG':
        return <DebugIcon sx={{ color: '#9e9e9e', fontSize: 16 }} />
      default:
        return <InfoIcon sx={{ color: '#2196f3', fontSize: 16 }} />
    }
  }

  const getLogLevelColor = (level: string) => {
    switch (level.toUpperCase()) {
      case 'ERROR':
        return '#f44336'
      case 'WARN':
        return '#ff9800'
      case 'INFO':
        return '#2196f3'
      case 'DEBUG':
        return '#9e9e9e'
      default:
        return '#2196f3'
    }
  }

  useEffect(() => {
    fetchLogsSummary()
  }, [])

  // Auto-fetch logs when a slot is selected
  useEffect(() => {
    if (selectedSlot) {
      fetchLogs(selectedSlot)
    }
  }, [selectedSlot, fetchLogs]) // Re-fetch when slot or fetchLogs changes

  useEffect(() => {
    let interval: NodeJS.Timeout | null = null
    if (autoRefresh && selectedSlot) {
      interval = setInterval(() => {
        fetchLogs(selectedSlot)
      }, 5000) // Refresh every 5 seconds
    }
    return () => {
      if (interval) clearInterval(interval)
    }
  }, [autoRefresh, selectedSlot, maxLines, logLevel])

  const availableSlots = runner.slots || []

  return (
    <Box>
      <Accordion>
        <AccordionSummary
          expandIcon={<ExpandMoreIcon />}
          aria-controls="model-logs-content"
          id="model-logs-header"
        >
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, width: '100%' }}>
            <Typography variant="h6">Model Instance Logs</Typography>
            {summary && (
              <Box sx={{ display: 'flex', gap: 1, ml: 'auto', mr: 2 }}>
                <Chip
                  size="small"
                  label={`${summary.active_instances} Active`}
                  color="primary"
                  variant="outlined"
                />
                {summary.instances_with_errors > 0 && (
                  <Chip
                    size="small"
                    label={`${summary.instances_with_errors} Errors`}
                    color="error"
                    variant="outlined"
                  />
                )}
                {summary.recent_errors > 0 && (
                  <Chip
                    size="small"
                    label={`${summary.recent_errors} Recent Errors`}
                    color="warning"
                    variant="outlined"
                  />
                )}
              </Box>
            )}
          </Box>
        </AccordionSummary>
        <AccordionDetails>
          <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
            {/* Controls */}
            <Box sx={{ display: 'flex', gap: 2, alignItems: 'center', flexWrap: 'wrap' }}>
              <FormControl size="small" sx={{ minWidth: 200 }}>
                <InputLabel>Select Slot</InputLabel>
                <Select
                  value={selectedSlot}
                  onChange={(e) => setSelectedSlot(e.target.value)}
                  label="Select Slot"
                >
                  {availableSlots.map((slot) => (
                    <MenuItem key={slot.id} value={slot.id || ''}>
                      {slot.model} ({slot.id?.substring(0, 8)})
                    </MenuItem>
                  ))}
                </Select>
              </FormControl>

              <FormControl size="small" sx={{ minWidth: 120 }}>
                <InputLabel>Log Level</InputLabel>
                <Select
                  value={logLevel}
                  onChange={(e) => setLogLevel(e.target.value)}
                  label="Log Level"
                >
                  <MenuItem value="all">All</MenuItem>
                  <MenuItem value="error">Error</MenuItem>
                  <MenuItem value="warn">Warning</MenuItem>
                  <MenuItem value="info">Info</MenuItem>
                  <MenuItem value="debug">Debug</MenuItem>
                </Select>
              </FormControl>

              <TextField
                size="small"
                type="number"
                label="Max Lines"
                value={maxLines}
                onChange={(e) => setMaxLines(parseInt(e.target.value) || 100)}
                sx={{ width: 100 }}
              />

              <Button
                variant="outlined"
                onClick={handleRefresh}
                disabled={loading}
                startIcon={loading ? <CircularProgress size={16} /> : <RefreshIcon />}
              >
                Refresh
              </Button>

              <Button
                variant="outlined"
                onClick={() => setAutoRefresh(!autoRefresh)}
                color={autoRefresh ? 'success' : 'inherit'}
              >
                Auto Refresh
              </Button>

              {logs.length > 0 && (
                <Tooltip title="Export logs to file">
                  <IconButton onClick={exportLogs}>
                    <DownloadIcon />
                  </IconButton>
                </Tooltip>
              )}
            </Box>

            {/* Command Line Display */}
            {selectedSlot && (
              (() => {
                const slot = runner.slots?.find(s => s.id === selectedSlot)
                if (slot?.command_line && slot.runtime === 'vllm') {
                  return (
                    <Paper sx={{ p: 2, backgroundColor: 'rgba(0, 0, 0, 0.1)', mb: 2 }}>
                      <Typography variant="subtitle2" sx={{ mb: 1 }}>Calculated Command Line</Typography>
                      <Box sx={{ 
                        backgroundColor: 'rgba(0, 0, 0, 0.1)', 
                        p: 1.5, 
                        borderRadius: 1, 
                        fontFamily: 'monospace',
                        fontSize: '0.8rem',
                        wordBreak: 'break-all',
                        whiteSpace: 'pre-wrap',
                        border: '1px solid rgba(0, 0, 0, 0.2)'
                      }}>
                        {slot.command_line}
                      </Box>
                    </Paper>
                  )
                }
                return null
              })()
            )}

            {/* Metadata */}
            {metadata && (
              <Paper sx={{ p: 2, backgroundColor: 'rgba(0, 0, 0, 0.1)' }}>
                <Typography variant="subtitle2" sx={{ mb: 1 }}>Instance Metadata</Typography>
                <Box sx={{ display: 'flex', gap: 2, flexWrap: 'wrap' }}>
                  <Chip size="small" label={`Model: ${metadata.model_id}`} />
                  <Chip size="small" label={`Status: ${metadata.status}`} color={metadata.status === 'errored' ? 'error' : 'default'} />
                  <Chip size="small" label={`Created: ${new Date(metadata.created_at).toLocaleString()}`} />
                  {metadata.last_error && (
                    <Chip size="small" label={`Last Error: ${metadata.last_error}`} color="error" />
                  )}
                </Box>
              </Paper>
            )}

            {/* Error Display */}
            {error && (
              <Alert severity="error" onClose={() => setError(null)}>
                {error}
              </Alert>
            )}

            {/* Logs Display */}
            {selectedSlot && (
              <Paper sx={{ maxHeight: 400, overflow: 'auto', backgroundColor: 'rgba(0, 0, 0, 0.9)' }}>
                {logs.length === 0 && !loading ? (
                  <Box sx={{ p: 2, textAlign: 'center', color: 'rgba(255, 255, 255, 0.6)' }}>
                    No logs available for this slot
                  </Box>
                ) : (
                  <List dense>
                    {logs.map((log, index) => (
                      <ListItem key={index} sx={{ py: 0.5, borderBottom: '1px solid rgba(255, 255, 255, 0.1)' }}>
                        <ListItemText
                          primary={
                            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, fontFamily: 'monospace' }}>
                              <Typography variant="caption" sx={{ color: 'rgba(255, 255, 255, 0.5)', minWidth: 140 }}>
                                {new Date(log.timestamp).toLocaleTimeString()}
                              </Typography>
                              <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, minWidth: 60 }}>
                                {getLogLevelIcon(log.level)}
                                <Typography
                                  variant="caption"
                                  sx={{
                                    color: getLogLevelColor(log.level),
                                    fontWeight: 600,
                                    fontSize: '0.7rem'
                                  }}
                                >
                                  {log.level.toUpperCase()}
                                </Typography>
                              </Box>
                              <Typography
                                variant="body2"
                                sx={{
                                  color: '#ffffff',
                                  fontFamily: 'monospace',
                                  fontSize: '0.8rem',
                                  flex: 1,
                                  wordBreak: 'break-all'
                                }}
                              >
                                {log.message}
                              </Typography>
                            </Box>
                          }
                        />
                      </ListItem>
                    ))}
                  </List>
                )}
              </Paper>
            )}
          </Box>
        </AccordionDetails>
      </Accordion>
    </Box>
  )
}

export default ModelInstanceLogs
