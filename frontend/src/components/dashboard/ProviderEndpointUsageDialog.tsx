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
    totalTokens: (userData.metrics || []).reduce((sum, metric) => sum + (metric.total_tokens || 0), 0)
  }));

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

  return (
    <Dialog
      open={open}
      onClose={onClose}
      maxWidth="md"
      fullWidth
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
                  }]}
                  series={chartData.series}
                  height={300}
                />
              ) : (
                <Typography variant="body1" textAlign="center">No usage data available</Typography>
              )}
            </Box>

            <TableContainer component={Paper}>
              <Table>
                <TableHead>
                  <TableRow>
                    <TableCell>User</TableCell>
                    <TableCell>Email</TableCell>
                    <TableCell align="right">Total Tokens (7 days)</TableCell>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {userTotals.map(({ user, totalTokens }) => (
                    <TableRow key={user?.id || 'unknown'}>
                      <TableCell>{user?.username || 'Unknown User'}</TableCell>
                      <TableCell>{user?.email || 'N/A'}</TableCell>
                      <TableCell align="right">{totalTokens.toLocaleString()}</TableCell>
                    </TableRow>
                  ))}
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