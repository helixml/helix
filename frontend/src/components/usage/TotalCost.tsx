import React, { FC, useMemo } from 'react';
import { Typography } from '@mui/material';
import { TypesUsersAggregatedUsageMetric, TypesAggregatedUsageMetric } from '../../api/api';
import ShadcnAreaChart, { ShadcnSeries } from './ShadcnAreaChart';

interface TotalCostProps {
  usageData: TypesUsersAggregatedUsageMetric[];
  isLoading: boolean;
}

const formatCurrency = (n: number): string => {
  if (n >= 1000) return `$${(n / 1000).toFixed(2)}k`;
  if (n >= 1) return `$${n.toFixed(2)}`;
  return `$${n.toFixed(4)}`;
};

// Stacked: input + cache read + cache write + output.
const SERIES: ShadcnSeries[] = [
  { key: 'input', label: 'Input', color: '#3b82f6' },
  { key: 'cacheRead', label: 'Cache Read', color: '#22c55e' },
  { key: 'cacheWrite', label: 'Cache Write', color: '#f59e0b' },
  { key: 'output', label: 'Output', color: '#a855f7' },
  { key: 'sandbox', label: 'Sandboxes', color: '#ef4444' },
];

const TotalCost: FC<TotalCostProps> = ({ usageData, isLoading }) => {
  const { data, totalValue } = useMemo(() => {
    if (!usageData || !Array.isArray(usageData) || usageData.length === 0) {
      return { data: [] as Array<{ date: string } & Record<string, number>>, totalValue: 0 };
    }
    const byDate = new Map<string, { date: string; input: number; cacheRead: number; cacheWrite: number; output: number; sandbox: number }>();
    usageData.forEach((userData: TypesUsersAggregatedUsageMetric) => {
      userData.metrics?.forEach((m: TypesAggregatedUsageMetric) => {
        if (!m.date) return;
        const entry = byDate.get(m.date) ?? { date: m.date, input: 0, cacheRead: 0, cacheWrite: 0, output: 0, sandbox: 0 };
        entry.input += m.prompt_cost || 0;
        entry.cacheRead += m.cache_read_cost || 0;
        entry.cacheWrite += m.cache_write_cost || 0;
        entry.output += m.completion_cost || 0;
        entry.sandbox += m.sandbox_cost || 0;
        byDate.set(m.date, entry);
      });
    });
    const rows = Array.from(byDate.values()).sort((a, b) => a.date.localeCompare(b.date));
    const total = rows.reduce(
      (sum, r) => sum + r.input + r.cacheRead + r.cacheWrite + r.output + r.sandbox,
      0,
    );
    return { data: rows, totalValue: total };
  }, [usageData]);

  if (isLoading) {
    return <Typography variant="body1" textAlign="center">Loading usage data...</Typography>;
  }

  return (
    <ShadcnAreaChart
      title="TOTAL SPEND"
      headline={formatCurrency(totalValue)}
      data={data}
      series={SERIES}
      valueFormatter={formatCurrency}
    />
  );
};

export default TotalCost;
