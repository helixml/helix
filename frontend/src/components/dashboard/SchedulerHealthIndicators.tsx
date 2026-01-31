import React, { useState, useEffect } from 'react'
import { Box, Tooltip } from '@mui/material'
import CheckCircleIcon from '@mui/icons-material/CheckCircle'
import WarningIcon from '@mui/icons-material/Warning'
import useApi from '../../hooks/useApi'

interface GoroutineHeartbeat {
  name: string
  last_beat: string
  restart_count: number
  is_healthy: boolean
  current_status: string
}

interface SchedulerHealthIndicatorsProps {
  runnerId: string
}

const SchedulerHealthIndicators: React.FC<SchedulerHealthIndicatorsProps> = ({ runnerId }) => {
  const api = useApi()
  const apiClient = api.getApiClient()
  const [heartbeats, setHeartbeats] = useState<Record<string, GoroutineHeartbeat>>({})
  const [isLoading, setIsLoading] = useState(true)

  useEffect(() => {
    const fetchHeartbeats = async () => {
      try {
        const response = await apiClient.v1SchedulerHeartbeatsList()
        setHeartbeats(response.data || {})
        setIsLoading(false)
      } catch (error) {
        console.error('Failed to fetch scheduler heartbeats:', error)
        setIsLoading(false)
      }
    }

    // Initial fetch
    fetchHeartbeats()

    // Poll every 5 seconds
    const interval = setInterval(fetchHeartbeats, 5000)

    return () => clearInterval(interval)
  }, [apiClient])

  if (isLoading) {
    return null
  }

  const goroutineNames = ['processQueue', 'reconcileSlots', 'reconcileActivity', 'reconcileRunners']

  const formatLastBeat = (lastBeat: string) => {
    const date = new Date(lastBeat)
    const now = new Date()
    const secondsAgo = Math.floor((now.getTime() - date.getTime()) / 1000)
    
    if (secondsAgo < 60) {
      return `${secondsAgo}s ago`
    } else if (secondsAgo < 3600) {
      return `${Math.floor(secondsAgo / 60)}m ago`
    } else {
      return date.toLocaleTimeString()
    }
  }

  const getDisplayName = (name: string) => {
    switch (name) {
      case 'processQueue':
        return 'Queue Processor'
      case 'reconcileSlots':
        return 'Slot Manager'
      case 'reconcileActivity':
        return 'Activity Cleaner'
      case 'reconcileRunners':
        return 'Runner Monitor'
      default:
        return name
    }
  }

  return (
    <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, ml: 1 }}>
      {goroutineNames.map((name) => {
        const heartbeat = heartbeats[name]
        const isHealthy = heartbeat?.is_healthy ?? false
        const lastBeat = heartbeat?.last_beat
        const restartCount = heartbeat?.restart_count ?? 0
        const currentStatus = heartbeat?.current_status ?? 'unknown'

        const tooltipTitle = heartbeat ? (
          <Box sx={{ p: 0.5 }}>
            <Box sx={{ fontWeight: 600, mb: 0.5 }}>
              {getDisplayName(name)}
            </Box>
            <Box sx={{ fontSize: '0.75rem', opacity: 0.9 }}>
              Status: {isHealthy ? 'Healthy' : 'Unhealthy'}
            </Box>
            <Box sx={{ fontSize: '0.75rem', opacity: 0.9, mb: 0.5 }}>
              Current: {currentStatus}
            </Box>
            <Box sx={{ fontSize: '0.75rem', opacity: 0.9 }}>
              Last Beat: {formatLastBeat(lastBeat)}
            </Box>
            {restartCount > 0 && (
              <Box sx={{ fontSize: '0.75rem', opacity: 0.9 }}>
                Restarts: {restartCount}
              </Box>
            )}
          </Box>
        ) : (
          <Box sx={{ p: 0.5 }}>
            <Box sx={{ fontWeight: 600, mb: 0.5 }}>
              {getDisplayName(name)}
            </Box>
            <Box sx={{ fontSize: '0.75rem', opacity: 0.9 }}>
              No data available
            </Box>
          </Box>
        )

        return (
          <Tooltip key={name} title={tooltipTitle} placement="top">
            <Box
              sx={{
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                width: 16,
                height: 16,
                cursor: 'help',
              }}
            >
              {isHealthy ? (
                <CheckCircleIcon
                  sx={{
                    fontSize: 16,
                    color: '#4ade80', // Green-400
                    filter: 'drop-shadow(0 0 2px rgba(74, 222, 128, 0.5))',
                  }}
                />
              ) : (
                <WarningIcon
                  sx={{
                    fontSize: 16,
                    color: '#f59e0b', // Amber-500
                    filter: 'drop-shadow(0 0 2px rgba(245, 158, 11, 0.5))',
                  }}
                />
              )}
            </Box>
          </Tooltip>
        )
      })}
    </Box>
  )
}

export default SchedulerHealthIndicators
