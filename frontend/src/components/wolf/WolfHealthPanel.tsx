import React, { useState } from 'react'
import {
  Box,
  Card,
  CardContent,
  Typography,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Chip,
  Alert,
  CircularProgress,
  Collapse,
  IconButton,
} from '@mui/material'
import KeyboardArrowDownIcon from '@mui/icons-material/KeyboardArrowDown'
import KeyboardArrowUpIcon from '@mui/icons-material/KeyboardArrowUp'
import { useWolfHealth } from '../../services'


const formatUptime = (seconds: number): string => {
  const hours = Math.floor(seconds / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)
  const secs = seconds % 60

  if (hours > 0) {
    return `${hours}h ${minutes}m ${secs}s`
  } else if (minutes > 0) {
    return `${minutes}m ${secs}s`
  } else {
    return `${secs}s`
  }
}

const WolfHealthPanel: React.FC = () => {
  const { data: health, isLoading, error } = useWolfHealth()
  const [expandedRows, setExpandedRows] = useState<Set<number>>(new Set())

  const toggleRow = (tid: number) => {
    setExpandedRows(prev => {
      const newSet = new Set(prev)
      if (newSet.has(tid)) {
        newSet.delete(tid)
      } else {
        newSet.add(tid)
      }
      return newSet
    })
  }

  if (isLoading) {
    return (
      <Box display="flex" justifyContent="center" p={4}>
        <CircularProgress />
      </Box>
    )
  }

  if (error) {
    return (
      <Alert severity="error">
        Failed to load Wolf health data: {error.message}
      </Alert>
    )
  }

  if (!health) {
    return null
  }

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'healthy':
        return 'success'
      case 'degraded':
        return 'warning'
      case 'critical':
        return 'error'
      default:
        return 'default'
    }
  }

  return (
    <Card>
      <CardContent>
        <Box display="flex" justifyContent="space-between" alignItems="center" mb={2}>
          <Typography variant="h6">
            Wolf System Health
          </Typography>
          <Box display="flex" gap={2} alignItems="center">
            <Chip
              label={health.overall_status?.toUpperCase() || 'UNKNOWN'}
              color={getStatusColor(health.overall_status || '')}
              size="small"
            />
            <Typography variant="body2" color="text.secondary">
              Uptime: {formatUptime(health.process_uptime_seconds || 0)}
            </Typography>
          </Box>
        </Box>

        {health.can_create_new_pipelines === false && (
          <Alert severity="error" sx={{ mb: 2 }}>
            <strong>CRITICAL:</strong> Pipeline creation test FAILED - new streaming sessions will NOT work.
            GStreamer type lock is likely held by a crashed thread. System needs restart.
          </Alert>
        )}

        {health.stuck_thread_count && health.stuck_thread_count > 0 && (
          <Alert
            severity={health.can_create_new_pipelines ? "warning" : "error"}
            sx={{ mb: 2 }}
          >
            <strong>{health.stuck_thread_count}</strong> of <strong>{health.total_thread_count}</strong> threads are stuck (no heartbeat &gt;30s)
            {health.can_create_new_pipelines && (
              <Typography variant="caption" display="block" sx={{ mt: 0.5 }}>
                Pipeline creation test still works - new sessions OK despite stuck threads
              </Typography>
            )}
          </Alert>
        )}

        {health.can_create_new_pipelines === true && health.stuck_thread_count === 0 && (
          <Alert severity="success" sx={{ mb: 2 }}>
            System healthy - pipeline creation works, no stuck threads
          </Alert>
        )}

        {health.threads && health.threads.length > 0 ? (
          <TableContainer>
            <Table size="small">
              <TableHead>
                <TableRow>
                  <TableCell />
                  <TableCell>TID</TableCell>
                  <TableCell>Name</TableCell>
                  <TableCell>Details / Current Request</TableCell>
                  <TableCell align="right">Heartbeat</TableCell>
                  <TableCell align="right">Alive</TableCell>
                  <TableCell align="right">Count</TableCell>
                  <TableCell>Status</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {health.threads.map((thread) => {
                  const isExpanded = expandedRows.has(thread.tid || 0)
                  const hasExpandableContent = thread.stack_trace || thread.has_active_request

                  return (
                    <React.Fragment key={thread.tid}>
                      <TableRow
                        sx={{
                          backgroundColor: thread.is_stuck ? 'error.light' : 'inherit',
                          opacity: thread.is_stuck ? 1 : 0.9,
                        }}
                      >
                        <TableCell>
                          {hasExpandableContent && (
                            <IconButton size="small" onClick={() => toggleRow(thread.tid || 0)}>
                              {isExpanded ? <KeyboardArrowUpIcon /> : <KeyboardArrowDownIcon />}
                            </IconButton>
                          )}
                        </TableCell>
                        <TableCell>
                          <Typography variant="body2" fontFamily="monospace">
                            {thread.tid}
                          </Typography>
                        </TableCell>
                    <TableCell>
                      <Typography variant="body2" fontWeight={thread.is_stuck ? 'bold' : 'normal'}>
                        {thread.name}
                      </Typography>
                    </TableCell>
                    <TableCell>
                      {thread.has_active_request ? (
                        <Box>
                          <Typography variant="body2" fontSize="0.75rem" fontWeight="bold" color={thread.request_duration_seconds && thread.request_duration_seconds > 30 ? 'error.main' : 'warning.main'}>
                            {thread.current_request_path}
                          </Typography>
                          <Typography variant="caption" color="text.secondary">
                            {thread.request_duration_seconds}s elapsed
                          </Typography>
                        </Box>
                      ) : (
                        <Typography variant="body2" fontSize="0.75rem" color="text.secondary" sx={{
                          maxWidth: 300,
                          overflow: 'hidden',
                          textOverflow: 'ellipsis',
                          whiteSpace: 'nowrap',
                        }}>
                          {thread.details}
                        </Typography>
                      )}
                    </TableCell>
                    <TableCell align="right">
                      <Typography variant="body2" fontFamily="monospace">
                        {thread.seconds_since_heartbeat}s
                      </Typography>
                    </TableCell>
                    <TableCell align="right">
                      <Typography variant="body2" fontFamily="monospace">
                        {thread.seconds_alive}s
                      </Typography>
                    </TableCell>
                    <TableCell align="right">
                      <Typography variant="body2" fontFamily="monospace">
                        {thread.heartbeat_count}
                      </Typography>
                    </TableCell>
                        <TableCell>
                          <Chip
                            label={thread.is_stuck ? 'STUCK' : 'OK'}
                            color={thread.is_stuck ? 'error' : 'success'}
                            size="small"
                            sx={{ minWidth: 60 }}
                          />
                        </TableCell>
                      </TableRow>
                      {hasExpandableContent && (
                        <TableRow>
                          <TableCell style={{ paddingBottom: 0, paddingTop: 0 }} colSpan={8}>
                            <Collapse in={isExpanded} timeout="auto" unmountOnExit>
                              <Box sx={{ margin: 2 }}>
                                {thread.has_active_request && thread.current_request_path && (
                                  <Box sx={{ mb: 2 }}>
                                    <Typography variant="subtitle2" gutterBottom>
                                      Active HTTP Request:
                                    </Typography>
                                    <Typography variant="body2" fontFamily="monospace" color="warning.main">
                                      {thread.current_request_path}
                                    </Typography>
                                    <Typography variant="caption" color="text.secondary">
                                      Duration: {thread.request_duration_seconds}s
                                    </Typography>
                                  </Box>
                                )}
                                {thread.stack_trace && (
                                  <Box>
                                    <Typography variant="subtitle2" gutterBottom>
                                      Current System Call:
                                    </Typography>
                                    <Box
                                      component="pre"
                                      sx={{
                                        fontSize: '0.8rem',
                                        fontFamily: 'monospace',
                                        backgroundColor: 'background.default',
                                        padding: 1,
                                        borderRadius: 1,
                                        overflow: 'auto',
                                      }}
                                    >
                                      {thread.stack_trace || 'Thread is running in userspace'}
                                    </Box>
                                  </Box>
                                )}
                              </Box>
                            </Collapse>
                          </TableCell>
                        </TableRow>
                      )}
                    </React.Fragment>
                  )
                })}
              </TableBody>
            </Table>
          </TableContainer>
        ) : (
          <Typography variant="body2" color="text.secondary">
            No threads being monitored
          </Typography>
        )}

        <Typography variant="caption" color="text.secondary" display="block" mt={2}>
          Data refreshes every 5 seconds
        </Typography>
      </CardContent>
    </Card>
  )
}

export default WolfHealthPanel
