import { FC } from 'react'
import {
  Box,
  Chip,
  CircularProgress,
  IconButton,
  Tooltip,
} from '@mui/material'
import {
  Check,
  AlertCircle,
  RefreshCw,
  Clock,
} from 'lucide-react'
import { KoditIndexingStatus } from '../../services/koditService'

interface KoditStatusPillProps {
  data?: KoditIndexingStatus
  isLoading?: boolean
  error?: Error | null
  onRefresh?: () => void
  isRefreshing?: boolean
}

const KoditStatusPill: FC<KoditStatusPillProps> = ({
  data,
  isLoading,
  error,
  onRefresh,
  isRefreshing,
}) => {
  const attrs = data?.data?.attributes
  const status = attrs?.status
  const message = attrs?.message
  const updatedAt = attrs?.updated_at

  // Determine if refresh should be disabled (during indexing or when already refreshing)
  const isIndexing = status === 'indexing' || status === 'in_progress' || status === 'queued' || status === 'pending'
  const refreshDisabled = isRefreshing || isIndexing || isLoading

  const refreshButton = onRefresh ? (
    <Tooltip title={isIndexing ? 'Indexing in progress' : 'Refresh code intelligence'} arrow placement="top">
      <span>
        <IconButton
          size="small"
          onClick={onRefresh}
          disabled={refreshDisabled}
          sx={{
            ml: 0.5,
            p: 0.25,
            '& svg': {
              animation: isRefreshing ? 'spin 1s linear infinite' : 'none',
              '@keyframes spin': {
                '0%': { transform: 'rotate(0deg)' },
                '100%': { transform: 'rotate(360deg)' },
              },
            },
          }}
        >
          <RefreshCw size={14} />
        </IconButton>
      </span>
    </Tooltip>
  ) : null

  // Handle API errors first
  if (error) {
    return (
      <Box sx={{ display: 'flex', alignItems: 'center' }}>
        <Tooltip title={error.message || 'Failed to fetch status'} arrow placement="top">
          <Chip
            icon={<AlertCircle size={14} />}
            label="Error"
            size="small"
            color="error"
            sx={{
              '& .MuiChip-icon': {
                color: 'inherit',
              },
            }}
          />
        </Tooltip>
        {refreshButton}
      </Box>
    )
  }

  if (isLoading) {
    return (
      <Box sx={{ display: 'flex', alignItems: 'center' }}>
        <Chip
          icon={<CircularProgress size={14} color="inherit" />}
          label="Loading..."
          size="small"
          color="default"
          variant="outlined"
        />
        {refreshButton}
      </Box>
    )
  }

  if (status === 'completed') {
    const formattedDate = formatLastUpdated(updatedAt)
    const tooltipContent = formattedDate
      ? `Last synced: ${formattedDate}`
      : 'Repository is indexed and up to date'

    return (
      <Box sx={{ display: 'flex', alignItems: 'center' }}>
        <Tooltip title={tooltipContent} arrow placement="top">
          <Chip
            icon={<Check size={14} />}
            label={formattedDate ? `Synced ${formattedDate}` : 'Up to date'}
            size="small"
            color="success"
            sx={{
              '& .MuiChip-icon': {
                color: 'inherit',
              },
            }}
          />
        </Tooltip>
        {refreshButton}
      </Box>
    )
  }

  if (status === 'failed') {
    return (
      <Box sx={{ display: 'flex', alignItems: 'center' }}>
        <Tooltip title={message || 'Indexing failed'} arrow placement="top">
          <Chip
            icon={<AlertCircle size={14} />}
            label="Error"
            size="small"
            color="error"
            sx={{
              '& .MuiChip-icon': {
                color: 'inherit',
              },
            }}
          />
        </Tooltip>
        {refreshButton}
      </Box>
    )
  }

  if (status === 'indexing' || status === 'in_progress') {
    return (
      <Box sx={{ display: 'flex', alignItems: 'center' }}>
        <Tooltip title={message || 'Repository is being indexed...'} arrow placement="top">
          <Chip
            icon={
              <Box
                sx={{
                  display: 'flex',
                  alignItems: 'center',
                  animation: 'spin 1.5s linear infinite',
                  '@keyframes spin': {
                    '0%': { transform: 'rotate(0deg)' },
                    '100%': { transform: 'rotate(360deg)' },
                  },
                }}
              >
                <RefreshCw size={14} />
              </Box>
            }
            label="Indexing..."
            size="small"
            color="warning"
            sx={{
              '& .MuiChip-icon': {
                color: 'inherit',
              },
            }}
          />
        </Tooltip>
        {refreshButton}
      </Box>
    )
  }

  if (status === 'queued' || status === 'pending') {
    return (
      <Box sx={{ display: 'flex', alignItems: 'center' }}>
        <Tooltip title={message || 'Repository is queued for indexing'} arrow placement="top">
          <Chip
            icon={<Clock size={14} />}
            label="Queued"
            size="small"
            color="info"
            sx={{
              '& .MuiChip-icon': {
                color: 'inherit',
              },
            }}
          />
        </Tooltip>
        {refreshButton}
      </Box>
    )
  }

  // Default: unknown state
  return (
    <Box sx={{ display: 'flex', alignItems: 'center' }}>
      <Chip
        label="Unknown"
        size="small"
        color="default"
        variant="outlined"
      />
      {refreshButton}
    </Box>
  )
}

function formatLastUpdated(dateString?: string): string {
  if (!dateString) {
    return ''
  }

  try {
    const date = new Date(dateString)
    const now = new Date()
    const diffMs = now.getTime() - date.getTime()
    const diffMinutes = Math.floor(diffMs / (1000 * 60))
    const diffHours = Math.floor(diffMs / (1000 * 60 * 60))
    const diffDays = Math.floor(diffMs / (1000 * 60 * 60 * 24))

    if (diffMinutes < 1) {
      return 'just now'
    }
    if (diffMinutes < 60) {
      return `${diffMinutes}m ago`
    }
    if (diffHours < 24) {
      return `${diffHours}h ago`
    }
    if (diffDays < 7) {
      return `${diffDays}d ago`
    }

    // For older dates, show the date
    return date.toLocaleDateString(undefined, {
      month: 'short',
      day: 'numeric',
    })
  } catch {
    return ''
  }
}

export default KoditStatusPill
