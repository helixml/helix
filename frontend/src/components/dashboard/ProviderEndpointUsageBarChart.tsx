import React from 'react';
import { Box, Tooltip, Typography } from '@mui/material';
import useLightTheme from '../../hooks/useLightTheme';
import { TypesAggregatedUsageMetric } from '../../api/api';

interface ProviderEndpointUsageBarChartProps {
  data: TypesAggregatedUsageMetric[] | null | undefined;
  onClick?: () => void;
}

const BAR_LIMIT = 20;
const CHART_WIDTH = 200;
const CHART_HEIGHT = 50;
const CHART_TOP = 5;
const CHART_BOTTOM = 45;
const BAR_WIDTH = 8;
const BAR_SPACING = 2;

// Emerald palette, bottom → top
const COLOR_PROMPT = '#065F46';       // input (darkest)
const COLOR_CACHE_READ = '#10B981';
const COLOR_CACHE_WRITE = '#34D399';
const COLOR_COMPLETION = '#6EE7B7';   // output (lightest)

const formatDate = (dateString?: string) => {
  if (!dateString) return '';
  const date = new Date(dateString);
  return date.toLocaleDateString('en-US', {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
  });
};

const formatTokens = (value?: number) => (value ?? 0).toLocaleString();

const formatCost = (value?: number) => {
  const v = value ?? 0;
  if (v === 0) return '$0.00';
  if (v < 0.01) return `$${v.toFixed(4)}`;
  return `$${v.toFixed(2)}`;
};

interface StackSegment {
  label: string;
  tokens: number;
  cost: number;
  color: string;
}

const buildSegments = (point: TypesAggregatedUsageMetric): StackSegment[] => [
  {
    label: 'Input',
    tokens: point.prompt_tokens ?? 0,
    cost: point.prompt_cost ?? 0,
    color: COLOR_PROMPT,
  },
  {
    label: 'Cache read',
    tokens: point.cache_read_tokens ?? 0,
    cost: point.cache_read_cost ?? 0,
    color: COLOR_CACHE_READ,
  },
  {
    label: 'Cache write',
    tokens: point.cache_write_tokens ?? 0,
    cost: point.cache_write_cost ?? 0,
    color: COLOR_CACHE_WRITE,
  },
  {
    label: 'Output',
    tokens: point.completion_tokens ?? 0,
    cost: point.completion_cost ?? 0,
    color: COLOR_COMPLETION,
  },
];

const LegendSwatch: React.FC<{ color: string; label: string; tokens: number; cost: number }> = ({
  color,
  label,
  tokens,
  cost,
}) => (
  <Box
    sx={{
      display: 'grid',
      gridTemplateColumns: '10px 1fr auto auto',
      alignItems: 'center',
      gap: 1,
      fontSize: 12,
    }}
  >
    <Box sx={{ width: 10, height: 10, bgcolor: color, borderRadius: '2px' }} />
    <Box>{label}</Box>
    <Box sx={{ fontVariantNumeric: 'tabular-nums' }}>{formatTokens(tokens)}</Box>
    <Box sx={{ fontVariantNumeric: 'tabular-nums', color: 'text.secondary', minWidth: 48, textAlign: 'right' }}>
      {formatCost(cost)}
    </Box>
  </Box>
);

const BarTooltipContent: React.FC<{ point: TypesAggregatedUsageMetric }> = ({ point }) => {
  const segments = buildSegments(point);
  const totalTokens = point.total_tokens ?? segments.reduce((acc, s) => acc + s.tokens, 0);
  const totalCost = point.total_cost ?? segments.reduce((acc, s) => acc + s.cost, 0);
  const requests = point.total_requests ?? 0;
  const latency = point.latency_ms ?? 0;

  return (
    <Box sx={{ p: 0.5, minWidth: 240 }}>
      <Typography variant="caption" sx={{ fontWeight: 600, display: 'block', mb: 0.5 }}>
        {formatDate(point.date)}
      </Typography>
      <Box sx={{ display: 'flex', flexDirection: 'column', gap: 0.25, mb: 0.75 }}>
        {segments.map((segment) => (
          <LegendSwatch
            key={segment.label}
            color={segment.color}
            label={segment.label}
            tokens={segment.tokens}
            cost={segment.cost}
          />
        ))}
      </Box>
      <Box
        sx={{
          borderTop: '1px solid rgba(255,255,255,0.15)',
          pt: 0.5,
          display: 'flex',
          flexDirection: 'column',
          gap: 0.25,
          fontSize: 12,
        }}
      >
        <Box sx={{ display: 'flex', justifyContent: 'space-between' }}>
          <span>Total tokens</span>
          <span style={{ fontVariantNumeric: 'tabular-nums', fontWeight: 600 }}>
            {formatTokens(totalTokens)}
          </span>
        </Box>
        <Box sx={{ display: 'flex', justifyContent: 'space-between' }}>
          <span>Total cost</span>
          <span style={{ fontVariantNumeric: 'tabular-nums', fontWeight: 600 }}>
            {formatCost(totalCost)}
          </span>
        </Box>
        <Box sx={{ display: 'flex', justifyContent: 'space-between' }}>
          <span>Requests</span>
          <span style={{ fontVariantNumeric: 'tabular-nums' }}>{requests.toLocaleString()}</span>
        </Box>
        {latency > 0 && (
          <Box sx={{ display: 'flex', justifyContent: 'space-between' }}>
            <span>Avg latency</span>
            <span style={{ fontVariantNumeric: 'tabular-nums' }}>{latency.toLocaleString()} ms</span>
          </Box>
        )}
      </Box>
    </Box>
  );
};

const ProviderEndpointUsageBarChart: React.FC<ProviderEndpointUsageBarChartProps> = ({
  data,
  onClick,
}) => {
  const lightTheme = useLightTheme();
  const points = React.useMemo(() => {
    if (!data || data.length === 0) return [];
    const sorted = [...data].sort(
      (a, b) => new Date(a.date || '').getTime() - new Date(b.date || '').getTime(),
    );
    return sorted.slice(-BAR_LIMIT);
  }, [data]);

  if (!points.length) {
    return (
      <Box
        sx={{
          width: CHART_WIDTH,
          height: CHART_HEIGHT,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          color: lightTheme.isLight ? '#475569' : '#6B7280',
          fontSize: 12,
        }}
      >
        No usage
      </Box>
    );
  }

  const maxTotal = Math.max(
    1,
    ...points.map((p) => buildSegments(p).reduce((acc, s) => acc + s.tokens, 0)),
  );
  const availableHeight = CHART_BOTTOM - CHART_TOP;

  return (
    <Box
      sx={{
        width: CHART_WIDTH,
        height: CHART_HEIGHT,
        cursor: onClick ? 'pointer' : 'default',
      }}
      onClick={onClick}
    >
      <svg width={CHART_WIDTH} height={CHART_HEIGHT} style={{ overflow: 'visible' }}>
        {points.map((point, index) => {
          const segments = buildSegments(point);
          const x = index * (BAR_WIDTH + BAR_SPACING) + 10;
          let yCursor = CHART_BOTTOM;
          const rects: React.ReactNode[] = [];
          segments.forEach((segment) => {
            if (segment.tokens <= 0) return;
            const height = (segment.tokens / maxTotal) * availableHeight;
            if (height <= 0) return;
            yCursor -= height;
            rects.push(
              <rect
                key={segment.label}
                x={x}
                y={yCursor}
                width={BAR_WIDTH}
                height={height}
                fill={segment.color}
                opacity={0.9}
              />,
            );
          });

          // Zero-usage placeholder so the slot is still hoverable.
          if (rects.length === 0) {
            rects.push(
              <rect
                key="empty"
                x={x}
                y={CHART_BOTTOM - 2}
                width={BAR_WIDTH}
                height={2}
                fill={lightTheme.isLight ? '#cbd5e1' : '#374151'}
                opacity={0.4}
                rx={1}
              />,
            );
          }

          return (
            <Tooltip
              key={point.date || index}
              title={<BarTooltipContent point={point} />}
              placement="top"
              arrow
            >
              <g style={{ cursor: onClick ? 'pointer' : 'default' }}>
                <rect
                  x={x - 1}
                  y={CHART_TOP}
                  width={BAR_WIDTH + 2}
                  height={availableHeight}
                  fill="transparent"
                />
                {rects}
              </g>
            </Tooltip>
          );
        })}
      </svg>
    </Box>
  );
};

export default ProviderEndpointUsageBarChart;
