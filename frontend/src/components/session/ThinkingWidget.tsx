import React, { useEffect, useRef, useState } from 'react'
import Box from '@mui/material/Box'
import CircularProgress from '@mui/material/CircularProgress'
import IconButton from '@mui/material/IconButton'
import Typography from '@mui/material/Typography'
import { useTheme } from '@mui/material/styles'
import { ChevronDown, ChevronUp, Lightbulb } from 'lucide-react'
import { preserveDisclosureExpansion } from './disclosureScroll'

interface ThinkingWidgetProps {
  text: string
  startTime?: number | Date
  isStreaming: boolean
  compact?: boolean
}

function formatDuration(seconds: number) {
  const minutes = Math.floor(seconds / 60)
  const remainder = seconds % 60
  return `${minutes}:${remainder.toString().padStart(2, '0')}`
}

const ThinkingWidget: React.FC<ThinkingWidgetProps> = ({ text, startTime, isStreaming }) => {
  const [elapsed, setElapsed] = useState(0)
  const [expanded, setExpanded] = useState(false)
  const theme = useTheme()
  const isDark = theme.palette.mode === 'dark'
  const startedAt = useRef(
    typeof startTime === 'number'
      ? startTime
      : startTime instanceof Date
        ? startTime.getTime()
        : Date.now(),
  )

  useEffect(() => {
    if (!isStreaming) return
    const updateElapsed = () => setElapsed(Math.floor((Date.now() - startedAt.current) / 1000))
    updateElapsed()
    const interval = window.setInterval(updateElapsed, 1000)
    return () => window.clearInterval(interval)
  }, [isStreaming])

  const iconColor = isDark ? 'rgba(255,255,255,0.5)' : 'rgba(0,0,0,0.45)'
  const textColor = isDark ? 'rgba(255,255,255,0.65)' : 'text.secondary'

  return (
    <Box
      sx={{
        my: 0.75,
      }}
    >
      <Box
        onClick={(event) => {
          if (!expanded) preserveDisclosureExpansion(event.currentTarget)
          setExpanded((value) => !value)
        }}
        sx={{
          display: 'flex',
          alignItems: 'center',
          gap: 0.75,
          px: 0,
          py: 0.5,
          cursor: 'pointer',
          backgroundColor: 'transparent',
          '&:hover': {
            backgroundColor: 'transparent',
          },
          userSelect: 'none',
        }}
      >
        <Lightbulb size={15} strokeWidth={1.8} color={iconColor} aria-hidden="true" />
        <Typography
          variant="body2"
          sx={{ flex: 1, fontSize: '0.76rem', color: textColor, fontFamily: 'monospace' }}
        >
          {isStreaming ? `Thinking ${formatDuration(elapsed)}` : 'Thoughts'}
        </Typography>
        {isStreaming && <CircularProgress size={16} thickness={4} color="warning" />}
        <IconButton
          size="small"
          aria-label={expanded ? 'Collapse thoughts' : 'Expand thoughts'}
          sx={{ p: 0, ml: 0.5, '&:hover': { backgroundColor: 'transparent' } }}
        >
          {expanded ? <ChevronUp size={15} strokeWidth={1.8} /> : <ChevronDown size={15} strokeWidth={1.8} />}
        </IconButton>
      </Box>

      {expanded && text && (
        <Box
          sx={{
            pl: 2.5,
            pr: 0,
            py: 1,
            fontSize: '0.8rem',
            fontFamily: 'monospace',
            whiteSpace: 'pre-wrap',
            wordBreak: 'break-word',
            color: isDark ? 'rgba(255,255,255,0.55)' : 'text.secondary',
            backgroundColor: 'transparent',
            maxHeight: '300px',
            overflow: 'auto',
          }}
        >
          {text}
        </Box>
      )}
    </Box>
  )
}

export default ThinkingWidget
