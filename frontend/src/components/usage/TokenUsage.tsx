import React, { FC, useMemo } from 'react';
import { Typography } from '@mui/material';
import { TypesUsersAggregatedUsageMetric, TypesAggregatedUsageMetric } from '../../api/api';
import ShadcnAreaChart, { ShadcnSeries } from './ShadcnAreaChart';

interface TokenUsageProps {
  usageData: TypesUsersAggregatedUsageMetric[];
  isLoading: boolean;
}

const formatCompact = (n: number): string => {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}k`;
  return Math.round(n).toLocaleString();
};

// Stacked order (bottom to top): input → cache read → cache write → output.
const SERIES: ShadcnSeries[] = [
  { key: 'input', label: 'Input', color: '#3b82f6' },          // blue-500
  { key: 'cacheRead', label: 'Cache Read', color: '#22c55e' }, // green-500
  { key: 'cacheWrite', label: 'Cache Write', color: '#f59e0b' }, // amber-500
  { key: 'output', label: 'Output', color: '#a855f7' },        // purple-500
];

const TokenUsage: FC<TokenUsageProps> = ({ usageData, isLoading }) => {
  const { data, totalValue } = useMemo(() => {
    if (!usageData || !Array.isArray(usageData) || usageData.length === 0) {
      return { data: [] as Array<{ date: string } & Record<string, number>>, totalValue: 0 };
    }

    const byDate = new Map<string, { date: string; input: number; cacheRead: number; cacheWrite: number; output: number }>();
    usageData.forEach((userData: TypesUsersAggregatedUsageMetric) => {
      userData.metrics?.forEach((m: TypesAggregatedUsageMetric) => {
        if (!m.date) return;
        const entry = byDate.get(m.date) ?? { date: m.date, input: 0, cacheRead: 0, cacheWrite: 0, output: 0 };
        const cacheRead = m.cache_read_tokens || 0;
        const cacheWrite = m.cache_write_tokens || 0;
        // Non-cached prompt: prompt_tokens already includes cache, so subtract.
        entry.input += Math.max((m.prompt_tokens || 0) - cacheRead - cacheWrite, 0);
        entry.cacheRead += cacheRead;
        entry.cacheWrite += cacheWrite;
        entry.output += m.completion_tokens || 0;
        byDate.set(m.date, entry);
      });
    });
    const rows = Array.from(byDate.values()).sort((a, b) => a.date.localeCompare(b.date));
    const total = rows.reduce((sum, r) => sum + r.input + r.cacheRead + r.cacheWrite + r.output, 0);
    return { data: rows, totalValue: total };
  }, [usageData]);

  if (isLoading) {
    return <Typography variant="body1" textAlign="center">Loading usage data...</Typography>;
  }

  return (
    <ShadcnAreaChart
      title="TOTAL TOKENS"
      headline={formatCompact(totalValue)}
      data={data}
      series={SERIES}
      valueFormatter={formatCompact}
    />
  );
};

export default TokenUsage;
