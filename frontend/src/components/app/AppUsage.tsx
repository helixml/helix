import React, { FC, useState, useEffect } from 'react';
import {
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Paper,
  TablePagination,
  Button,
  Box,
  Typography,
  Modal,
  Divider,
  Tooltip,
} from '@mui/material';
import RefreshIcon from '@mui/icons-material/Refresh';
import useApi from '../../hooks/useApi';
import { TypesPaginatedLLMCalls, TypesLLMCall } from '../../api/api';
import JsonView from '../widgets/JsonView';
import { LineChart } from '@mui/x-charts';
import { TypesUsersAggregatedUsageMetric, TypesAggregatedUsageMetric } from '../../api/api';

interface AppLogsTableProps {
  appId: string;
}

const win = (window as any)

const AppLogsTable: FC<AppLogsTableProps> = ({ appId }) => {
  const api = useApi();
  const apiClient = api.getApiClient();
  const [llmCalls, setLLMCalls] = useState<TypesPaginatedLLMCalls | null>(null);
  const [usageData, setUsageData] = useState<TypesUsersAggregatedUsageMetric[]>([]);
  const [page, setPage] = useState(0);
  const [rowsPerPage, setRowsPerPage] = useState(10);
  const [modalContent, setModalContent] = useState<any>(null);
  const [modalOpen, setModalOpen] = useState(false);

  const headerCellStyle = {
    bgcolor: 'rgba(0, 0, 0, 0.2)',
    backdropFilter: 'blur(10px)'
  };  

  const fetchUsageData = async () => {
    try {
      const response = await apiClient.v1AppsUsersDailyUsageDetail(appId);
      setUsageData(response.data as unknown as TypesUsersAggregatedUsageMetric[]);
    } catch (error) {
      console.error('Error fetching usage data:', error);
    }
  };

  const fetchLLMCalls = async () => {
    try {
      const queryParams = new URLSearchParams({
        page: (page + 1).toString(),
        pageSize: rowsPerPage.toString(),
      }).toString();

      const data = await api.get<TypesPaginatedLLMCalls>(`/api/v1/apps/${appId}/llm-calls?${queryParams}`);
      setLLMCalls(data);
    } catch (error) {
      console.error('Error fetching LLM calls:', error);
    }
  };

  useEffect(() => {
    if (appId !== 'new') {
      fetchLLMCalls();
      fetchUsageData();
    }
  }, [page, rowsPerPage, appId]);

  const handleChangePage = (event: unknown, newPage: number) => {
    setPage(newPage);
  };

  const handleChangeRowsPerPage = (event: React.ChangeEvent<HTMLInputElement>) => {
    setRowsPerPage(parseInt(event.target.value, 10));
    setPage(0);
  };

  const handleRefresh = () => {
    fetchLLMCalls();
    fetchUsageData();
  };

  const handleOpenModal = (content: any, call: TypesLLMCall) => {
    setModalContent({
      content,
      sessionId: call.session_id,
      interactionId: call.interaction_id,
      step: call.step,
      isError: !!call.error
    });
    setModalOpen(true);
  };

  const handleCloseModal = () => {
    setModalOpen(false);
  };

  // Prepare data for the line chart
  const prepareChartData = () => {
    if (!usageData.length) return { xAxis: [], series: [] };

    // Get all unique dates across all users
    const allDates = new Set<string>();
    usageData.forEach(userData => {
      userData.metrics?.forEach(metric => {
        if (metric.date) allDates.add(metric.date);
      });
    });

    // Sort dates
    const sortedDates = Array.from(allDates).sort();
    const xAxisDates = sortedDates.map(date => new Date(date));

    // Create series data for each user
    const series = usageData.map(userData => {
      const userMetricsByDate = new Map<string, TypesAggregatedUsageMetric>();
      userData.metrics?.forEach(metric => {
        if (metric.date) userMetricsByDate.set(metric.date, metric);
      });

      return {
        label: userData.user?.username || 'Unknown User',
        data: sortedDates.map(date => {
          const metric = userMetricsByDate.get(date);
          return metric?.total_tokens || 0;
        }),
      };
    });

    return {
      xAxis: xAxisDates,
      series,
    };
  };

  const chartData = prepareChartData();

  if (!llmCalls) return null;

  return (
    <Paper 
      sx={{ 
        width: '100%', 
        overflow: 'hidden',
        bgcolor: 'transparent',
        backdropFilter: 'blur(10px)'
      }}
    >
      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', p: 2 }}>
        <Typography variant="h6">Usage tokens (last 7 days)</Typography>
        <Button startIcon={<RefreshIcon />} onClick={handleRefresh}>
          Refresh
        </Button>
      </Box>
      <Box sx={{ p: 2, height: 300 }}> 
        {chartData.series.length > 0 ? (
          <LineChart
            xAxis={[{
              data: chartData.xAxis,
              scaleType: 'time',
              valueFormatter: (value: number) => {
                const date = new Date(value);
                return date.toLocaleDateString('en-US', { weekday: 'short', day: 'numeric' });
              },
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

      <Divider sx={{ my: 2 }} />

      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', p: 2 }}>
        <Typography variant="h6">LLM calls</Typography>        
      </Box>
      <TableContainer sx={{ mt: 2 }}>
        <Table stickyHeader aria-label="LLM calls table">
          <TableHead>
            <TableRow>
              <TableCell sx={headerCellStyle}>Created</TableCell>
              <TableCell sx={headerCellStyle}>Session ID</TableCell>
              <TableCell sx={headerCellStyle}>Step</TableCell>
              <TableCell sx={headerCellStyle}>Duration (ms)</TableCell>
              <TableCell sx={headerCellStyle}>Original Request</TableCell>
              <TableCell sx={headerCellStyle}>Request</TableCell>
              <TableCell sx={headerCellStyle}>Response</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            { win.DISABLE_LLM_CALL_LOGGING ? (
              <TableRow>
                <TableCell colSpan={6}>LLM call logging is disabled by the administrator.</TableCell>
              </TableRow>
            ) : (
              llmCalls.calls?.map((call: TypesLLMCall) => (
                <TableRow 
                  key={call.id}
                  sx={{
                    ...(call.error && {
                      border: '2px solid #ff4d4f',
                      bgcolor: 'rgba(255, 77, 79, 0.1)',
                      '& td': {
                        borderColor: 'rgba(255, 77, 79, 0.2)'
                      }
                    })
                  }}
                >
                <TableCell>{call.created ? new Date(call.created).toLocaleString() : ''}</TableCell>
                <TableCell>{call.session_id}</TableCell>
                <TableCell>{call.step || 'n/a'}</TableCell>
                <TableCell>{call.duration_ms || 'n/a'}</TableCell>
                <TableCell>
                  {call.original_request && (
                    <Button onClick={() => handleOpenModal(call.original_request, call)}>View</Button>
                  )}
                </TableCell>
                <TableCell>
                  <Button onClick={() => handleOpenModal(call.request, call)}>View</Button>
                </TableCell>
                <TableCell>
                  <Tooltip title={call.error || ''}>
                    <span>
                      <Button onClick={() => handleOpenModal(call.error ? { error: call.error } : call.response, call)}>
                        {call.error ? 'View Error' : 'View'}
                      </Button>
                    </span>
                  </Tooltip>
                </TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </TableContainer>
      <TablePagination
        rowsPerPageOptions={[10, 25, 100]}
        component="div"
        count={llmCalls.totalCount || 0}
        rowsPerPage={rowsPerPage}
        page={page}
        onPageChange={handleChangePage}
        onRowsPerPageChange={handleChangeRowsPerPage}
      />
      <Modal
        open={modalOpen}
        onClose={handleCloseModal}
        aria-labelledby="json-modal-title"
        aria-describedby="json-modal-description"
      >
        <Box sx={{
          position: 'absolute',
          top: '50%',
          left: '50%',
          transform: 'translate(-50%, -50%)',
          width: '80%',
          maxHeight: '80%',
          bgcolor: '#070714',
          border: '2px solid #000',
          boxShadow: 24,
          p: 4,
          overflow: 'auto',
        }}>
          <Typography id="json-modal-title" variant="h6" component="h2" gutterBottom>
            JSON Content
          </Typography>
          
          <Box sx={{ mb: 2, p: 2, bgcolor: 'rgba(0, 0, 0, 0.1)', borderRadius: 1 }}>
            <Typography variant="subtitle2" gutterBottom>
              Session ID: {modalContent?.sessionId}
            </Typography>
            <Typography variant="subtitle2" gutterBottom>
              Interaction ID: {modalContent?.interactionId}
            </Typography>
            <Typography variant="subtitle2" gutterBottom>
              Step: {modalContent?.step}
            </Typography>
            {modalContent?.isError && (
              <Typography variant="subtitle2" color="error" gutterBottom>
                Error occurred during this call
              </Typography>
            )}
          </Box>

          <JsonView data={modalContent?.content} />
        </Box>
      </Modal>
    </Paper>
  );
};

export default AppLogsTable; 