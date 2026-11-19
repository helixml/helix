import React, { FC } from 'react';
import Box from '@mui/material/Box';
import Typography from '@mui/material/Typography';
import {
  Area,
  AreaChart,
  CartesianGrid,
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts';
import type { TooltipContentProps } from 'recharts';

import useLightTheme from '../../hooks/useLightTheme';

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
  /** Control rendered in the header beside the headline, e.g. a metric toggle. */
  action?: React.ReactNode;
  /** Chart data rows; `date` is ISO string, other keys are series values. */
  data: Array<{ date: string; [key: string]: number | string }>;
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
  /** Height of the whole card. Default 300 (the usage page's grid); 'auto' hugs the chart. */
  cardHeight?: number | 'auto';
  /** Treat an explicit zero as data instead of an empty series. */
  zeroIsData?: boolean;
  /**
   * 'area' stacks filled areas; 'line' draws unfilled comparison lines;
   * 'trend' draws each series as its own smooth line over a faint fill.
   */
  variant?: 'area' | 'line' | 'trend';
  /** Fixed Y-axis bounds for values with a known range. */
  yDomain?: [number, number];
  /** Width reserved for Y-axis tick labels. Default 72. */
  yAxisWidth?: number;
  /** Include the time of day in X-axis labels and tooltips. */
  showTime?: boolean;
  /**
   * Bridge gaps in a line series instead of breaking it. Use when a missing
   * point means "no sample that day" rather than "measured zero" — bridging
   * keeps a sparsely-sampled series readable as a trend.
   */
  connectNulls?: boolean;
}

const uid = () => Math.random().toString(36).slice(2, 9);

// niceStep rounds a raw tick interval up to 1, 2, 2.5 or 5 × 10ⁿ, so axes read
// 0 / 10M / 20M / 30M rather than 0 / 7.5M / 15M / 22M.
export const niceStep = (raw: number): number => {
  if (!(raw > 0)) return 1;
  const magnitude = 10 ** Math.floor(Math.log10(raw));
  const f = raw / magnitude;
  const nice = f <= 1 ? 1 : f <= 2 ? 2 : f <= 2.5 ? 2.5 : f <= 5 ? 5 : 10;
  return nice * magnitude;
};

// plottedSeries is what a chart draws and its legend lists: series with a
// value in range. An explicit zero counts when zero is data, except on a
// trend, where an all-zero series would be a flat line along the axis.
export const plottedSeries = (
  series: ShadcnSeries[],
  data: Array<Record<string, unknown>>,
  variant: ShadcnAreaChartProps['variant'],
  zeroIsData: boolean,
): ShadcnSeries[] => series.filter(s => data.some(d => (
  (Number(d[s.key]) || 0) > 0 || (zeroIsData && variant !== 'trend' && d[s.key] != null)
)));

// niceTicks spans [0, max] in at most ~5 round ticks.
export const niceTicks = (max: number): number[] => {
  const step = niceStep(max / 4);
  const top = Math.max(step, Math.ceil(max / step - 1e-9) * step);
  const ticks: number[] = [];
  for (let t = 0; t <= top + step / 2; t += step) ticks.push(Number(t.toPrecision(12)));
  return ticks;
};

const ShadcnTooltip = (
  seriesConfig: ShadcnSeries[],
  valueFormatter: (v: number) => string,
  zeroIsData: boolean,
  showTime: boolean,
  showTotal: boolean,
) => {
  const TooltipComponent: FC<TooltipContentProps<number, string>> = ({ active, payload, label }) => {
    const lightTheme = useLightTheme();
    if (!active || !payload || !payload.length) return null;
    const date = label ? new Date(label as string) : null;
    const dateLabel = date
      ? date.toLocaleString(undefined, showTime
        ? { month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit' }
        : { month: 'short', day: 'numeric' })
      : '';
    const byKey = new Map(payload.map(p => [p.dataKey as string, p] as const));
    return (
      <Box
        sx={{
          bgcolor: lightTheme.isLight ? 'rgba(255, 255, 255, 0.98)' : 'rgba(10, 10, 15, 0.95)',
          border: lightTheme.isLight ? '1px solid rgba(0, 0, 0, 0.12)' : '1px solid rgba(255, 255, 255, 0.12)',
          borderRadius: 2,
          px: 1.5,
          py: 1,
          fontSize: '0.8rem',
          boxShadow: lightTheme.isLight ? '0 4px 12px rgba(0,0,0,0.12)' : '0 4px 12px rgba(0,0,0,0.4)',
          minWidth: 140,
        }}
      >
        <Typography
          variant="caption"
          sx={{ color: lightTheme.isLight ? 'rgba(0,0,0,0.6)' : 'rgba(255,255,255,0.6)', display: 'block', mb: 0.5 }}
        >
          {dateLabel}
        </Typography>
        {seriesConfig.map(s => {
          const entry = byKey.get(s.key);
          if (!entry || typeof entry.value !== 'number') return null;
          const value = entry.value;
          if (!zeroIsData && !value) return null;
          return (
            <Box
              key={s.key}
              sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 0.25 }}
            >
              <Box sx={{ width: 8, height: 8, borderRadius: '2px', bgcolor: s.color }} />
              <Typography variant="caption" sx={{ color: lightTheme.isLight ? 'rgba(0,0,0,0.85)' : 'rgba(255,255,255,0.85)', flex: 1 }}>
                {s.label}
              </Typography>
              <Typography
                variant="caption"
                sx={{
                  color: lightTheme.textColor,
                  fontVariantNumeric: 'tabular-nums',
                  fontWeight: 500,
                }}
              >
                {valueFormatter(value)}
              </Typography>
            </Box>
          );
        })}
        {showTotal && (() => {
          const values = seriesConfig.map(s => byKey.get(s.key)?.value).filter((v): v is number => typeof v === 'number' && v > 0);
          if (values.length < 2) return null;
          return (
            <Box sx={{ display: 'flex', gap: 1, mt: 0.5, pt: 0.5, borderTop: '1px solid', borderColor: lightTheme.isLight ? 'rgba(0,0,0,0.08)' : 'rgba(255,255,255,0.1)' }}>
              <Typography variant="caption" sx={{ flex: 1, pl: 2, color: lightTheme.isLight ? 'rgba(0,0,0,0.6)' : 'rgba(255,255,255,0.6)' }}>Total</Typography>
              <Typography variant="caption" sx={{ color: lightTheme.textColor, fontVariantNumeric: 'tabular-nums', fontWeight: 600 }}>
                {valueFormatter(values.reduce((a, b) => a + b, 0))}
              </Typography>
            </Box>
          );
        })()}
      </Box>
    );
  };
  return TooltipComponent;
};

const ShadcnAreaChart: FC<ShadcnAreaChartProps> = ({
  title,
  headline,
  action,
  data,
  series,
  valueFormatter,
  stacked = true,
  hideLegend = false,
  chartHeight = 220,
  cardHeight = 300,
  zeroIsData = false,
  variant = 'area',
  yDomain,
  yAxisWidth = 72,
  showTime = false,
  connectNulls = false,
}) => {
  const lightTheme = useLightTheme();
  const Chart = variant === 'line' ? LineChart : AreaChart;
  // Trend lines are drawn side by side, so the axis scales to the largest
  // single series, not their sum.
  const stackSeries = stacked && variant !== 'line' && variant !== 'trend';
  const drawn = React.useMemo(() => plottedSeries(series, data, variant, zeroIsData), [series, data, variant, zeroIsData]);
  // Few points (sparse days, a provider seen twice) draw as straight segments
  // between real samples, since smoothing invents shape between dates. A
  // trend is smooth by design and always curves.
  const curve = data.length <= 14 && variant !== 'trend' ? 'linear' : 'monotone';
  const yTicks = React.useMemo(() => {
    if (yDomain) return undefined;
    const max = Math.max(0, ...data.map(d => (stackSeries
      ? series.reduce((acc, s) => acc + (Number(d[s.key]) || 0), 0)
      : Math.max(0, ...series.map(s => Number(d[s.key]) || 0)))));
    // A little headroom so a peak that lands on a tick doesn't ride the top edge.
    return niceTicks(max * 1.08);
  }, [data, series, stackSeries, yDomain]);
  const dense = data.length > 40;
  // A chart within one calendar day labels its ticks with the time alone; the
  // tooltip keeps the date.
  const oneDay = React.useMemo(() => {
    if (!showTime || data.length < 2) return false;
    return new Date(data[0].date).toDateString() === new Date(data[data.length - 1].date).toDateString();
  }, [data, showTime]);
  // Unique gradient ids so multiple charts on the same page don't collide.
  const gradientPrefix = React.useMemo(() => `shadcn-${uid()}`, []);

  const hasData = data.some(d => series.some(s => (
    Object.prototype.hasOwnProperty.call(d, s.key) && (zeroIsData || (Number(d[s.key]) || 0) > 0)
  )));

  return (
    <Box
      sx={{
        height: cardHeight,
        bgcolor: 'background.paper',
        border: '1px solid',
        borderColor: 'divider',
        borderRadius: 2,
        p: 2,
        position: 'relative',
        minWidth: 0,
      }}
    >
      <Box sx={{ display: 'flex', flexWrap: 'wrap', justifyContent: 'space-between', alignItems: 'center', columnGap: 1, rowGap: 1, mb: 1, minHeight: 30 }}>
        <Box sx={{ minWidth: 0 }}>
          <Typography
            variant="body2"
            noWrap
            sx={{ color: 'text.primary', fontSize: '0.875rem', fontWeight: 600 }}
          >
            {title}
          </Typography>
          <Typography
            variant="caption"
            noWrap
            sx={{ display: 'block', color: 'text.secondary', fontVariantNumeric: 'tabular-nums' }}
          >
            {headline}
          </Typography>
        </Box>
        {action && <Box sx={{ display: 'flex', flexWrap: 'wrap', columnGap: 2, rowGap: 1 }}>{action}</Box>}
      </Box>
      {/* Phones get shorter plots: most usage charts hold a handful of points. */}
      <Box sx={{ height: { xs: Math.min(chartHeight, 170), sm: chartHeight }, width: '100%' }}>
        {hasData ? (
          <ResponsiveContainer width="100%" height="100%">
            <Chart data={data} margin={{ top: 10, right: 12, left: 16, bottom: 0 }}>
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
                    <stop offset="5%" stopColor={s.color} stopOpacity={variant === 'trend' ? 0.22 : 0.8} />
                    <stop offset="95%" stopColor={s.color} stopOpacity={variant === 'trend' ? 0 : 0.1} />
                  </linearGradient>
                ))}
              </defs>
              <CartesianGrid vertical={false} stroke={lightTheme.isLight ? 'rgba(15,23,42,0.08)' : 'rgba(255,255,255,0.08)'} />
              <XAxis
                dataKey="date"
                tickLine={false}
                axisLine={false}
                tickMargin={8}
                minTickGap={32}
                tick={{ fill: lightTheme.isLight ? '#475569' : '#94A3B8', fontSize: 12 }}
                tickFormatter={(v: string) => new Date(v).toLocaleString(undefined, !showTime
                  ? { month: 'short', day: 'numeric' }
                  : oneDay
                    ? { hour: 'numeric', minute: '2-digit' }
                    : { month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit' })}
              />
              <YAxis
                tickLine={false}
                axisLine={false}
                tickMargin={10}
                width={yAxisWidth}
                tick={{ fill: lightTheme.isLight ? '#475569' : '#94A3B8', fontSize: 12 }}
                tickFormatter={valueFormatter}
                domain={yDomain ?? [0, yTicks?.[yTicks.length - 1] ?? 'auto']}
                ticks={yTicks}
                // A caller-fixed range (0–100%, 0–1) is the scale; never widen it for stray data.
                allowDataOverflow={!!yDomain}
              />
              <Tooltip
                cursor={{ stroke: lightTheme.isLight ? 'rgba(15,23,42,0.25)' : 'rgba(255,255,255,0.2)' }}
                content={React.createElement(ShadcnTooltip(series, valueFormatter, zeroIsData, showTime, stacked && variant !== 'line'))}
              />
              {/* Recharts' default legend lays items at whatever order they
                  enter the chart and wraps awkwardly when there are many
                  series — items end up in two ragged rows. We render our own
                  legend below the chart instead (see <ChartLegend/> below). */}
              {drawn.map(s => variant === 'line' ? (
                <Line
                  key={s.key}
                  type={curve}
                  dataKey={s.key}
                  name={s.label}
                  stroke={s.color}
                  // Dense series draw as a thin trace; dots only where samples are sparse.
                  strokeWidth={dense ? 1.5 : 2}
                  dot={dense ? false : { r: 2.5, fill: s.color, strokeWidth: 0 }}
                  activeDot={{ r: 4, fill: s.color, strokeWidth: 0 }}
                  connectNulls={connectNulls}
                  isAnimationActive={false}
                />
              ) : (
                <Area
                  key={s.key}
                  type={curve}
                  dataKey={s.key}
                  name={s.label}
                  stackId={stackSeries ? 'stack' : undefined}
                  stroke={s.color}
                  strokeWidth={variant === 'trend' ? 2 : 1.5}
                  fill={`url(#${gradientPrefix}-${s.key})`}
                  // A one- or two-sample trend has no line to speak of; mark the samples.
                  {...(variant === 'trend' ? {
                    dot: data.length <= 2 ? { r: 3, fill: s.color, strokeWidth: 0 } : false,
                    activeDot: { r: 4, fill: s.color, strokeWidth: 0 },
                  } : {})}
                  isAnimationActive={false}
                />
              ))}
            </Chart>
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
      {!hideLegend && hasData && (
        // Tight, single-line legend. Recharts' default Legend wrapped 5-item
        // legends to two rows with one orphaned item ("Sandboxes" alone on
        // line 2), which looked broken. We render our own with small swatches
        // and a compact font so 5 items fit on one row at typical card widths;
        // when the parent really is too narrow, items wrap evenly rather than
        // dropping a single trailing label below.
        <Box
          sx={{
            display: 'flex',
            flexWrap: 'wrap',
            justifyContent: 'center',
            alignItems: 'center',
            columnGap: 1.25,
            rowGap: 0.5,
            mt: 1,
            px: 0.5,
          }}
        >
          {/* The legend lists what is drawn: an entry with nothing plotted reads as a missing line. */}
          {drawn.map(s => (
            <Box
              key={s.key}
              sx={{ display: 'flex', alignItems: 'center', gap: 0.5, flex: '0 0 auto' }}
            >
              <Box
                sx={{
                  width: 8,
                  height: 8,
                  borderRadius: '2px',
                  bgcolor: s.color,
                  flexShrink: 0,
                }}
              />
              <Typography
                variant="caption"
                title={s.label}
                sx={{ color: lightTheme.isLight ? '#334155' : '#cbd5e1', fontSize: '0.75rem', whiteSpace: 'nowrap', lineHeight: 1.4, maxWidth: 180, overflow: 'hidden', textOverflow: 'ellipsis' }}
              >
                {s.label}
              </Typography>
            </Box>
          ))}
        </Box>
      )}
    </Box>
  );
};

export default ShadcnAreaChart;
