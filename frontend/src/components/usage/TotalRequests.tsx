import React, { useMemo } from 'react';
import { Typography } from '@mui/material';
import { TypesUsersAggregatedUsageMetric, TypesAggregatedUsageMetric } from '../../api/api';
import ShadcnAreaChart, { ShadcnSeries } from './ShadcnAreaChart';

interface TotalRequestsProps {
  usageData: TypesUsersAggregatedUsageMetric[];
  isLoading: boolean;
}

const formatCompact = (n: number): string => {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}k`;
  return Math.round(n).toLocaleString();
};

const SERIES: ShadcnSeries[] = [
  { key: 'requests', label: 'Requests', color: '#8b5cf6' },
];

const TotalRequests: React.FC<TotalRequestsProps> = ({ usageData, isLoading }) => {
  const { data, totalValue } = useMemo(() => {
    if (!usageData || !Array.isArray(usageData) || usageData.length === 0) {
      return { data: [] as Array<{ date: string } & Record<string, number>>, totalValue: 0 };
    }
    const byDate = new Map<string, { date: string; requests: number }>();
    usageData.forEach((userData: TypesUsersAggregatedUsageMetric) => {
      userData.metrics?.forEach((m: TypesAggregatedUsageMetric) => {
        if (!m.date) return;
        const entry = byDate.get(m.date) ?? { date: m.date, requests: 0 };
        entry.requests += m.total_requests || 0;
        byDate.set(m.date, entry);
      });
    });
    const rows = Array.from(byDate.values()).sort((a, b) => a.date.localeCompare(b.date));
    const total = rows.reduce((sum, r) => sum + r.requests, 0);
    return { data: rows, totalValue: total };
  }, [usageData]);

  if (isLoading) {
    return <Typography variant="body1" textAlign="center">Loading usage data...</Typography>;
  }

  return (
    <ShadcnAreaChart
      title="TOTAL REQUESTS"
      headline={formatCompact(totalValue)}
      data={data}
      series={SERIES}
      valueFormatter={formatCompact}
      hideLegend
    />
  );
};

export default TotalRequests;
