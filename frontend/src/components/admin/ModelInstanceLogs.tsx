import React, { FC, useState, useEffect } from 'react'
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
import { useSlotLogs, LogEntry, LogMetadata } from '../../services/logsService'

const ModelInstanceLogs: FC<{ runner: TypesDashboardRunner }> = ({ runner }) => {
  const [selectedSlot, setSelectedSlot] = useState<string>('')
  const [logLevel, setLogLevel] = useState<string>('all')
  const [maxLines, setMaxLines] = useState<number>(100)
  const [autoRefresh, setAutoRefresh] = useState(false)

  // Build stable query parameters (no 'since')
  const query = {
    lines: maxLines > 0 ? maxLines : undefined,
    level: logLevel !== 'all' ? logLevel.toUpperCase() : undefined,
  }

  // Use React Query for fetching logs
  const { data, isLoading, error, refetch } = useSlotLogs(
    selectedSlot,
    query,
    {
      enabled: !!selectedSlot,
      refetchInterval: autoRefresh ? 5000 : false, // Refetch every 5s when auto-refresh is on
      // No sinceRef needed - this component replaces logs on each fetch (non-tail mode)
    }
  )

  const logs = data?.logs || []
  const metadata = data?.metadata || null

  const handleRefresh = () => {
    if (selectedSlot) {
      refetch()
    }
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
                disabled={isLoading}
                startIcon={isLoading ? <CircularProgress size={16} /> : <RefreshIcon />}
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
              <Alert severity="error">
                {error instanceof Error ? error.message : String(error)}
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
