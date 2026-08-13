import { FC } from 'react'
import { Box, Tooltip, Typography, alpha } from '@mui/material'

import { useListInteractions } from '../../services/sessionService'

interface ContextUsageIndicatorProps {
  usedTokens: number
  maxTokens: number
}

const formatTokens = (value: number): string => {
  if (value < 1_000) return Math.round(value).toString()
  if (value < 10_000) return `${(value / 1_000).toFixed(1).replace(/\.0$/, '')}k`
  if (value < 1_000_000) return `${Math.round(value / 1_000)}k`
  return `${(value / 1_000_000).toFixed(1).replace(/\.0$/, '')}m`
}

const formatPercentage = (value: number): string => (
  value < 10 ? `${value.toFixed(1).replace(/\.0$/, '')}%` : `${Math.round(value)}%`
)

export const ContextUsageIndicator: FC<ContextUsageIndicatorProps> = ({
  usedTokens,
  maxTokens,
}) => {
  if (!Number.isFinite(usedTokens) || !Number.isFinite(maxTokens) || usedTokens < 0 || maxTokens <= 0) {
    return null
  }

  const percentage = Math.max(0, Math.min(100, (usedTokens / maxTokens) * 100))
  const percentageLabel = formatPercentage(percentage)
  const radius = 8.5
  const circumference = 2 * Math.PI * radius
  const dashOffset = circumference * (1 - percentage / 100)
  const overloaded = percentage > 90

  return (
    <Tooltip
      placement="top"
      arrow
      title={(
        <Box sx={{ width: 210, p: 0.5 }}>
          <Box sx={{ display: 'flex', justifyContent: 'space-between', gap: 2, mb: 0.75 }}>
            <Typography variant="caption" sx={{ fontWeight: 600 }}>
              Context window
            </Typography>
            <Typography variant="caption" sx={{ fontVariantNumeric: 'tabular-nums' }}>
              {percentageLabel} · {formatTokens(usedTokens)}/{formatTokens(maxTokens)}
            </Typography>
          </Box>
          <Box
            sx={{
              height: 5,
              overflow: 'hidden',
              borderRadius: 999,
              bgcolor: (theme) => alpha(theme.palette.common.white, 0.18),
            }}
          >
            <Box
              sx={{
                width: `${percentage}%`,
                height: '100%',
                borderRadius: 'inherit',
                bgcolor: overloaded ? 'error.main' : 'text.secondary',
              }}
            />
          </Box>
        </Box>
      )}
    >
      <Box
        data-context-usage-indicator
        role="progressbar"
        aria-label={`Context window ${percentageLabel} used`}
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={Math.round(percentage)}
        sx={{
          width: 30,
          height: 30,
          flexShrink: 0,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          color: overloaded
            ? 'error.main'
            : 'text.secondary',
          '& .context-usage-track': {
            stroke: (theme) => alpha(theme.palette.text.secondary, 0.22),
          },
        }}
      >
        <Box
          component="svg"
          viewBox="0 0 20 20"
          aria-hidden="true"
          sx={{ width: 20, height: 20, transform: 'rotate(-90deg)' }}
        >
          <circle
            className="context-usage-track"
            cx="10"
            cy="10"
            r={radius}
            fill="none"
            strokeWidth="2.5"
          />
          <circle
            cx="10"
            cy="10"
            r={radius}
            fill="none"
            stroke="currentColor"
            strokeWidth="2.5"
            strokeLinecap="round"
            strokeDasharray={circumference}
            strokeDashoffset={dashOffset}
            style={{ transition: 'stroke-dashoffset 500ms ease-out' }}
          />
        </Box>
      </Box>
    </Tooltip>
  )
}

export const SessionContextUsageIndicator: FC<{ sessionId: string }> = ({ sessionId }) => {
  const { data } = useListInteractions(sessionId, 0, 5, 'desc', {
    enabled: !!sessionId,
    refetchInterval: 3000,
  })
  const interactions = data?.data?.interactions ?? []
  const usage = interactions.find((interaction) => (
    (interaction.usage?.context_length ?? 0) > 0 &&
    (interaction.usage?.context_tokens ?? -1) >= 0
  ))?.usage

  if (usage?.context_tokens === undefined || !usage.context_length) return null
  return (
    <ContextUsageIndicator
      usedTokens={usage.context_tokens}
      maxTokens={usage.context_length}
    />
  )
}
