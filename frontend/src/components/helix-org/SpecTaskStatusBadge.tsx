import { FC } from 'react'
import Box from '@mui/material/Box'
import Chip from '@mui/material/Chip'

type StatusTone = 'neutral' | 'queued' | 'active' | 'review' | 'success' | 'error' | 'pullRequest'

export const formatSpecTaskStatus = (status?: string): string => {
  if (!status) return 'Unknown'
  return status
    .split('_')
    .map((word) => word.charAt(0).toUpperCase() + word.slice(1))
    .join(' ')
}

const statusTone = (status?: string): StatusTone => {
  switch (status) {
    case 'done':
    case 'completed':
    case 'implementation_complete':
    case 'spec_approved':
      return 'success'
    case 'implementation_review':
    case 'spec_review':
    case 'spec_revision':
      return 'review'
    case 'implementation_failed':
    case 'spec_failed':
    case 'failed':
      return 'error'
    case 'implementation':
    case 'implementing':
    case 'implementation_in_progress':
      return 'active'
    case 'implementation_queued':
    case 'queued_implementation':
    case 'spec_generation':
    case 'queued_spec_generation':
    case 'ready':
      return 'queued'
    case 'pull_request':
      return 'pullRequest'
    default:
      return 'neutral'
  }
}

const palettes = {
  neutral: {
    light: { color: '#475569', background: 'rgba(100,116,139,0.10)', border: 'rgba(100,116,139,0.22)' },
    dark: { color: '#cbd5e1', background: 'rgba(148,163,184,0.12)', border: 'rgba(148,163,184,0.24)' },
  },
  queued: {
    light: { color: '#1d4ed8', background: 'rgba(59,130,246,0.10)', border: 'rgba(59,130,246,0.24)' },
    dark: { color: '#93c5fd', background: 'rgba(59,130,246,0.14)', border: 'rgba(96,165,250,0.28)' },
  },
  active: {
    light: { color: '#6d28d9', background: 'rgba(124,58,237,0.10)', border: 'rgba(124,58,237,0.24)' },
    dark: { color: '#c4b5fd', background: 'rgba(139,92,246,0.14)', border: 'rgba(167,139,250,0.28)' },
  },
  review: {
    light: { color: '#b45309', background: 'rgba(245,158,11,0.11)', border: 'rgba(245,158,11,0.28)' },
    dark: { color: '#fcd34d', background: 'rgba(245,158,11,0.14)', border: 'rgba(251,191,36,0.28)' },
  },
  success: {
    light: { color: '#047857', background: 'rgba(16,185,129,0.10)', border: 'rgba(16,185,129,0.24)' },
    dark: { color: '#6ee7b7', background: 'rgba(16,185,129,0.14)', border: 'rgba(52,211,153,0.28)' },
  },
  error: {
    light: { color: '#b91c1c', background: 'rgba(239,68,68,0.10)', border: 'rgba(239,68,68,0.24)' },
    dark: { color: '#fca5a5', background: 'rgba(239,68,68,0.14)', border: 'rgba(248,113,113,0.28)' },
  },
  pullRequest: {
    light: { color: '#0e7490', background: 'rgba(6,182,212,0.10)', border: 'rgba(6,182,212,0.24)' },
    dark: { color: '#67e8f9', background: 'rgba(6,182,212,0.14)', border: 'rgba(34,211,238,0.28)' },
  },
} as const

const SpecTaskStatusBadge: FC<{ status?: string }> = ({ status }) => {
  const tone = statusTone(status)

  return (
    <Chip
      icon={<Box sx={{ width: 6, height: 6, borderRadius: '50%', backgroundColor: 'currentColor' }} />}
      label={formatSpecTaskStatus(status)}
      size="small"
      sx={(theme) => {
        const palette = palettes[tone][theme.palette.mode]
        return {
          maxWidth: 160,
          height: 24,
          color: palette.color,
          backgroundColor: palette.background,
          border: `1px solid ${palette.border}`,
          borderRadius: '999px',
          fontSize: '0.7rem',
          fontWeight: 650,
          letterSpacing: '0.01em',
          '& .MuiChip-icon': {
            color: 'inherit',
            marginLeft: '8px',
            marginRight: '-2px',
          },
          '& .MuiChip-label': {
            overflow: 'hidden',
            textOverflow: 'ellipsis',
          },
        }
      }}
    />
  )
}

export default SpecTaskStatusBadge
