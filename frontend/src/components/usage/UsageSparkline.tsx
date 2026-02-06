import React, { FC, useState } from 'react'
import {
  Box,
  Typography,
  Popper,
  Paper,
  Fade,
} from '@mui/material'
import type { TypesAggregatedUsageMetric } from '../../api/api'

export const formatNumber = (num: number) => {
  if (num >= 1000000) return `${(num / 1000000).toFixed(1)}M`
  if (num >= 1000) return `${(num / 1000).toFixed(1)}K`
  return num.toString()
}

interface UsageSparklineProps {
  data: TypesAggregatedUsageMetric[]
  color?: string
  height?: number
  showTooltip?: boolean
}

const UsageSparkline: FC<UsageSparklineProps> = ({ 
  data, 
  color = '#10b981',
  height = 32,
  showTooltip = true,
}) => {
  const [hoveredIndex, setHoveredIndex] = useState<number | null>(null)
  const [anchorEl, setAnchorEl] = useState<HTMLElement | null>(null)
  const containerRef = React.useRef<HTMLDivElement>(null)

  if (!data || data.length === 0) {
    return (
      <Box sx={{ height, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
        <Typography variant="caption" sx={{ color: 'text.disabled', fontSize: '0.65rem' }}>
          No usage data
        </Typography>
      </Box>
    )
  }

  const tokenData = data.map(m => m.total_tokens || 0)
  const max = Math.max(...tokenData, 1)
  const min = Math.min(...tokenData, 0)
  const range = max - min || 1
  const width = 100
  const padding = 2

  const points = tokenData.map((value, index) => {
    const x = padding + (index / (tokenData.length - 1 || 1)) * (width - padding * 2)
    const y = height - padding - ((value - min) / range) * (height - padding * 2)
    return `${x},${y}`
  }).join(' ')

  const areaPoints = `${padding},${height - padding} ${points} ${width - padding},${height - padding}`

  const handleMouseMove = (event: React.MouseEvent<SVGRectElement>) => {
    if (!showTooltip) return
    const rect = event.currentTarget.getBoundingClientRect()
    const x = event.clientX - rect.left
    const relativeX = x / rect.width
    const index = Math.round(relativeX * (data.length - 1))
    const clampedIndex = Math.max(0, Math.min(data.length - 1, index))
    setHoveredIndex(clampedIndex)
    setAnchorEl(containerRef.current)
  }

  const handleMouseLeave = () => {
    setHoveredIndex(null)
    setAnchorEl(null)
  }

  const hoveredData = hoveredIndex !== null ? data[hoveredIndex] : null
  const hoveredX = hoveredIndex !== null 
    ? padding + (hoveredIndex / (data.length - 1 || 1)) * (width - padding * 2)
    : 0

  return (
    <Box sx={{ position: 'relative' }} ref={containerRef}>
      <svg width="100%" height={height} viewBox={`0 0 ${width} ${height}`} preserveAspectRatio="none">
        <defs>
          <linearGradient id={`gradient-${color.replace('#', '')}`} x1="0%" y1="0%" x2="0%" y2="100%">
            <stop offset="0%" stopColor={color} stopOpacity="0.3" />
            <stop offset="100%" stopColor={color} stopOpacity="0" />
          </linearGradient>
        </defs>
        <polygon
          points={areaPoints}
          fill={`url(#gradient-${color.replace('#', '')})`}
        />
        <polyline
          points={points}
          fill="none"
          stroke={color}
          strokeWidth="2"
          strokeLinecap="round"
          strokeLinejoin="round"
        />
        {hoveredIndex !== null && (
          <line
            x1={hoveredX}
            y1={0}
            x2={hoveredX}
            y2={height}
            stroke="rgba(255,255,255,0.5)"
            strokeWidth="0.5"
            strokeDasharray="1,1"
          />
        )}
        <rect
          x={0}
          y={0}
          width={width}
          height={height}
          fill="transparent"
          style={{ cursor: showTooltip ? 'crosshair' : 'default' }}
          onMouseMove={handleMouseMove}
          onMouseLeave={handleMouseLeave}
        />
      </svg>
      {showTooltip && (
        <Popper
          open={hoveredIndex !== null}
          anchorEl={anchorEl}
          placement="top"
          modifiers={[{ name: 'offset', options: { offset: [0, 8] } }]}
          sx={{ zIndex: 1500 }}
        >
          <Fade in={hoveredIndex !== null} timeout={150}>
            <Paper sx={{
              p: 1,
              backgroundColor: 'rgba(30, 30, 30, 0.95)',
              border: '1px solid rgba(255,255,255,0.1)',
              borderRadius: 1,
            }}>
              {hoveredData && (
                <Box sx={{ minWidth: 100 }}>
                  <Typography variant="caption" sx={{ color: 'text.secondary', display: 'block', mb: 0.5 }}>
                    {new Date(hoveredData.date || '').toLocaleDateString(undefined, { weekday: 'short', month: 'short', day: 'numeric' })}
                  </Typography>
                  <Box sx={{ display: 'flex', justifyContent: 'space-between', gap: 2 }}>
                    <Typography variant="caption" sx={{ color: 'text.secondary' }}>Tokens:</Typography>
                    <Typography variant="caption" sx={{ color: 'text.primary', fontWeight: 600, fontFamily: 'monospace' }}>
                      {formatNumber(hoveredData.total_tokens || 0)}
                    </Typography>
                  </Box>
                  <Box sx={{ display: 'flex', justifyContent: 'space-between', gap: 2 }}>
                    <Typography variant="caption" sx={{ color: 'text.secondary' }}>Requests:</Typography>
                    <Typography variant="caption" sx={{ color: 'text.primary', fontWeight: 600, fontFamily: 'monospace' }}>
                      {formatNumber(hoveredData.total_requests || 0)}
                    </Typography>
                  </Box>
                </Box>
              )}
            </Paper>
          </Fade>
        </Popper>
      )}
    </Box>
  )
}

export default UsageSparkline
