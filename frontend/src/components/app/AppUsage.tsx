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
  IconButton,
  Collapse,
  Link,
  useTheme,
} from '@mui/material';
import RefreshIcon from '@mui/icons-material/Refresh';
import KeyboardArrowDownIcon from '@mui/icons-material/KeyboardArrowDown';
import KeyboardArrowUpIcon from '@mui/icons-material/KeyboardArrowUp';
import OpenInNewIcon from '@mui/icons-material/OpenInNew';
import WarningIcon from '@mui/icons-material/Warning';
import useApi from '../../hooks/useApi';
import { TypesPaginatedLLMCalls, TypesLLMCall } from '../../api/api';
import JsonView from '../widgets/JsonView';
import { LineChart } from '@mui/x-charts';
import { TypesUsersAggregatedUsageMetric, TypesAggregatedUsageMetric } from '../../api/api';
import useAccount from '../../hooks/useAccount';
import LLMCallTimelineChart from './LLMCallTimelineChart';

interface AppLogsTableProps {
  appId: string;
}

interface GroupedLLMCall {
  interaction_id: string;
  created: string;
  total_duration: number;
  total_tokens: number;
  original_request: any;
  status: 'OK' | 'ERROR';
  calls: TypesLLMCall[];
  user_id?: string;
  session_id?: string;
}

const win = (window as any)

const formatDuration = (ms: number): string => {
  if (ms < 1000) {
    return `${ms}ms`;
  }
  const seconds = Math.floor(ms / 1000);
  if (seconds < 60) {
    return `${seconds}s`;
  }
  const minutes = Math.floor(seconds / 60);
  const remainingSeconds = seconds % 60;
  return `${minutes}m ${remainingSeconds}s`;
};

const AppLogsTable: FC<AppLogsTableProps> = ({ appId }) => {
  const api = useApi();
  const apiClient = api.getApiClient();
  const account = useAccount();
  const theme = useTheme();
  const [llmCalls, setLLMCalls] = useState<TypesPaginatedLLMCalls | null>(null);
  const [usageData, setUsageData] = useState<TypesUsersAggregatedUsageMetric[]>([]);
  const [page, setPage] = useState(0);
  const [rowsPerPage, setRowsPerPage] = useState(100);
  const [modalContent, setModalContent] = useState<any>(null);
  const [modalOpen, setModalOpen] = useState(false);
  const [expandedRows, setExpandedRows] = useState<Set<string>>(new Set());
  const [hoveredCallId, setHoveredCallId] = useState<string | null>(null);

  const headerCellStyle = {
    bgcolor: 'rgba(0, 0, 0, 0.2)',
    backdropFilter: 'blur(10px)'
  };  

  const parseRequest = (request: any): any => {
    try {
      if (typeof request === 'string') {
        return JSON.parse(request);
      }
      return request;
    } catch (e) {
      return request;
    }
  };

  const getReasoningEffort = (request: any): string => {
    const parsed = parseRequest(request);
    return parsed?.reasoning_effort || 'n/a';
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

  const handleOpenModal = (content: any) => {
    setModalContent(content);
    setModalOpen(true);
  };

  const handleCloseModal = () => {
    setModalOpen(false);
  };

  const toggleRow = (interactionId: string) => {
    const newExpandedRows = new Set(expandedRows);
    if (newExpandedRows.has(interactionId)) {
      newExpandedRows.delete(interactionId);
    } else {
      newExpandedRows.add(interactionId);
    }
    setExpandedRows(newExpandedRows);
  };

  // Group LLM calls by interaction_id
  const groupedCalls = React.useMemo(() => {
    if (!llmCalls?.calls) return [];

    const groups = new Map<string, GroupedLLMCall>();
    
    llmCalls.calls.forEach(call => {
      if (!call.interaction_id) return;
      
      if (!groups.has(call.interaction_id)) {
        groups.set(call.interaction_id, {
          interaction_id: call.interaction_id,
          created: call.created || '',
          total_duration: 0,
          total_tokens: 0,
          original_request: call.original_request,
          status: 'OK',
          calls: [],
          user_id: call.user_id,
          session_id: call.session_id,
        });
      }
      
      const group = groups.get(call.interaction_id)!;
      group.calls.push(call);
      group.total_tokens += call.total_tokens || 0;
      if (call.error) {
        group.status = 'ERROR';
      }
    });

    // Sort individual calls within each group by creation time (oldest to newest)
    groups.forEach(group => {
      group.calls.sort((a, b) => 
        new Date(a.created || '').getTime() - new Date(b.created || '').getTime()
      );
      
      // Calculate total duration based on first and last call timestamps
      if (group.calls.length > 0) {
        const firstCall = group.calls[0];
        const lastCall = group.calls[group.calls.length - 1];
        if (firstCall.created && lastCall.created) {
          const startTime = new Date(firstCall.created).getTime();
          const endTime = new Date(lastCall.created).getTime();
          group.total_duration = endTime - startTime;
        }
      }
    });

    return Array.from(groups.values()).sort((a, b) => 
      new Date(b.created).getTime() - new Date(a.created).getTime()
    );
  }, [llmCalls?.calls]);

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
        <Typography variant="h6">Token usage (last 7 days)</Typography>
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
              },
              tickMinStep: 24 * 60 * 60 * 1000
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
                <stop offset="0%" stopColor={theme.chartGradientStart} stopOpacity={theme.chartGradientStartOpacity} />
                <stop offset="100%" stopColor={theme.chartGradientEnd} stopOpacity={theme.chartGradientEndOpacity} />
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
              <TableCell sx={headerCellStyle} width="50px"></TableCell>
              <TableCell sx={headerCellStyle}>Time</TableCell>
              <TableCell sx={headerCellStyle}>Duration</TableCell>
              <TableCell sx={headerCellStyle}>Total Tokens</TableCell>
              <TableCell sx={headerCellStyle}>Original Request</TableCell>
              <TableCell sx={headerCellStyle}>Status</TableCell>
              <TableCell sx={headerCellStyle}>Actions</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            { win.DISABLE_LLM_CALL_LOGGING ? (
              <TableRow>
                <TableCell colSpan={8}>LLM call logging is disabled by the administrator.</TableCell>
              </TableRow>
            ) : (
              groupedCalls.map((group) => (
                <React.Fragment key={group.interaction_id}>
                  <TableRow 
                    sx={{
                      ...(group.status === 'ERROR' && {
                        border: '2px solid #ff4d4f',
                        bgcolor: 'rgba(255, 77, 79, 0.1)',
                        '& td': {
                          borderColor: 'rgba(255, 77, 79, 0.2)'
                        }
                      }),
                      cursor: 'pointer',
                      '&:hover': {
                        bgcolor: 'rgba(255, 255, 255, 0.05)'
                      }
                    }}
                    onClick={() => toggleRow(group.interaction_id)}
                  >
                    <TableCell>
                      <IconButton
                        size="small"
                        onClick={(e) => {
                          e.stopPropagation();
                          toggleRow(group.interaction_id);
                        }}
                      >
                        {expandedRows.has(group.interaction_id) ? 
                          <KeyboardArrowUpIcon /> : 
                          <KeyboardArrowDownIcon />
                        }
                      </IconButton>
                    </TableCell>
                    <TableCell>{group.created ? new Date(group.created).toLocaleString() : ''}</TableCell>
                    <TableCell>{formatDuration(group.total_duration)}</TableCell>
                    <TableCell>{group.total_tokens}</TableCell>
                    <TableCell>
                      {group.original_request && (
                        <Button onClick={() => handleOpenModal(group.original_request)}>View</Button>
                      )}
                    </TableCell>
                    <TableCell>
                      <Button 
                        onClick={() => {
                          // Get the latest call (last in the array since we sorted by oldest first)
                          const latestCall = group.calls[group.calls.length - 1];
                          if (latestCall) {
                            handleOpenModal(latestCall.error ? { error: latestCall.error } : latestCall.response);
                          }
                        }}
                        color={group.status === 'ERROR' ? 'error' : 'primary'}
                      >
                        {group.status}
                      </Button>
                    </TableCell>
                    <TableCell>
                      {group.user_id === account.user?.id && group.session_id && (
                        <Link href={`/session/${group.session_id}`} target="_blank" rel="noopener noreferrer">
                          <OpenInNewIcon />
                        </Link>
                      )}
                    </TableCell>
                  </TableRow>
                  <TableRow>
                    <TableCell style={{ paddingBottom: 0, paddingTop: 0 }} colSpan={8}>
                      <Collapse in={expandedRows.has(group.interaction_id)} timeout="auto" unmountOnExit>
                        <Box sx={{ margin: 1 }}>
                          <Box sx={{ mb: 2, p: 2, bgcolor: 'rgba(0, 0, 0, 0.2)', borderRadius: 1 }}>
                            <Typography variant="subtitle2" gutterBottom>Session Details</Typography>
                            <Box sx={{ display: 'grid', gridTemplateColumns: 'repeat(2, 1fr)', gap: 2 }}>
                              <Box>
                                <Typography variant="caption" color="text.secondary">Session ID</Typography>
                                <Typography variant="body2">
                                  {group.session_id ? (
                                    <Link href={`/session/${group.session_id}`} target="_blank" rel="noopener noreferrer">
                                      {group.session_id}
                                    </Link>
                                  ) : 'N/A'}
                                </Typography>
                              </Box>
                              <Box>
                                <Typography variant="caption" color="text.secondary">Interaction ID</Typography>
                                <Typography variant="body2">{group.interaction_id}</Typography>
                              </Box>
                              <Box>
                                <Typography variant="caption" color="text.secondary">User ID</Typography>
                                <Typography variant="body2">{group.user_id || 'N/A'}</Typography>
                              </Box>
                              <Box>
                                <Typography variant="caption" color="text.secondary">Total Tokens</Typography>
                                <Typography variant="body2">
                                  {group.calls.reduce((acc, call) => acc + (call.prompt_tokens || 0), 0)} prompt / {' '}
                                  {group.calls.reduce((acc, call) => acc + (call.completion_tokens || 0), 0)} completion
                                </Typography>
                              </Box>
                            </Box>
                          </Box>
                          <LLMCallTimelineChart
                            calls={group.calls.map(call => ({
                              id: call.id || '',
                              created: call.created || '',
                              duration_ms: call.duration_ms || 0,
                              step: call.step,
                              model: call.model,
                              response: call.response,
                              request: call.request,
                            }))}
                            onHoverCallId={setHoveredCallId}
                            highlightedCallId={hoveredCallId}
                          />
                          <Table size="small" sx={{ bgcolor: 'rgba(0, 0, 0, 0.2)' }}>
                            <TableHead>
                              <TableRow>
                                <TableCell>Timestamp</TableCell>
                                <TableCell>Step</TableCell>
                                <TableCell>Duration (ms)</TableCell>
                                <TableCell>Model</TableCell>
                                <TableCell>Request</TableCell>
                                <TableCell>Response</TableCell>
                              </TableRow>
                            </TableHead>
                            <TableBody>
                              {group.calls.map((call) => (
                                <TableRow 
                                  key={call.id} 
                                  sx={hoveredCallId === call.id ? { bgcolor: 'rgba(0,200,255,0.12)' } : {}}
                                  onMouseEnter={() => call.id && setHoveredCallId(call.id)}
                                  onMouseLeave={() => setHoveredCallId(null)}
                                >
                                  <TableCell>{call.created ? new Date(call.created).toLocaleString() : ''}</TableCell>
                                  <TableCell>{call.step || 'n/a'}</TableCell>
                                  <TableCell>
                                    {call.duration_ms ? formatDuration(call.duration_ms) : 'n/a'}
                                    {call.duration_ms && call.duration_ms > 5000 && (
                                      <Tooltip title="Model taking a long time to think">
                                        <WarningIcon 
                                          sx={{ 
                                            ml: 1, 
                                            color: '#ff9800',
                                            verticalAlign: 'middle',
                                            fontSize: '1rem'
                                          }} 
                                        />
                                      </Tooltip>
                                    )}
                                  </TableCell>
                                  <TableCell>
                                    <Tooltip title={getReasoningEffort(call.request) !== 'n/a' ? `Reasoning effort: ${getReasoningEffort(call.request)}` : ''}>
                                      <span>{call.model || 'n/a'}</span>
                                    </Tooltip>
                                  </TableCell>
                                  <TableCell>
                                    <Button onClick={() => handleOpenModal(call.request)}>View</Button>
                                  </TableCell>
                                  <TableCell>
                                    <Tooltip title={call.error || ''}>
                                      <span>
                                        <Button sx={{ mr: 3 }} onClick={() => handleOpenModal(call.error ? { error: call.error } : call.response)}>
                                          {call.error ? 'View Error' : 'View'}
                                        </Button>
                                      </span>
                                    </Tooltip>
                                  </TableCell>
                                </TableRow>
                              ))}
                            </TableBody>
                          </Table>
                        </Box>
                      </Collapse>
                    </TableCell>
                  </TableRow>
                </React.Fragment>
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
          <JsonView data={modalContent} />
        </Box>
      </Modal>
    </Paper>
  );
};

export default AppLogsTable; 