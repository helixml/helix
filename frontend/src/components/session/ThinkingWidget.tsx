import React, { useEffect, useRef, useState } from 'react'
import Box from '@mui/material/Box'
import CircularProgress from '@mui/material/CircularProgress'
import IconButton from '@mui/material/IconButton'
import Typography from '@mui/material/Typography'
import { useTheme } from '@mui/material/styles'
import ExpandLessIcon from '@mui/icons-material/ExpandLess'
import ExpandMoreIcon from '@mui/icons-material/ExpandMore'
import LightbulbOutlinedIcon from '@mui/icons-material/LightbulbOutlined'
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

  const borderColor = isDark ? 'rgba(255,255,255,0.15)' : 'rgba(0,0,0,0.12)'
  const iconColor = isDark ? 'rgba(255,255,255,0.5)' : 'rgba(0,0,0,0.45)'
  const textColor = isDark ? 'rgba(255,255,255,0.7)' : 'rgba(0,0,0,0.6)'

  return (
    <Box
      sx={{
        my: 1,
        borderLeft: `3px solid ${borderColor}`,
        borderRadius: '4px',
        overflow: 'hidden',
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
          px: 1.5,
          py: 0.75,
          cursor: 'pointer',
          backgroundColor: isDark ? 'rgba(255,255,255,0.04)' : 'rgba(0,0,0,0.03)',
          '&:hover': {
            backgroundColor: isDark ? 'rgba(255,255,255,0.08)' : 'rgba(0,0,0,0.06)',
          },
          transition: 'background-color 0.15s ease',
          userSelect: 'none',
        }}
      >
        <LightbulbOutlinedIcon sx={{ fontSize: 16, color: iconColor }} />
        <Typography
          variant="body2"
          sx={{ flex: 1, fontSize: '0.82rem', color: textColor, fontFamily: 'monospace' }}
        >
          {isStreaming ? `Thinking ${formatDuration(elapsed)}` : 'Thoughts'}
        </Typography>
        {isStreaming && <CircularProgress size={16} thickness={4} color="warning" />}
        <IconButton
          size="small"
          aria-label={expanded ? 'Collapse thoughts' : 'Expand thoughts'}
          sx={{ p: 0, ml: 0.5 }}
        >
          {expanded
            ? <ExpandLessIcon sx={{ fontSize: 18 }} />
            : <ExpandMoreIcon sx={{ fontSize: 18 }} />}
        </IconButton>
      </Box>

      {expanded && text && (
        <Box
          sx={{
            px: 1.5,
            py: 1,
            fontSize: '0.8rem',
            fontFamily: 'monospace',
            whiteSpace: 'pre-wrap',
            wordBreak: 'break-word',
            color: isDark ? 'rgba(255,255,255,0.6)' : 'rgba(0,0,0,0.55)',
            backgroundColor: isDark ? 'rgba(255,255,255,0.02)' : 'rgba(0,0,0,0.015)',
            borderTop: `1px solid ${isDark ? 'rgba(255,255,255,0.06)' : 'rgba(0,0,0,0.06)'}`,
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
