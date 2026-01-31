import React, { FC, useMemo } from 'react';
import { Box, Typography, useTheme } from '@mui/material';
import { LineChart } from '@mui/x-charts';
import { TypesUsersAggregatedUsageMetric, TypesAggregatedUsageMetric } from '../../api/api';

interface TokenUsageProps {
  usageData: TypesUsersAggregatedUsageMetric[];
  isLoading: boolean;
}

const TokenUsage: FC<TokenUsageProps> = ({ usageData, isLoading }) => {
  const theme = useTheme();

  // Prepare data for the line chart - combined usage
  const prepareChartData = (usageData: TypesUsersAggregatedUsageMetric[]) => {
    if (isLoading) return { xAxis: [], series: [], totalValue: 0 };

    if (!usageData || !Array.isArray(usageData) || usageData.length === 0) return { xAxis: [], series: [], totalValue: 0 };

    // Get all unique dates across all users
    const allDates = new Set<string>();
    usageData.forEach((userData: TypesUsersAggregatedUsageMetric) => {
      userData.metrics?.forEach((metric: TypesAggregatedUsageMetric) => {
        if (metric.date) allDates.add(metric.date);
      });
    });

    // Sort dates
    const sortedDates = Array.from(allDates).sort();
    const xAxisDates = sortedDates.map(date => new Date(date));

    // Combine all users' data for each date
    const combinedData = sortedDates.map(date => {
      let totalTokens = 0;
      usageData.forEach((userData: TypesUsersAggregatedUsageMetric) => {
        const metric = userData.metrics?.find(m => m.date === date);
        if (metric) {
          totalTokens += metric.total_tokens || 0;
        }
      });
      return totalTokens;
    });

    const totalValue = combinedData.reduce((sum, value) => sum + value, 0);

    return {
      xAxis: xAxisDates,
      series: [{
        label: 'Total Tokens',
        data: combinedData,
      }],
      totalValue,
    };
  };

  const chartData = useMemo(() => prepareChartData(usageData), [usageData, isLoading]);

  if (isLoading) {
    return (
      <Typography variant="body1" textAlign="center">Loading usage data...</Typography>
    );
  }

  if (chartData.series.length === 0) {
    return (
      <Typography variant="body1" textAlign="center">No usage data available</Typography>
    );
  }

  return (
    <Box sx={{ 
      height: 300, 
      bgcolor: 'rgba(0, 0, 0, 0.2)', 
      borderRadius: 2, 
      p: 2,
      position: 'relative'
    }}>
      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 1 }}>
        <Typography variant="body2" sx={{ color: 'text.secondary', fontSize: '0.875rem' }}>
          TOTAL TOKENS
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
          borderColor: 'grey.500'
        }}>
          {chartData.totalValue?.toLocaleString() || '0'}
        </Box>
      </Box>
      <LineChart
        xAxis={[{
          data: chartData.xAxis,
          scaleType: 'time',
          valueFormatter: (value: number) => {
            const date = new Date(value);
            return date.toLocaleDateString('en-US', { day: 'numeric', month: 'short' });
          },
          labelStyle: {
            angle: 0,
            textAnchor: 'middle',
            fontSize: '0.75rem',
            fill: '#9ca3af'
          },
          tickMinStep: 24 * 60 * 60 * 1000,
          min: chartData.xAxis.length > 0 ? chartData.xAxis[0].getTime() : undefined,
          max: chartData.xAxis.length > 0 ? chartData.xAxis[chartData.xAxis.length - 1].getTime() : undefined,
          hideTooltip: true
        }]}
        yAxis={[{                            
          valueFormatter: (value: number) => {
            if (value >= 1000000) {
              return `${(value / 1000000).toFixed(0)}M`;
            } else if (value >= 1000) {
              return `${(value / 1000).toFixed(0)}k`;
            }
            return value.toString();
          },
          labelStyle: {
            fontSize: '0.75rem',
            fill: '#9ca3af'
          }
        }]}
        margin={{ left: 50, right: 20, top: 10, bottom: 30 }}
        series={chartData.series.map(series => ({
          ...series,
          showMarkers: false,
          area: true,
          lineStyle: { 
            marker: { display: 'none' },
            stroke: '#8b5cf6',
            strokeWidth: 2
          },
          areaStyle: {
            fill: 'url(#tokensGradient)',
            opacity: 0.3
          }
        }))}
        height={220}
        slotProps={{
          legend: {
            hidden: true
          }
        }}
        sx={{
          '& .MuiAreaElement-root': {
            fill: 'url(#tokensGradient)',
          },
          '& .MuiMarkElement-root': {
            display: 'none',
          },
        }}
      >
        <defs>
          <linearGradient id="tokensGradient" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor={theme.chartGradientStart} stopOpacity={theme.chartGradientStartOpacity} />
            <stop offset="100%" stopColor={theme.chartGradientEnd} stopOpacity={theme.chartGradientEndOpacity} />
          </linearGradient>
        </defs>
      </LineChart>
    </Box>
  );
};

export default TokenUsage;
