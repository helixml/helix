import React, { useEffect, useRef, useState } from 'react'
import Box from '@mui/material/Box'
import CircularProgress from '@mui/material/CircularProgress'
import IconButton from '@mui/material/IconButton'
import Typography from '@mui/material/Typography'
import { useTheme } from '@mui/material/styles'
import { ChevronDown, ChevronUp, Lightbulb } from 'lucide-react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { preserveDisclosureExpansion } from './disclosureScroll'
import { getChatColors } from './chatStyles'
import { APP_MONO_FONT_FAMILY } from '../../styles/typography'

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

export function formatThinkingMarkdown(text: string): string {
  const blocks = text.trim().split(/\n{2,}/)
  let markdown = ''
  let previousWasSummary = false

  blocks.forEach((block) => {
    const trimmed = block.trim()
    if (!trimmed) return
    const isSummary = /^\*\*[\s\S]+\*\*$/.test(trimmed)
    const separator = markdown ? (isSummary && previousWasSummary ? '\n' : '\n\n') : ''
    markdown += `${separator}${isSummary ? `- ${trimmed}` : trimmed}`
    previousWasSummary = isSummary
  })

  return markdown
}

export function thinkingSummary(text: string): string {
  const firstLine = text
    .split(/\n+/)
    .map((line) => line.trim())
    .find(Boolean) || ''

  return firstLine
    .replace(/\*\*(.*?)\*\*/g, '$1')
    .replace(/`([^`]+)`/g, '$1')
    .replace(/^[-*]\s+/, '')
}

const ThinkingWidget: React.FC<ThinkingWidgetProps> = ({ text, startTime, isStreaming }) => {
  const [elapsed, setElapsed] = useState(0)
  const [expanded, setExpanded] = useState(false)
  const theme = useTheme()
  const isDark = theme.palette.mode === 'dark'
  const chatColors = getChatColors(theme)
  const isMultiline = text.trim().includes('\n')
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

  const iconColor = isDark ? chatColors.subtle : 'rgba(0,0,0,0.45)'
  const textColor = isDark ? chatColors.muted : 'text.secondary'

  return (
    <Box
      sx={{
        my: 0.75,
      }}
    >
      <Box
        onClick={isMultiline ? (event) => {
          if (!expanded) preserveDisclosureExpansion(event.currentTarget)
          setExpanded((value) => !value)
        } : undefined}
        sx={{
          display: 'flex',
          alignItems: 'center',
          gap: 0.75,
          px: 0,
          py: 0.5,
          cursor: isMultiline ? 'pointer' : 'default',
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
          sx={{
            flex: 1,
            minWidth: 0,
            overflow: 'hidden',
            textOverflow: 'ellipsis',
            whiteSpace: 'nowrap',
            fontSize: '0.76rem',
            color: textColor,
            fontFamily: APP_MONO_FONT_FAMILY,
          }}
        >
          {isStreaming ? `Thinking ${formatDuration(elapsed)}` : thinkingSummary(text)}
        </Typography>
        {isStreaming && <CircularProgress size={16} thickness={4} color="warning" />}
        {isMultiline && (
          <IconButton
            size="small"
            aria-label={expanded ? 'Collapse thoughts' : 'Expand thoughts'}
            sx={{ p: 0, ml: 0.5, '&:hover': { backgroundColor: 'transparent' } }}
          >
            {expanded ? <ChevronUp size={15} strokeWidth={1.8} /> : <ChevronDown size={15} strokeWidth={1.8} />}
          </IconButton>
        )}
      </Box>

      {expanded && text && (
        <Box
          sx={{
            pl: 2.5,
            pr: 0,
            py: 1,
            fontSize: '0.8rem',
            lineHeight: 1.6,
            wordBreak: 'break-word',
            color: isDark ? chatColors.muted : 'text.secondary',
            backgroundColor: 'transparent',
            maxHeight: '300px',
            overflow: 'auto',
            '& p': { m: 0 },
            '& p + p': { mt: 1 },
            '& ul, & ol': { my: 0, pl: 2.5 },
            '& li + li': { mt: 0.5 },
            '& strong': {
              color: isDark ? '#d4d4d4' : 'text.primary',
              fontWeight: 600,
            },
            '& code': {
              fontFamily: APP_MONO_FONT_FAMILY,
              fontSize: '0.76rem',
            },
          }}
        >
          <ReactMarkdown remarkPlugins={[remarkGfm]}>
            {formatThinkingMarkdown(text)}
          </ReactMarkdown>
        </Box>
      )}
    </Box>
  )
}

export default ThinkingWidget
