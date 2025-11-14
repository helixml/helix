import React from 'react'
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
} from '@mui/material'
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

        {health.stuck_thread_count && health.stuck_thread_count > 0 && (
          <Alert severity="error" sx={{ mb: 2 }}>
            <strong>{health.stuck_thread_count}</strong> of <strong>{health.total_thread_count}</strong> threads are stuck (no heartbeat &gt;30s)
          </Alert>
        )}

        {health.threads && health.threads.length > 0 ? (
          <TableContainer>
            <Table size="small">
              <TableHead>
                <TableRow>
                  <TableCell>TID</TableCell>
                  <TableCell>Name</TableCell>
                  <TableCell>Details</TableCell>
                  <TableCell align="right">Heartbeat</TableCell>
                  <TableCell align="right">Alive</TableCell>
                  <TableCell align="right">Count</TableCell>
                  <TableCell>Status</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {health.threads.map((thread) => (
                  <TableRow
                    key={thread.tid}
                    sx={{
                      backgroundColor: thread.is_stuck ? 'error.light' : 'inherit',
                      opacity: thread.is_stuck ? 1 : 0.9,
                    }}
                  >
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
                      <Typography variant="body2" fontSize="0.75rem" color="text.secondary" sx={{
                        maxWidth: 300,
                        overflow: 'hidden',
                        textOverflow: 'ellipsis',
                        whiteSpace: 'nowrap',
                      }}>
                        {thread.details}
                      </Typography>
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
                ))}
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
