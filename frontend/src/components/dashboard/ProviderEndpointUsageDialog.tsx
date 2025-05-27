import React, { useState, useEffect } from 'react';
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
import { LineChart } from '@mui/x-charts';
import { IProviderEndpoint } from '../../types';
import { useApi } from '../../hooks/useApi';
import { TypesUsersAggregatedUsageMetric } from '../../api/api';

interface ProviderEndpointUsageDialogProps {
  open: boolean;
  onClose: () => void;
  endpoint: IProviderEndpoint | null;
}

const ProviderEndpointUsageDialog: React.FC<ProviderEndpointUsageDialogProps> = ({
  open,
  onClose,
  endpoint,
}) => {
  const [loading, setLoading] = useState(true);
  const [usageData, setUsageData] = useState<TypesUsersAggregatedUsageMetric[]>([]);
  const [orderBy, setOrderBy] = useState<'username' | 'email' | 'promptTokens' | 'completionTokens' | 'totalTokens'>('totalTokens');
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

  // Calculate total tokens for each user over the last 7 days
  const userTotals = usageData.map(userData => ({
    user: userData.user,
    promptTokens: (userData.metrics || []).reduce((sum, metric) => sum + (metric.prompt_tokens || 0), 0),
    completionTokens: (userData.metrics || []).reduce((sum, metric) => sum + (metric.completion_tokens || 0), 0),
    totalTokens: (userData.metrics || []).reduce((sum, metric) => sum + (metric.total_tokens || 0), 0)
  })).sort((a, b) => b.totalTokens - a.totalTokens);

  // Get the first user's metrics dates for X axis (assuming all users have same dates)
  const firstUserMetrics = usageData[0]?.metrics || [];
  const xAxisDates = firstUserMetrics.map(m => new Date(m.date || ''));

  // Prepare data for the line chart
  const chartData = {
    xAxis: xAxisDates,
    series: usageData.map(userData => ({
      data: (userData.metrics || []).map(m => m.total_tokens || 0),
      label: userData.user?.username || 'Unknown User',
    }))
  };

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
            <Box mb={4} height={300}>
              {usageData.length > 0 ? (
                <LineChart
                  xAxis={[{
                    data: chartData.xAxis,
                    scaleType: 'time',
                    valueFormatter: (value: number) => {
                      const date = new Date(value);
                      return date.toLocaleDateString('en-US', { weekday: 'short', day: 'numeric' });
                    },
                    tickNumber: 7,
                    labelStyle: {
                      angle: 0,
                      textAnchor: 'middle'
                    }
                  }]}
                  series={chartData.series.map(series => ({
                    ...series,
                    showMarkers: false,
                    area: true,
                    lineStyle: { marker: { display: 'none' } }
                  }))}
                  height={300}
                  slotProps={{
                    legend: {
                      hidden: true
                    }
                  }}
                  sx={{
                    '& .MuiAreaElement-root': {
                      fill: 'url(#usageGradient)',
                    },
                    '& .MuiMarkElement-root': {
                      display: 'none',
                    },
                  }}
                >
                  <defs>
                    <linearGradient id="usageGradient" x1="0" y1="0" x2="0" y2="1">
                      <stop offset="0%" stopColor="#00c8ff" stopOpacity={0.5} />
                      <stop offset="100%" stopColor="#070714" stopOpacity={0.1} />
                    </linearGradient>
                  </defs>
                </LineChart>
              ) : (
                <Typography variant="body1" textAlign="center">No usage data available</Typography>
              )}
            </Box>

            <TableContainer component={Paper}>
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
                  {sortedUserTotals.map(({ user, promptTokens, completionTokens, totalTokens }) => (
                    <TableRow key={user?.id || 'unknown'}>
                      <TableCell>{user?.username || 'Unknown User'}</TableCell>
                      <TableCell>{user?.email || 'N/A'}</TableCell>
                      <TableCell align="right">{promptTokens.toLocaleString()}</TableCell>
                      <TableCell align="right">{completionTokens.toLocaleString()}</TableCell>
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