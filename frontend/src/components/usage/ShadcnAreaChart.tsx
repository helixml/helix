import React, { FC } from 'react';
import { Box, Typography } from '@mui/material';
import {
  Area,
  AreaChart,
  CartesianGrid,
  Legend,
  ResponsiveContainer,
  Tooltip,
  TooltipProps,
  XAxis,
  YAxis,
} from 'recharts';

export interface ShadcnSeries {
  key: string;
  label: string;
  color: string;
}

export interface ShadcnAreaChartProps {
  /** Title rendered above the chart (e.g. "TOTAL TOKENS"). */
  title: string;
  /** Headline value (already formatted, e.g. "$12.45" or "1.2M"). */
  headline: string;
  /** Chart data rows; `date` is ISO string, other keys are series values. */
  data: Array<{ date: string } & Record<string, number>>;
  /** Series to render as stacked areas, in bottom-to-top order. */
  series: ShadcnSeries[];
  /** Formatter for Y-axis ticks and tooltip values. */
  valueFormatter: (value: number) => string;
  /** Stack areas (default) or render side-by-side. */
  stacked?: boolean;
  /** Hide the legend when there's only a single series. */
  hideLegend?: boolean;
  /** Height for the chart body (title bar sits above). Default 220. */
  chartHeight?: number;
}

const uid = () => Math.random().toString(36).slice(2, 9);

const ShadcnTooltip = (seriesConfig: ShadcnSeries[], valueFormatter: (v: number) => string) => {
  const TooltipComponent: FC<TooltipProps<number, string>> = ({ active, payload, label }) => {
    if (!active || !payload || !payload.length) return null;
    const date = label ? new Date(label as string) : null;
    const dateLabel = date
      ? date.toLocaleDateString(undefined, { month: 'short', day: 'numeric' })
      : '';
    const byKey = new Map(payload.map(p => [p.dataKey as string, p]));
    return (
      <Box
        sx={{
          bgcolor: 'rgba(10, 10, 15, 0.95)',
          border: '1px solid rgba(255, 255, 255, 0.12)',
          borderRadius: 2,
          px: 1.5,
          py: 1,
          fontSize: '0.8rem',
          boxShadow: '0 4px 12px rgba(0,0,0,0.4)',
          minWidth: 140,
        }}
      >
        <Typography
          variant="caption"
          sx={{ color: 'rgba(255,255,255,0.6)', display: 'block', mb: 0.5 }}
        >
          {dateLabel}
        </Typography>
        {seriesConfig.map(s => {
          const entry = byKey.get(s.key);
          const value = typeof entry?.value === 'number' ? entry.value : 0;
          if (!value) return null;
          return (
            <Box
              key={s.key}
              sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 0.25 }}
            >
              <Box sx={{ width: 8, height: 8, borderRadius: '2px', bgcolor: s.color }} />
              <Typography variant="caption" sx={{ color: 'rgba(255,255,255,0.85)', flex: 1 }}>
                {s.label}
              </Typography>
              <Typography
                variant="caption"
                sx={{
                  color: '#fff',
                  fontVariantNumeric: 'tabular-nums',
                  fontWeight: 500,
                }}
              >
                {valueFormatter(value)}
              </Typography>
            </Box>
          );
        })}
      </Box>
    );
  };
  return TooltipComponent;
};

const ShadcnAreaChart: FC<ShadcnAreaChartProps> = ({
  title,
  headline,
  data,
  series,
  valueFormatter,
  stacked = true,
  hideLegend = false,
  chartHeight = 220,
}) => {
  // Unique gradient ids so multiple charts on the same page don't collide.
  const gradientPrefix = React.useMemo(() => `shadcn-${uid()}`, []);

  const hasData = data.some(d => series.some(s => (d[s.key] || 0) > 0));

  return (
    <Box
      sx={{
        height: 300,
        bgcolor: 'rgba(0, 0, 0, 0.2)',
        borderRadius: 2,
        p: 2,
        position: 'relative',
      }}
    >
      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 1 }}>
        <Typography
          variant="body2"
          sx={{
            color: 'text.secondary',
            fontSize: '0.875rem',
            fontWeight: 500,
            letterSpacing: '0.04em',
          }}
        >
          {title}
        </Typography>
        <Box
          sx={{
            bgcolor: 'transparent',
            color: 'text.primary',
            px: 1.5,
            py: 0.5,
            borderRadius: '50px',
            fontSize: '0.75rem',
            fontWeight: 500,
            border: '1px solid',
            borderColor: 'grey.500',
            fontVariantNumeric: 'tabular-nums',
          }}
        >
          {headline}
        </Box>
      </Box>
      <Box sx={{ height: chartHeight, width: '100%' }}>
        {hasData ? (
          <ResponsiveContainer width="100%" height="100%">
            <AreaChart data={data} margin={{ top: 10, right: 12, left: 0, bottom: 0 }}>
              <defs>
                {series.map(s => (
                  <linearGradient
                    key={s.key}
                    id={`${gradientPrefix}-${s.key}`}
                    x1="0"
                    y1="0"
                    x2="0"
                    y2="1"
                  >
                    <stop offset="5%" stopColor={s.color} stopOpacity={0.8} />
                    <stop offset="95%" stopColor={s.color} stopOpacity={0.1} />
                  </linearGradient>
                ))}
              </defs>
              <CartesianGrid vertical={false} stroke="rgba(255,255,255,0.08)" />
              <XAxis
                dataKey="date"
                tickLine={false}
                axisLine={false}
                tickMargin={8}
                minTickGap={32}
                tick={{ fill: 'rgba(255,255,255,0.55)', fontSize: 12 }}
                tickFormatter={(v: string) =>
                  new Date(v).toLocaleDateString(undefined, {
                    month: 'short',
                    day: 'numeric',
                  })
                }
              />
              <YAxis
                tickLine={false}
                axisLine={false}
                tickMargin={8}
                width={48}
                tick={{ fill: 'rgba(255,255,255,0.55)', fontSize: 12 }}
                tickFormatter={valueFormatter}
              />
              <Tooltip
                cursor={{ stroke: 'rgba(255,255,255,0.2)' }}
                content={React.createElement(ShadcnTooltip(series, valueFormatter))}
              />
              {!hideLegend && (
                <Legend
                  verticalAlign="bottom"
                  height={24}
                  iconType="square"
                  iconSize={10}
                  wrapperStyle={{ fontSize: '0.72rem', color: 'rgba(255,255,255,0.75)' }}
                />
              )}
              {series.map(s => (
                <Area
                  key={s.key}
                  type="monotone"
                  dataKey={s.key}
                  name={s.label}
                  stackId={stacked ? 'stack' : undefined}
                  stroke={s.color}
                  strokeWidth={1.5}
                  fill={`url(#${gradientPrefix}-${s.key})`}
                  isAnimationActive={false}
                />
              ))}
            </AreaChart>
          </ResponsiveContainer>
        ) : (
          <Box
            sx={{
              height: '100%',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
            }}
          >
            <Typography variant="body2" color="text.secondary">
              No data
            </Typography>
          </Box>
        )}
      </Box>
    </Box>
  );
};

export default ShadcnAreaChart;
