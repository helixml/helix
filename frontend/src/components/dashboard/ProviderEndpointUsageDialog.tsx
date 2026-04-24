import React, { useState, useEffect, useMemo } from 'react';
import {
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  Box,
  Typography,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Paper,
  CircularProgress,
  TableSortLabel,
} from '@mui/material';
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
import { IProviderEndpoint } from '../../types';
import { useApi } from '../../hooks/useApi';
import { TypesUsersAggregatedUsageMetric } from '../../api/api';

interface ProviderEndpointUsageDialogProps {
  open: boolean;
  onClose: () => void;
  endpoint: IProviderEndpoint | null;
}

// shadcn/ui-inspired stacked area chart palette.
const SERIES = [
  { key: 'prompt', label: 'Input', color: '#3b82f6' },       // blue-500
  { key: 'cacheRead', label: 'Cache Read', color: '#22c55e' },   // green-500
  { key: 'cacheWrite', label: 'Cache Write', color: '#f59e0b' }, // amber-500
  { key: 'completion', label: 'Output', color: '#a855f7' },  // purple-500
] as const;

const formatCompact = (n: number) => {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}k`;
  return n.toLocaleString();
};

const ShadcnTooltip: React.FC<TooltipProps<number, string>> = ({ active, payload, label }) => {
  if (!active || !payload || !payload.length) return null;
  const date = label ? new Date(label as string) : null;
  const dateLabel = date ? date.toLocaleDateString(undefined, { month: 'short', day: 'numeric' }) : '';
  // Recharts sends payload in stack order; we want descriptive order from SERIES.
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
      }}
    >
      <Typography variant="caption" sx={{ color: 'rgba(255,255,255,0.6)', display: 'block', mb: 0.5 }}>
        {dateLabel}
      </Typography>
      {SERIES.map(s => {
        const entry = byKey.get(s.key);
        const value = typeof entry?.value === 'number' ? entry.value : 0;
        if (!value) return null;
        return (
          <Box key={s.key} sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 0.25 }}>
            <Box sx={{ width: 8, height: 8, borderRadius: '2px', bgcolor: s.color }} />
            <Typography variant="caption" sx={{ color: 'rgba(255,255,255,0.85)', flex: 1 }}>
              {s.label}
            </Typography>
            <Typography variant="caption" sx={{ color: '#fff', fontVariantNumeric: 'tabular-nums', fontWeight: 500 }}>
              {formatCompact(value)}
            </Typography>
          </Box>
        );
      })}
    </Box>
  );
};

interface ChartDatum {
  date: string;
  prompt: number;
  completion: number;
  cacheRead: number;
  cacheWrite: number;
}

const ShadcnUsageAreaChart: React.FC<{ data: ChartDatum[] }> = ({ data }) => (
  <ResponsiveContainer width="100%" height="100%">
    <AreaChart data={data} margin={{ top: 10, right: 12, left: 0, bottom: 0 }}>
      <defs>
        {SERIES.map(s => (
          <linearGradient key={s.key} id={`fill-${s.key}`} x1="0" y1="0" x2="0" y2="1">
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
        tick={{ fill: 'rgba(255,255,255,0.6)', fontSize: 12 }}
        tickFormatter={(v: string) => new Date(v).toLocaleDateString(undefined, { month: 'short', day: 'numeric' })}
      />
      <YAxis
        tickLine={false}
        axisLine={false}
        tickMargin={8}
        tick={{ fill: 'rgba(255,255,255,0.6)', fontSize: 12 }}
        tickFormatter={formatCompact}
        width={48}
      />
      <Tooltip content={<ShadcnTooltip />} cursor={{ stroke: 'rgba(255,255,255,0.2)' }} />
      <Legend
        verticalAlign="bottom"
        height={28}
        iconType="square"
        wrapperStyle={{ fontSize: '0.75rem', color: 'rgba(255,255,255,0.75)' }}
      />
      {SERIES.map(s => (
        <Area
          key={s.key}
          type="monotone"
          dataKey={s.key}
          name={s.label}
          stackId="tokens"
          stroke={s.color}
          strokeWidth={1.5}
          fill={`url(#fill-${s.key})`}
          isAnimationActive={false}
        />
      ))}
    </AreaChart>
  </ResponsiveContainer>
);

const ProviderEndpointUsageDialog: React.FC<ProviderEndpointUsageDialogProps> = ({
  open,
  onClose,
  endpoint,
}) => {
  const [loading, setLoading] = useState(true);
  const [usageData, setUsageData] = useState<TypesUsersAggregatedUsageMetric[]>([]);
  const [orderBy, setOrderBy] = useState<'username' | 'email' | 'promptTokens' | 'completionTokens' | 'cacheReadTokens' | 'cacheWriteTokens' | 'totalTokens'>('totalTokens');
  const [order, setOrder] = useState<'asc' | 'desc'>('desc');
  const api = useApi();

  useEffect(() => {
    const fetchData = async () => {
      if (!endpoint || !open) return;
      
      setLoading(true);
      try {
        const response = await api.getApiClient().v1ProviderEndpointsUsersDailyUsageDetail(
          endpoint.id && endpoint.id !== "-" ? endpoint.id : endpoint.name
        );
        setUsageData(response.data);
      } catch (error) {
        console.error('Failed to fetch usage data:', error);
      } finally {
        setLoading(false);
      }
    };

    fetchData();
  }, [endpoint, open]);

  // Calculate per-user totals over the window
  const userTotals = usageData.map(userData => ({
    user: userData.user,
    promptTokens: (userData.metrics || []).reduce((sum, metric) => sum + (metric.prompt_tokens || 0), 0),
    completionTokens: (userData.metrics || []).reduce((sum, metric) => sum + (metric.completion_tokens || 0), 0),
    cacheReadTokens: (userData.metrics || []).reduce((sum, metric) => sum + (metric.cache_read_tokens || 0), 0),
    cacheWriteTokens: (userData.metrics || []).reduce((sum, metric) => sum + (metric.cache_write_tokens || 0), 0),
    totalTokens: (userData.metrics || []).reduce((sum, metric) => sum + (metric.total_tokens || 0), 0)
  })).sort((a, b) => b.totalTokens - a.totalTokens);

  // Aggregate usage across all users per date for the stacked area chart.
  // Prompt is the non-cached portion so the stack sums to the provider's reported prompt+completion+cache.
  const chartData = useMemo(() => {
    const byDate = new Map<string, { date: string; prompt: number; completion: number; cacheRead: number; cacheWrite: number }>();
    for (const user of usageData) {
      for (const m of user.metrics || []) {
        if (!m.date) continue;
        const key = m.date;
        const entry = byDate.get(key) ?? { date: key, prompt: 0, completion: 0, cacheRead: 0, cacheWrite: 0 };
        const cacheRead = m.cache_read_tokens || 0;
        const cacheWrite = m.cache_write_tokens || 0;
        entry.prompt += Math.max((m.prompt_tokens || 0) - cacheRead - cacheWrite, 0);
        entry.completion += m.completion_tokens || 0;
        entry.cacheRead += cacheRead;
        entry.cacheWrite += cacheWrite;
        byDate.set(key, entry);
      }
    }
    return Array.from(byDate.values()).sort((a, b) => a.date.localeCompare(b.date));
  }, [usageData]);

  const chartHasData = chartData.some(d => d.prompt + d.completion + d.cacheRead + d.cacheWrite > 0);

  const handleSort = (property: typeof orderBy) => {
    const isAsc = orderBy === property && order === 'asc';
    setOrder(isAsc ? 'desc' : 'asc');
    setOrderBy(property);
  };

  const sortedUserTotals = [...userTotals].sort((a, b) => {
    const multiplier = order === 'asc' ? 1 : -1;
    
    switch (orderBy) {
      case 'username':
        return multiplier * ((a.user?.username || '').localeCompare(b.user?.username || ''));
      case 'email':
        return multiplier * ((a.user?.email || '').localeCompare(b.user?.email || ''));
      case 'promptTokens':
        return multiplier * (a.promptTokens - b.promptTokens);
      case 'completionTokens':
        return multiplier * (a.completionTokens - b.completionTokens);
      case 'cacheReadTokens':
        return multiplier * (a.cacheReadTokens - b.cacheReadTokens);
      case 'cacheWriteTokens':
        return multiplier * (a.cacheWriteTokens - b.cacheWriteTokens);
      case 'totalTokens':
        return multiplier * (a.totalTokens - b.totalTokens);
      default:
        return 0;
    }
  });

  return (
    <Dialog
      open={open}
      onClose={onClose}
      maxWidth="md"
      fullWidth
      PaperProps={{
        sx: {
          bgcolor: '#000000',
          backdropFilter: 'blur(10px)',
          border: '1px solid rgba(255, 255, 255, 0.1)',
          '& .MuiDialogTitle-root': {
            color: 'white',
            borderBottom: '1px solid rgba(255, 255, 255, 0.1)',
            padding: '16px 24px'
          },
          '& .MuiDialogContent-root': {
            color: 'white'
          },
          '& .MuiDialogActions-root': {
            borderTop: '1px solid rgba(255, 255, 255, 0.1)',
            padding: '16px 24px'
          }
        }
      }}
    >
      <DialogTitle>
        Usage Statistics: {endpoint?.name}
      </DialogTitle>
      <DialogContent>
        {loading ? (
          <Box display="flex" justifyContent="center" alignItems="center" minHeight={400}>
            <CircularProgress />
          </Box>
        ) : (
          <Box>
            <Box mb={4} sx={{ height: 300, width: '100%' }}>
              {chartHasData ? (
                <ShadcnUsageAreaChart data={chartData} />
              ) : (
                <Typography variant="body1" textAlign="center">No usage data available</Typography>
              )}
            </Box>

            <TableContainer
              component={Paper}
              sx={{
                bgcolor: 'rgba(10, 10, 15, 0.6)',
                border: '1px solid rgba(255, 255, 255, 0.08)',
                borderRadius: 2,
                backgroundImage: 'none',
                '& .MuiTableCell-root': {
                  color: 'rgba(255, 255, 255, 0.85)',
                  borderBottomColor: 'rgba(255, 255, 255, 0.08)',
                  fontVariantNumeric: 'tabular-nums',
                },
                '& .MuiTableCell-head': {
                  bgcolor: 'rgba(255, 255, 255, 0.03)',
                  color: 'rgba(255, 255, 255, 0.6)',
                  fontWeight: 500,
                  fontSize: '0.75rem',
                  textTransform: 'uppercase',
                  letterSpacing: '0.04em',
                },
                '& .MuiTableSortLabel-root, & .MuiTableSortLabel-root:hover, & .MuiTableSortLabel-root.Mui-active': {
                  color: 'inherit',
                },
                '& .MuiTableSortLabel-icon': {
                  color: 'inherit !important',
                },
                '& .MuiTableRow-root:hover .MuiTableCell-root': {
                  bgcolor: 'rgba(255, 255, 255, 0.03)',
                },
              }}
            >
              <Table>
                <TableHead>
                  <TableRow>
                    <TableCell>
                      <TableSortLabel
                        active={orderBy === 'username'}
                        direction={orderBy === 'username' ? order : 'asc'}
                        onClick={() => handleSort('username')}
                      >
                        User
                      </TableSortLabel>
                    </TableCell>
                    <TableCell>
                      <TableSortLabel
                        active={orderBy === 'email'}
                        direction={orderBy === 'email' ? order : 'asc'}
                        onClick={() => handleSort('email')}
                      >
                        Email
                      </TableSortLabel>
                    </TableCell>
                    <TableCell align="right">
                      <TableSortLabel
                        active={orderBy === 'promptTokens'}
                        direction={orderBy === 'promptTokens' ? order : 'asc'}
                        onClick={() => handleSort('promptTokens')}
                      >
                        Prompt Tokens
                      </TableSortLabel>
                    </TableCell>
                    <TableCell align="right">
                      <TableSortLabel
                        active={orderBy === 'completionTokens'}
                        direction={orderBy === 'completionTokens' ? order : 'asc'}
                        onClick={() => handleSort('completionTokens')}
                      >
                        Completion Tokens
                      </TableSortLabel>
                    </TableCell>
                    <TableCell align="right">
                      <TableSortLabel
                        active={orderBy === 'cacheReadTokens'}
                        direction={orderBy === 'cacheReadTokens' ? order : 'asc'}
                        onClick={() => handleSort('cacheReadTokens')}
                      >
                        Cache Read
                      </TableSortLabel>
                    </TableCell>
                    <TableCell align="right">
                      <TableSortLabel
                        active={orderBy === 'cacheWriteTokens'}
                        direction={orderBy === 'cacheWriteTokens' ? order : 'asc'}
                        onClick={() => handleSort('cacheWriteTokens')}
                      >
                        Cache Write
                      </TableSortLabel>
                    </TableCell>
                    <TableCell align="right">
                      <TableSortLabel
                        active={orderBy === 'totalTokens'}
                        direction={orderBy === 'totalTokens' ? order : 'asc'}
                        onClick={() => handleSort('totalTokens')}
                      >
                        Total Tokens
                      </TableSortLabel>
                    </TableCell>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {sortedUserTotals.map(({ user, promptTokens, completionTokens, cacheReadTokens, cacheWriteTokens, totalTokens }) => (
                    <TableRow key={user?.id || 'unknown'}>
                      <TableCell>{user?.username || 'Unknown User'}</TableCell>
                      <TableCell>{user?.email || 'N/A'}</TableCell>
                      <TableCell align="right">{promptTokens.toLocaleString()}</TableCell>
                      <TableCell align="right">{completionTokens.toLocaleString()}</TableCell>
                      <TableCell align="right">{cacheReadTokens ? cacheReadTokens.toLocaleString() : '-'}</TableCell>
                      <TableCell align="right">{cacheWriteTokens ? cacheWriteTokens.toLocaleString() : '-'}</TableCell>
                      <TableCell align="right">{totalTokens.toLocaleString()}</TableCell>
                    </TableRow>
                  ))}
                  {userTotals.length > 1 && (
                    <TableRow>
                      <TableCell colSpan={2} sx={{ fontWeight: 'bold' }}>Total</TableCell>
                      <TableCell align="right" sx={{ fontWeight: 'bold' }}>
                        {userTotals.reduce((sum, user) => sum + user.promptTokens, 0).toLocaleString()}
                      </TableCell>
                      <TableCell align="right" sx={{ fontWeight: 'bold' }}>
                        {userTotals.reduce((sum, user) => sum + user.completionTokens, 0).toLocaleString()}
                      </TableCell>
                      <TableCell align="right" sx={{ fontWeight: 'bold' }}>
                        {userTotals.reduce((sum, user) => sum + user.cacheReadTokens, 0).toLocaleString()}
                      </TableCell>
                      <TableCell align="right" sx={{ fontWeight: 'bold' }}>
                        {userTotals.reduce((sum, user) => sum + user.cacheWriteTokens, 0).toLocaleString()}
                      </TableCell>
                      <TableCell align="right" sx={{ fontWeight: 'bold' }}>
                        {userTotals.reduce((sum, user) => sum + user.totalTokens, 0).toLocaleString()}
                      </TableCell>
                    </TableRow>
                  )}
                </TableBody>
              </Table>
            </TableContainer>
          </Box>
        )}
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>Close</Button>
      </DialogActions>
    </Dialog>
  );
};

export default ProviderEndpointUsageDialog; 