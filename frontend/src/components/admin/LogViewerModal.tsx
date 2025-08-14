import React, { FC, useState, useEffect, useRef, useCallback } from 'react'
import {
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Box,
  Typography,
  Button,
  TextField,
  Select,
  MenuItem,
  FormControl,
  InputLabel,
  IconButton,
  Chip,
  Alert,
  CircularProgress,
  List,
  ListItem,
  ListItemText,
  Switch,
  FormControlLabel,
  Tooltip,
  Paper
} from '@mui/material'
import {
  Close as CloseIcon,
  Refresh as RefreshIcon,
  Download as DownloadIcon,
  PlayArrow as PlayIcon,
  Stop as StopIcon,
  Error as ErrorIcon,
  Warning as WarningIcon,
  Info as InfoIcon,
  BugReport as DebugIcon,
  VerticalAlignBottom as ScrollDownIcon
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

interface LogViewerModalProps {
  open: boolean
  onClose: () => void
  runner: TypesDashboardRunner
  runnerUrl?: string
  isFloating?: boolean
}

const LogViewerModal: FC<LogViewerModalProps> = ({ open, onClose, runner, runnerUrl = 'http://localhost:8080', isFloating = false }) => {
  const [selectedSlot, setSelectedSlot] = useState<string>('')
  const [logs, setLogs] = useState<LogEntry[]>([])
  const [metadata, setMetadata] = useState<LogMetadata | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [logLevel, setLogLevel] = useState<string>('all')
  const [maxLines, setMaxLines] = useState<number>(500)
  const [tailMode, setTailMode] = useState(false)
  const [autoScroll, setAutoScroll] = useState(true)
  
  const logContainerRef = useRef<HTMLDivElement>(null)
  const tailIntervalRef = useRef<NodeJS.Timeout | null>(null)
  const lastTimestampRef = useRef<string | null>(null)

  const availableSlots = runner.slots || []

  const fetchLogs = useCallback(async (slotId: string, since?: string) => {
    if (!slotId) return
    
    setLoading(!since) // Only show loading for initial fetch, not for tail updates
    if (!since) setError(null)
    
    try {
      const params = new URLSearchParams()
      if (maxLines > 0) params.set('lines', maxLines.toString())
      if (logLevel !== 'all') params.set('level', logLevel.toUpperCase())
      if (since) params.set('since', since)
      
      const url = `${runnerUrl}/api/v1/logs/${slotId}?${params.toString()}`
      const response = await fetch(url)
      
      if (!response.ok) {
        throw new Error(`HTTP ${response.status}: ${response.statusText}`)
      }
      
      const data: LogResponse = await response.json()
      
      if (since) {
        // Tail mode: append new logs
        const newLogs = data.logs || []
        if (newLogs.length > 0) {
          setLogs(prevLogs => [...prevLogs, ...newLogs])
          lastTimestampRef.current = newLogs[newLogs.length - 1].timestamp
        }
      } else {
        // Initial fetch: replace all logs
        setLogs(data.logs || [])
        setMetadata(data.metadata)
        if (data.logs && data.logs.length > 0) {
          lastTimestampRef.current = data.logs[data.logs.length - 1].timestamp
        }
      }
    } catch (err) {
      const errorMsg = err instanceof Error ? err.message : 'Failed to fetch logs'
      setError(errorMsg)
      if (!since) {
        setLogs([])
        setMetadata(null)
      }
    } finally {
      setLoading(false)
    }
  }, [runnerUrl, maxLines, logLevel])

  // Auto-select first slot when modal opens
  useEffect(() => {
    if (open && availableSlots.length > 0 && !selectedSlot) {
      setSelectedSlot(availableSlots[0].id || '')
    }
  }, [open, availableSlots, selectedSlot])

  // Auto-fetch logs when a slot is selected
  useEffect(() => {
    if (selectedSlot && !tailMode) {
      fetchLogs(selectedSlot)
    }
  }, [selectedSlot, tailMode, fetchLogs]) // Re-fetch when slot or fetchLogs changes

  // Clear logs and reset state when modal closes
  useEffect(() => {
    if (!open) {
      setLogs([])
      setMetadata(null)
      setError(null)
      setSelectedSlot('')
      setTailMode(false)
      setLoading(false)
      if (tailIntervalRef.current) {
        clearInterval(tailIntervalRef.current)
        tailIntervalRef.current = null
      }
    }
  }, [open])

  const startTailMode = () => {
    setTailMode(true)
    // Fetch initial logs
    fetchLogs(selectedSlot)
    
    // Start polling for new logs
    tailIntervalRef.current = setInterval(() => {
      if (lastTimestampRef.current) {
        fetchLogs(selectedSlot, lastTimestampRef.current)
      }
    }, 2000) // Poll every 2 seconds
  }

  const stopTailMode = () => {
    setTailMode(false)
    if (tailIntervalRef.current) {
      clearInterval(tailIntervalRef.current)
      tailIntervalRef.current = null
    }
  }

  const handleRefresh = () => {
    if (selectedSlot) {
      fetchLogs(selectedSlot)
    }
  }

  const scrollToBottom = () => {
    if (logContainerRef.current) {
      logContainerRef.current.scrollTop = logContainerRef.current.scrollHeight
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
    a.download = `${selectedSlot}-logs-${new Date().toISOString().slice(0, 19)}.txt`
    document.body.appendChild(a)
    a.click()
    document.body.removeChild(a)
    URL.revokeObjectURL(url)
  }

  const getLogLevelIcon = (level: string) => {
    switch (level.toUpperCase()) {
      case 'ERROR':
        return <ErrorIcon sx={{ color: '#f44336', fontSize: 14 }} />
      case 'WARN':
        return <WarningIcon sx={{ color: '#ff9800', fontSize: 14 }} />
      case 'INFO':
        return <InfoIcon sx={{ color: '#2196f3', fontSize: 14 }} />
      case 'DEBUG':
        return <DebugIcon sx={{ color: '#9e9e9e', fontSize: 14 }} />
      default:
        return <InfoIcon sx={{ color: '#2196f3', fontSize: 14 }} />
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

  // Auto-scroll to bottom when new logs arrive
  useEffect(() => {
    if (autoScroll && tailMode) {
      setTimeout(scrollToBottom, 100)
    }
  }, [logs, autoScroll, tailMode])

  // Cleanup on unmount or modal close
  useEffect(() => {
    if (!open) {
      stopTailMode()
      setSelectedSlot('')
      setLogs([])
      setMetadata(null)
      setError(null)
      lastTimestampRef.current = null
    }
  }, [open])

  // If floating, render content only (title bar is handled by FloatingModal wrapper)
  if (isFloating) {
    return (
      <Box sx={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
        {/* Controls */}
        <Box sx={{ p: 2, borderBottom: '1px solid rgba(255, 255, 255, 0.1)' }}>
          <Box sx={{ display: 'flex', gap: 2, alignItems: 'center', flexWrap: 'wrap', mb: 2 }}>
            <FormControl size="small" sx={{ minWidth: 250 }}>
              <InputLabel sx={{ color: 'rgba(255, 255, 255, 0.7)' }}>Select Slot</InputLabel>
              <Select
                value={selectedSlot}
                onChange={(e) => setSelectedSlot(e.target.value)}
                label="Select Slot"
                sx={{ 
                  color: '#ffffff',
                  '& .MuiOutlinedInput-notchedOutline': {
                    borderColor: 'rgba(255, 255, 255, 0.3)'
                  }
                }}
              >
                {availableSlots.map((slot) => (
                  <MenuItem key={slot.id} value={slot.id || ''}>
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                      <Typography>{slot.model}</Typography>
                      <Chip 
                        size="small" 
                        label={slot.id?.substring(0, 8)} 
                        sx={{ height: 18, fontSize: '0.6rem' }}
                      />
                    </Box>
                  </MenuItem>
                ))}
              </Select>
            </FormControl>

            <FormControl size="small" sx={{ minWidth: 120 }}>
              <InputLabel sx={{ color: 'rgba(255, 255, 255, 0.7)' }}>Level</InputLabel>
              <Select
                value={logLevel}
                onChange={(e) => setLogLevel(e.target.value)}
                label="Level"
                sx={{ 
                  color: '#ffffff',
                  '& .MuiOutlinedInput-notchedOutline': {
                    borderColor: 'rgba(255, 255, 255, 0.3)'
                  }
                }}
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
              onChange={(e) => setMaxLines(parseInt(e.target.value) || 500)}
              sx={{ 
                width: 100,
                '& .MuiInputLabel-root': { color: 'rgba(255, 255, 255, 0.7)' },
                '& .MuiOutlinedInput-root': { 
                  color: '#ffffff',
                  '& .MuiOutlinedInput-notchedOutline': {
                    borderColor: 'rgba(255, 255, 255, 0.3)'
                  }
                }
              }}
            />
          </Box>

          <Box sx={{ display: 'flex', gap: 2, alignItems: 'center', flexWrap: 'wrap' }}>
            <Button
              variant="outlined"
              onClick={handleRefresh}
              disabled={loading || tailMode}
              startIcon={loading ? <CircularProgress size={16} /> : <RefreshIcon />}
            >
              Refresh
            </Button>

            <Button
              variant={tailMode ? "contained" : "outlined"}
              onClick={tailMode ? stopTailMode : startTailMode}
              disabled={!selectedSlot}
              startIcon={tailMode ? <StopIcon /> : <PlayIcon />}
              color={tailMode ? "error" : "primary"}
            >
              {tailMode ? 'Stop Tail' : 'Start Tail'}
            </Button>

            {logs.length > 0 && (
              <Button
                variant="outlined"
                onClick={exportLogs}
                startIcon={<DownloadIcon />}
              >
                Export
              </Button>
            )}

            <FormControlLabel
              control={
                <Switch
                  checked={autoScroll}
                  onChange={(e) => setAutoScroll(e.target.checked)}
                  size="small"
                />
              }
              label="Auto Scroll"
              sx={{ color: 'rgba(255, 255, 255, 0.7)' }}
            />

            <IconButton
              onClick={scrollToBottom}
              size="small"
              sx={{ color: 'rgba(255, 255, 255, 0.7)' }}
            >
              <ScrollDownIcon />
            </IconButton>

            {tailMode && (
              <Chip
                size="small"
                label="LIVE"
                color="success"
                sx={{ 
                  animation: 'pulse 1.5s infinite',
                  '@keyframes pulse': {
                    '0%': { opacity: 1 },
                    '50%': { opacity: 0.5 },
                    '100%': { opacity: 1 }
                  }
                }}
              />
            )}
          </Box>
        </Box>

        {/* Command Line Display */}
        {selectedSlot && (
          (() => {
            const slot = availableSlots.find(s => s.id === selectedSlot)
            if (slot?.command_line && slot.runtime === 'vllm') {
              return (
                <Paper sx={{ m: 2, p: 1.5, backgroundColor: 'rgba(0, 0, 0, 0.3)' }}>
                  <Typography variant="caption" sx={{ color: 'rgba(255, 255, 255, 0.6)', display: 'block', mb: 1 }}>
                    Calculated Command Line
                  </Typography>
                  <Box sx={{ 
                    backgroundColor: 'rgba(0, 0, 0, 0.5)', 
                    p: 1, 
                    borderRadius: 1, 
                    fontFamily: 'monospace',
                    fontSize: '0.8rem',
                    color: 'rgba(255, 255, 255, 0.9)',
                    wordBreak: 'break-all',
                    whiteSpace: 'pre-wrap'
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
          <Paper sx={{ m: 2, p: 1.5, backgroundColor: 'rgba(0, 0, 0, 0.3)' }}>
            <Typography variant="caption" sx={{ color: 'rgba(255, 255, 255, 0.6)', display: 'block', mb: 1 }}>
              Instance Metadata
            </Typography>
            <Box sx={{ display: 'flex', gap: 1, flexWrap: 'wrap' }}>
              <Chip size="small" label={`Model: ${metadata.model_id}`} />
              <Chip 
                size="small" 
                label={`Status: ${metadata.status}`} 
                color={metadata.status === 'errored' ? 'error' : metadata.status === 'running' ? 'success' : 'default'} 
              />
              <Chip size="small" label={`Created: ${new Date(metadata.created_at).toLocaleString()}`} />
              {metadata.last_error && (
                <Chip size="small" label={`Last Error: ${metadata.last_error.substring(0, 50)}...`} color="error" />
              )}
            </Box>
          </Paper>
        )}

        {/* Error Display */}
        {error && (
          <Alert severity="error" sx={{ m: 2 }} onClose={() => setError(null)}>
            {error}
          </Alert>
        )}

        {/* Logs Display */}
        <Box sx={{ flex: 1, overflow: 'hidden', mx: 2, mb: 2 }}>
          <Paper 
            ref={logContainerRef}
            sx={{ 
              height: '100%',
              overflow: 'auto', 
              backgroundColor: 'rgba(0, 0, 0, 0.8)',
              border: '1px solid rgba(255, 255, 255, 0.1)',
              '&::-webkit-scrollbar': {
                width: '8px',
              },
              '&::-webkit-scrollbar-track': {
                background: 'rgba(255, 255, 255, 0.1)',
              },
              '&::-webkit-scrollbar-thumb': {
                background: 'rgba(255, 255, 255, 0.3)',
                borderRadius: '4px',
              },
            }}
          >
            {logs.length === 0 && !loading ? (
              <Box sx={{ p: 3, textAlign: 'center', color: 'rgba(255, 255, 255, 0.6)' }}>
                {selectedSlot ? 'No logs available for this slot' : 'Select a slot to view logs'}
              </Box>
            ) : (
              <List dense sx={{ py: 0 }}>
                {logs.map((log, index) => (
                  <ListItem 
                    key={`${log.timestamp}-${index}`} 
                    sx={{ 
                      py: 0.25, 
                      px: 1,
                      borderBottom: index < logs.length - 1 ? '1px solid rgba(255, 255, 255, 0.05)' : 'none',
                      '&:hover': {
                        backgroundColor: 'rgba(255, 255, 255, 0.02)'
                      }
                    }}
                  >
                    <ListItemText
                      primary={
                        <Box sx={{ display: 'flex', alignItems: 'flex-start', gap: 1, fontFamily: 'monospace' }}>
                          <Typography 
                            variant="caption" 
                            sx={{ 
                              color: 'rgba(255, 255, 255, 0.4)', 
                              minWidth: 80,
                              fontSize: '0.7rem',
                              lineHeight: 1.2
                            }}
                          >
                            {new Date(log.timestamp).toLocaleTimeString()}
                          </Typography>
                          <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, minWidth: 50 }}>
                            {getLogLevelIcon(log.level)}
                            <Typography
                              variant="caption"
                              sx={{
                                color: getLogLevelColor(log.level),
                                fontWeight: 600,
                                fontSize: '0.65rem'
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
                              fontSize: '0.75rem',
                              flex: 1,
                              wordBreak: 'break-word',
                              lineHeight: 1.3
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
        </Box>
        
        {/* Footer */}
        <Box sx={{ borderTop: '1px solid rgba(255, 255, 255, 0.1)', p: 2, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <Typography variant="caption" sx={{ color: 'rgba(255, 255, 255, 0.5)' }}>
            {logs.length > 0 && `${logs.length} log entries`}
            {tailMode && ' • Live tail active'}
          </Typography>
          <Button onClick={onClose} variant="outlined">
            Close
          </Button>
        </Box>
      </Box>
    )
  }

  // Regular Dialog mode
  return (
    <Dialog
      open={open}
      onClose={onClose}
      maxWidth="lg"
      fullWidth
      PaperProps={{
        sx: {
          height: '80vh',
          backgroundColor: 'rgba(18, 18, 20, 0.95)',
          backdropFilter: 'blur(10px)',
        }
      }}
    >
      <DialogTitle sx={{ 
        display: 'flex', 
        justifyContent: 'space-between', 
        alignItems: 'center',
        borderBottom: '1px solid rgba(255, 255, 255, 0.1)',
        pb: 2
      }}>
        <Box>
          <Typography variant="h6" sx={{ color: '#ffffff' }}>
            Model Instance Logs
          </Typography>
          <Typography variant="caption" sx={{ color: 'rgba(255, 255, 255, 0.6)' }}>
            Runner: {runner.id?.substring(0, 8)} • {availableSlots.length} slots
          </Typography>
        </Box>
        <IconButton onClick={onClose} sx={{ color: 'rgba(255, 255, 255, 0.7)' }}>
          <CloseIcon />
        </IconButton>
      </DialogTitle>

      <DialogContent sx={{ p: 0, display: 'flex', flexDirection: 'column', height: '100%' }}>
        {/* Controls */}
        <Box sx={{ p: 2, borderBottom: '1px solid rgba(255, 255, 255, 0.1)' }}>
          <Box sx={{ display: 'flex', gap: 2, alignItems: 'center', flexWrap: 'wrap', mb: 2 }}>
            <FormControl size="small" sx={{ minWidth: 250 }}>
              <InputLabel sx={{ color: 'rgba(255, 255, 255, 0.7)' }}>Select Slot</InputLabel>
              <Select
                value={selectedSlot}
                onChange={(e) => setSelectedSlot(e.target.value)}
                label="Select Slot"
                sx={{ 
                  color: '#ffffff',
                  '& .MuiOutlinedInput-notchedOutline': {
                    borderColor: 'rgba(255, 255, 255, 0.3)'
                  }
                }}
              >
                {availableSlots.map((slot) => (
                  <MenuItem key={slot.id} value={slot.id || ''}>
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                      <Typography>{slot.model}</Typography>
                      <Chip 
                        size="small" 
                        label={slot.id?.substring(0, 8)} 
                        sx={{ height: 18, fontSize: '0.6rem' }}
                      />
                    </Box>
                  </MenuItem>
                ))}
              </Select>
            </FormControl>

            <FormControl size="small" sx={{ minWidth: 120 }}>
              <InputLabel sx={{ color: 'rgba(255, 255, 255, 0.7)' }}>Level</InputLabel>
              <Select
                value={logLevel}
                onChange={(e) => setLogLevel(e.target.value)}
                label="Level"
                sx={{ 
                  color: '#ffffff',
                  '& .MuiOutlinedInput-notchedOutline': {
                    borderColor: 'rgba(255, 255, 255, 0.3)'
                  }
                }}
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
              onChange={(e) => setMaxLines(parseInt(e.target.value) || 500)}
              sx={{ 
                width: 100,
                '& .MuiInputLabel-root': { color: 'rgba(255, 255, 255, 0.7)' },
                '& .MuiOutlinedInput-root': { 
                  color: '#ffffff',
                  '& .MuiOutlinedInput-notchedOutline': {
                    borderColor: 'rgba(255, 255, 255, 0.3)'
                  }
                }
              }}
            />
          </Box>

          <Box sx={{ display: 'flex', gap: 2, alignItems: 'center', flexWrap: 'wrap' }}>
            <Button
              variant="outlined"
              onClick={handleRefresh}
              disabled={loading || tailMode}
              startIcon={loading ? <CircularProgress size={16} /> : <RefreshIcon />}
            >
              Refresh
            </Button>

            <Button
              variant={tailMode ? "contained" : "outlined"}
              onClick={tailMode ? stopTailMode : startTailMode}
              disabled={!selectedSlot}
              startIcon={tailMode ? <StopIcon /> : <PlayIcon />}
              color={tailMode ? "error" : "primary"}
            >
              {tailMode ? 'Stop Tail' : 'Start Tail'}
            </Button>

            {logs.length > 0 && (
              <Button
                variant="outlined"
                onClick={exportLogs}
                startIcon={<DownloadIcon />}
              >
                Export
              </Button>
            )}

            <FormControlLabel
              control={
                <Switch
                  checked={autoScroll}
                  onChange={(e) => setAutoScroll(e.target.checked)}
                  size="small"
                />
              }
              label="Auto Scroll"
              sx={{ color: 'rgba(255, 255, 255, 0.7)' }}
            />

            <IconButton
              onClick={scrollToBottom}
              size="small"
              sx={{ color: 'rgba(255, 255, 255, 0.7)' }}
            >
              <ScrollDownIcon />
            </IconButton>

            {tailMode && (
              <Chip
                size="small"
                label="LIVE"
                color="success"
                sx={{ 
                  animation: 'pulse 1.5s infinite',
                  '@keyframes pulse': {
                    '0%': { opacity: 1 },
                    '50%': { opacity: 0.5 },
                    '100%': { opacity: 1 }
                  }
                }}
              />
            )}
          </Box>
        </Box>

        {/* Command Line Display */}
        {selectedSlot && (
          (() => {
            const slot = availableSlots.find(s => s.id === selectedSlot)
            if (slot?.command_line && slot.runtime === 'vllm') {
              return (
                <Paper sx={{ m: 2, p: 1.5, backgroundColor: 'rgba(0, 0, 0, 0.3)' }}>
                  <Typography variant="caption" sx={{ color: 'rgba(255, 255, 255, 0.6)', display: 'block', mb: 1 }}>
                    Calculated Command Line
                  </Typography>
                  <Box sx={{ 
                    backgroundColor: 'rgba(0, 0, 0, 0.5)', 
                    p: 1, 
                    borderRadius: 1, 
                    fontFamily: 'monospace',
                    fontSize: '0.8rem',
                    color: 'rgba(255, 255, 255, 0.9)',
                    wordBreak: 'break-all',
                    whiteSpace: 'pre-wrap'
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
          <Paper sx={{ m: 2, p: 1.5, backgroundColor: 'rgba(0, 0, 0, 0.3)' }}>
            <Typography variant="caption" sx={{ color: 'rgba(255, 255, 255, 0.6)', display: 'block', mb: 1 }}>
              Instance Metadata
            </Typography>
            <Box sx={{ display: 'flex', gap: 1, flexWrap: 'wrap' }}>
              <Chip size="small" label={`Model: ${metadata.model_id}`} />
              <Chip 
                size="small" 
                label={`Status: ${metadata.status}`} 
                color={metadata.status === 'errored' ? 'error' : metadata.status === 'running' ? 'success' : 'default'} 
              />
              <Chip size="small" label={`Created: ${new Date(metadata.created_at).toLocaleString()}`} />
              {metadata.last_error && (
                <Chip size="small" label={`Last Error: ${metadata.last_error.substring(0, 50)}...`} color="error" />
              )}
            </Box>
          </Paper>
        )}

        {/* Error Display */}
        {error && (
          <Alert severity="error" sx={{ m: 2 }} onClose={() => setError(null)}>
            {error}
          </Alert>
        )}

        {/* Logs Display */}
        <Box sx={{ flex: 1, overflow: 'hidden', mx: 2, mb: 2 }}>
          <Paper 
            ref={logContainerRef}
            sx={{ 
              height: '100%',
              overflow: 'auto', 
              backgroundColor: 'rgba(0, 0, 0, 0.8)',
              border: '1px solid rgba(255, 255, 255, 0.1)',
              '&::-webkit-scrollbar': {
                width: '8px',
              },
              '&::-webkit-scrollbar-track': {
                background: 'rgba(255, 255, 255, 0.1)',
              },
              '&::-webkit-scrollbar-thumb': {
                background: 'rgba(255, 255, 255, 0.3)',
                borderRadius: '4px',
              },
            }}
          >
            {logs.length === 0 && !loading ? (
              <Box sx={{ p: 3, textAlign: 'center', color: 'rgba(255, 255, 255, 0.6)' }}>
                {selectedSlot ? 'No logs available for this slot' : 'Select a slot to view logs'}
              </Box>
            ) : (
              <List dense sx={{ py: 0 }}>
                {logs.map((log, index) => (
                  <ListItem 
                    key={`${log.timestamp}-${index}`} 
                    sx={{ 
                      py: 0.25, 
                      px: 1,
                      borderBottom: index < logs.length - 1 ? '1px solid rgba(255, 255, 255, 0.05)' : 'none',
                      '&:hover': {
                        backgroundColor: 'rgba(255, 255, 255, 0.02)'
                      }
                    }}
                  >
                    <ListItemText
                      primary={
                        <Box sx={{ display: 'flex', alignItems: 'flex-start', gap: 1, fontFamily: 'monospace' }}>
                          <Typography 
                            variant="caption" 
                            sx={{ 
                              color: 'rgba(255, 255, 255, 0.4)', 
                              minWidth: 80,
                              fontSize: '0.7rem',
                              lineHeight: 1.2
                            }}
                          >
                            {new Date(log.timestamp).toLocaleTimeString()}
                          </Typography>
                          <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, minWidth: 50 }}>
                            {getLogLevelIcon(log.level)}
                            <Typography
                              variant="caption"
                              sx={{
                                color: getLogLevelColor(log.level),
                                fontWeight: 600,
                                fontSize: '0.65rem'
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
                              fontSize: '0.75rem',
                              flex: 1,
                              wordBreak: 'break-word',
                              lineHeight: 1.3
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
        </Box>
      </DialogContent>

      <DialogActions sx={{ borderTop: '1px solid rgba(255, 255, 255, 0.1)', p: 2 }}>
        <Typography variant="caption" sx={{ color: 'rgba(255, 255, 255, 0.5)', mr: 'auto' }}>
          {logs.length > 0 && `${logs.length} log entries`}
          {tailMode && ' • Live tail active'}
        </Typography>
        <Button onClick={onClose} variant="outlined">
          Close
        </Button>
      </DialogActions>
    </Dialog>
  )
}

export default LogViewerModal
