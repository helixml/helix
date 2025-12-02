import { FC } from 'react'
import {
  Box,
  Chip,
  CircularProgress,
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
}

const KoditStatusPill: FC<KoditStatusPillProps> = ({
  data,
  isLoading,
  error,
}) => {
  const attrs = data?.data?.attributes
  const status = attrs?.status
  const message = attrs?.message
  const updatedAt = attrs?.updated_at

  // Handle API errors first
  if (error) {
    return (
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
    )
  }

  if (isLoading) {
    return (
      <Chip
        icon={<CircularProgress size={14} color="inherit" />}
        label="Loading..."
        size="small"
        color="default"
        variant="outlined"
      />
    )
  }

  if (status === 'completed') {
    const formattedDate = formatLastUpdated(updatedAt)
    const tooltipContent = formattedDate
      ? `Last synced: ${formattedDate}`
      : 'Repository is indexed and up to date'

    return (
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
    )
  }

  if (status === 'failed') {
    return (
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
    )
  }

  if (status === 'indexing' || status === 'in_progress') {
    return (
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
    )
  }

  if (status === 'queued') {
    return (
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
    )
  }

  // Default: unknown state
  return (
    <Chip
      label="Unknown"
      size="small"
      color="default"
      variant="outlined"
    />
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
