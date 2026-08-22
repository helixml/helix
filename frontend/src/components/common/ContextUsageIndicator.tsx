import { FC } from 'react'
import { Box, Tooltip, Typography, alpha } from '@mui/material'

import { useGetSessionExecutionConfig, useListInteractions } from '../../services/sessionService'

interface ContextUsageIndicatorProps {
  usedTokens?: number
  maxTokens?: number
  totalProcessedTokens?: number
  compactionAgentName?: string
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
  totalProcessedTokens,
  compactionAgentName,
}) => {
  const usageAvailable = (
    Number.isFinite(usedTokens) &&
    Number.isFinite(maxTokens) &&
    (usedTokens ?? -1) >= 0 &&
    (maxTokens ?? 0) > 0
  )
  const percentage = usageAvailable
    ? Math.max(0, Math.min(100, ((usedTokens ?? 0) / (maxTokens ?? 1)) * 100))
    : 0
  const percentageLabel = formatPercentage(percentage)
  const radius = 8.5
  const circumference = 2 * Math.PI * radius
  const dashOffset = circumference * (1 - percentage / 100)
  const overloaded = percentage > 90

  return (
    <Tooltip
      placement="top"
      arrow
      title={usageAvailable ? (
        <Box sx={{ width: 210, p: 0.5 }}>
          <Box sx={{ display: 'flex', justifyContent: 'space-between', gap: 2, mb: 0.75 }}>
            <Typography variant="caption" sx={{ fontWeight: 600 }}>
              Context window
            </Typography>
            <Typography variant="caption" sx={{ fontVariantNumeric: 'tabular-nums' }}>
              {percentageLabel} · {formatTokens(usedTokens ?? 0)}/{formatTokens(maxTokens ?? 0)}
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
          {!!totalProcessedTokens && totalProcessedTokens > 0 && (
            <Box sx={{ display: 'flex', justifyContent: 'space-between', gap: 2, mt: 1 }}>
              <Typography variant="caption" color="text.secondary">
                Total processed
              </Typography>
              <Typography variant="caption" color="text.secondary" sx={{ fontVariantNumeric: 'tabular-nums' }}>
                {formatTokens(totalProcessedTokens)}
              </Typography>
            </Box>
          )}
          {!!compactionAgentName && (
            <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 1 }}>
              {compactionAgentName} automatically compacts its context when needed.
            </Typography>
          )}
        </Box>
      ) : (
        <Box sx={{ p: 0.5 }}>
          <Typography variant="caption" sx={{ display: 'block', fontWeight: 600 }}>
            Context window
          </Typography>
          <Typography variant="caption" color="text.secondary">
            Available after the next agent response
          </Typography>
        </Box>
      )}
    >
      <Box
        data-context-usage-indicator
        role={usageAvailable ? 'progressbar' : 'img'}
        aria-label={usageAvailable
          ? `Context window ${percentageLabel} used`
          : 'Context window usage unavailable'}
        aria-valuemin={usageAvailable ? 0 : undefined}
        aria-valuemax={usageAvailable ? 100 : undefined}
        aria-valuenow={usageAvailable ? Math.round(percentage) : undefined}
        sx={{
          // Matches the composer's icon buttons it sits beside — it was a 30px
          // box around a 20px glyph next to 28px boxes around 17px ones, which
          // read as one control being bigger than the rest.
          width: 28,
          height: 28,
          flexShrink: 0,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          color: overloaded
            ? 'error.main'
            : 'text.secondary',
          '& .context-usage-track': {
            stroke: (theme) => alpha(theme.palette.text.secondary, usageAvailable ? 0.22 : 0.42),
          },
        }}
      >
        <Box
          component="svg"
          viewBox="0 0 20 20"
          aria-hidden="true"
          sx={{ width: 17, height: 17, transform: 'rotate(-90deg)' }}
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
  const { data: executionConfig } = useGetSessionExecutionConfig(sessionId)
  const interactions = data?.data?.interactions ?? []
  const usage = interactions.find((interaction) => (
    (interaction.usage?.context_length ?? 0) > 0 &&
    (interaction.usage?.context_tokens ?? -1) >= 0
  ))?.usage
  const compactionAgentName = (() => {
    switch (executionConfig?.runtime) {
      case 'codex_cli':
        return 'Codex'
      case 'zed_agent':
        return (usage?.context_length ?? 0) >= 80_000 ? 'Zed Agent' : undefined
      default:
        return undefined
    }
  })()

  return (
    <ContextUsageIndicator
      usedTokens={usage?.context_tokens}
      maxTokens={usage?.context_length}
      totalProcessedTokens={usage?.total_processed_tokens}
      compactionAgentName={compactionAgentName}
    />
  )
}
