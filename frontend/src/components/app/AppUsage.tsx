import React, { FC, useState, useEffect, useMemo } from 'react';
import {
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  TablePagination,
  Button,
  Box,
  Typography,
  Divider,
  Tooltip,
  IconButton,
  Collapse,
  Link,
  useTheme,
  ToggleButton,
  ToggleButtonGroup,
} from '@mui/material';
import RefreshIcon from '@mui/icons-material/Refresh';
import KeyboardArrowDownIcon from '@mui/icons-material/KeyboardArrowDown';
import KeyboardArrowUpIcon from '@mui/icons-material/KeyboardArrowUp';
import OpenInNewIcon from '@mui/icons-material/OpenInNew';
import WarningIcon from '@mui/icons-material/Warning';
import VisibilityIcon from '@mui/icons-material/Visibility';
import useApi from '../../hooks/useApi';
import { TypesPaginatedLLMCalls, TypesLLMCall } from '../../api/api';
import { LineChart } from '@mui/x-charts';
import { TypesUsersAggregatedUsageMetric, TypesAggregatedUsageMetric } from '../../api/api';
import useAccount from '../../hooks/useAccount';
import LLMCallTimelineChart from './LLMCallTimelineChart';
import LLMCallDialog from './LLMCallDialog';
import { useGetAppUsage } from '../../services/appService';

// Add TokenUsageIcon component
const TokenUsageIcon = ({ promptTokens }: { promptTokens: number }) => {
  const getBars = () => {
    if (promptTokens < 100) {
      // Blue for low usage
      return (
        <Box sx={{ display: 'flex', alignItems: 'flex-end', height: 16, gap: 0.5 }}>
          <Box sx={{ width: 3, height: 8, bgcolor: 'info.main' }} />
        </Box>
      )
    } else if (promptTokens < 2000) {
      // Green for moderate usage
      return (
        <Box sx={{ display: 'flex', alignItems: 'flex-end', height: 16, gap: 0.5 }}>
          <Box sx={{ width: 3, height: 8, bgcolor: 'success.main' }} />
          <Box sx={{ width: 3, height: 12, bgcolor: 'success.main' }} />
        </Box>
      )
    } else if (promptTokens < 10000) {
      // Yellow warning for high usage
      return (
        <Box sx={{ display: 'flex', alignItems: 'flex-end', height: 16, gap: 0.5 }}>
          <Box sx={{ width: 3, height: 8, bgcolor: 'warning.main' }} />
          <Box sx={{ width: 3, height: 12, bgcolor: 'warning.main' }} />
          <Box sx={{ width: 3, height: 16, bgcolor: 'warning.main' }} />
        </Box>
      )
    } else {
      // Red for very high usage
      return (
        <Box sx={{ display: 'flex', alignItems: 'flex-end', height: 16, gap: 0.5 }}>
          {/* <Box sx={{ width: 3, height: 8, bgcolor: 'error.main' }} /> */}
          <Box sx={{ width: 3, height: 12, bgcolor: 'error.main' }} />
          <Box sx={{ width: 3, height: 16, bgcolor: 'error.main' }} />
          <Box sx={{ width: 3, height: 20, bgcolor: 'error.main' }} />
        </Box>
      )
    }
  }

  // Add a fixed width and center the bars
  return (
    <Box sx={{ width: 32, display: 'flex', justifyContent: 'center', alignItems: 'center' }}>
      {getBars()}
    </Box>
  )
}

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

type PeriodType = '1d' | '7d' | '1m' | '6m';

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

const getDateRange = (period: PeriodType): { from: string; to: string } => {
  const now = new Date();
  const to = now.toISOString().split('T')[0]; // YYYY-MM-DD format
  
  const from = new Date();
  switch (period) {
    case '1d':
      from.setDate(from.getDate() - 1);
      break;
    case '7d':
      from.setDate(from.getDate() - 7);
      break;
    case '1m':
      from.setMonth(from.getMonth() - 1);
      break;
    case '6m':
      from.setMonth(from.getMonth() - 6);
      break;
  }
  
  return {
    from: from.toISOString().split('T')[0],
    to
  };
};

const getPeriodLabel = (period: PeriodType): string => {
  switch (period) {
    case '1d': return '1 day';
    case '7d': return '7 days';
    case '1m': return '1 month';
    case '6m': return '6 months';
  }
};

const AppLogsTable: FC<AppLogsTableProps> = ({ appId }) => {
  const api = useApi();
  const apiClient = api.getApiClient();
  const account = useAccount();
  const theme = useTheme();
  const [llmCalls, setLLMCalls] = useState<TypesPaginatedLLMCalls | null>(null);
  const [page, setPage] = useState(0);
  const [rowsPerPage, setRowsPerPage] = useState(100);
  const [expandedRows, setExpandedRows] = useState<Set<string>>(new Set());
  const [hoveredCallId, setHoveredCallId] = useState<string | null>(null);
  const [selectedLLMCall, setSelectedLLMCall] = useState<TypesLLMCall | null>(null);
  const [llmCallDialogOpen, setLlmCallDialogOpen] = useState(false);
  const [selectedPeriod, setSelectedPeriod] = useState<PeriodType>('7d');

  const headerCellStyle = {
    bgcolor: 'rgba(0, 0, 0, 0.2)',
    backdropFilter: 'blur(10px)'
  };  

  // Calculate date range based on selected period
  const dateRange = getDateRange(selectedPeriod);

  // Use the useGetAppUsage hook
  const { data: usageData = [], isLoading: usageLoading, refetch: refetchUsage } = useGetAppUsage(
    appId,
    dateRange.from,
    dateRange.to
  );

  // Extract usage data from the response
  // const usageData = usageResponse?.data || []; // This line is no longer needed

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
    refetchUsage();
  };

  const handlePeriodChange = (event: React.MouseEvent<HTMLElement>, newPeriod: PeriodType | null) => {
    if (newPeriod !== null) {
      setSelectedPeriod(newPeriod);
    }
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
  const prepareChartData = (usageData: TypesUsersAggregatedUsageMetric[]) => {
    console.log('prepareChartData')
    console.log(usageData)

    if (usageLoading) return { xAxis: [], series: [] };

    if (!usageData || !Array.isArray(usageData) || usageData.length === 0) return { xAxis: [], series: [] };

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

    // Create series data for each user
    const series = usageData.map((userData: TypesUsersAggregatedUsageMetric) => {
      const userMetricsByDate = new Map<string, TypesAggregatedUsageMetric>();
      userData.metrics?.forEach((metric: TypesAggregatedUsageMetric) => {
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

  const chartData = useMemo(() => prepareChartData(usageData as TypesUsersAggregatedUsageMetric[]), [usageData, selectedPeriod, usageLoading]);

  const handleOpenLLMCallDialog = (call: TypesLLMCall) => {
    setSelectedLLMCall(call);
    setLlmCallDialogOpen(true);
  };

  const handleCloseLLMCallDialog = () => {
    setLlmCallDialogOpen(false);
    setSelectedLLMCall(null);
  };

  // Convert TypesLLMCall to LLMCall interface expected by LLMCallDialog
  const convertToLLMCall = (call: TypesLLMCall) => ({
    id: call.id || '',
    created: call.created || '',
    duration_ms: call.duration_ms || 0,
    step: call.step,
    model: call.model,
    response: call.response,
    request: call.request,
    provider: call.provider,
    prompt_tokens: call.prompt_tokens,
    completion_tokens: call.completion_tokens,
    total_tokens: call.total_tokens,
    error: call.error,
  });

  if (!llmCalls) return null;

  return (
    <div>
      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mt: 2, mr: 2 }}>
        <Typography variant="h6">Token usage</Typography>
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
          <ToggleButtonGroup
            value={selectedPeriod}
            exclusive
            onChange={handlePeriodChange}
            size="small"
            sx={{
              '& .MuiToggleButton-root': {
                px: 2,
                py: 0.5,
                fontSize: '0.875rem',
                fontWeight: 500,
              }
            }}
          >
            <ToggleButton value="1d">1d</ToggleButton>
            <ToggleButton value="7d">7d</ToggleButton>
            <ToggleButton value="1m">1m</ToggleButton>
            <ToggleButton value="6m">6m</ToggleButton>
          </ToggleButtonGroup>
          <Button startIcon={<RefreshIcon />} onClick={handleRefresh}>
            Refresh
          </Button>
        </Box>
      </Box>
      <Box sx={{ p: 2, height: 300 }}> 
        {usageLoading ? (
          <Typography variant="body1" textAlign="center">Loading usage data...</Typography>
        ) : chartData.series.length > 0 ? (
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
              tickMinStep: 24 * 60 * 60 * 1000,
              min: chartData.xAxis.length > 0 ? chartData.xAxis[0].getTime() : undefined,
              max: chartData.xAxis.length > 0 ? chartData.xAxis[chartData.xAxis.length - 1].getTime() : undefined
            }]}
            yAxis={[{                            
              valueFormatter: (value: number) => {
                if (value >= 1000) {
                  return `${(value / 1000).toFixed(0)}k`;
                }
                return value.toString();
              }
            }]}
            margin={{ left: 60, right: 20, top: 20, bottom: 40 }}
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

      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', p: 2,  mr: 2 }}>
        <Typography variant="h6">LLM calls</Typography>        
      </Box>
      <TableContainer sx={{ mt: 2, mr: 2 }}>
        <Table stickyHeader aria-label="LLM calls table">
          <TableHead>
            <TableRow>
              <TableCell sx={headerCellStyle} width="50px"></TableCell>
              <TableCell sx={headerCellStyle}>Time</TableCell>
              <TableCell sx={headerCellStyle}>Duration</TableCell>
              <TableCell sx={headerCellStyle}>Total Tokens</TableCell>
              <TableCell sx={headerCellStyle}>Status</TableCell>
              <TableCell sx={headerCellStyle}>Details</TableCell>
              <TableCell sx={headerCellStyle}>Actions</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            { win.DISABLE_LLM_CALL_LOGGING ? (
              <TableRow>
                <TableCell colSpan={7}>LLM call logging is disabled by the administrator.</TableCell>
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
                      <Button 
                        color={group.status === 'ERROR' ? 'error' : 'primary'}
                        disabled
                      >
                        {group.status}
                      </Button>
                    </TableCell>
                    <TableCell>
                      <Button 
                        onClick={(e) => {
                          e.stopPropagation();
                          // Get the latest call (last in the array since we sorted by oldest first)
                          const latestCall = group.calls[group.calls.length - 1];
                          if (latestCall) {
                            handleOpenLLMCallDialog(latestCall);
                          }
                        }}
                        variant="outlined"
                        size="small"
                      >
                        View Details
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
                    <TableCell style={{ paddingBottom: 0, paddingTop: 0 }} colSpan={7}>
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
                            appId={appId}
                            interactionId={group.interaction_id}
                            calls={group.calls.map(call => ({
                              id: call.id || '',
                              created: call.created || '',
                              duration_ms: call.duration_ms || 0,
                              step: call.step,
                              model: call.model,
                              response: call.response,
                              request: call.request,
                              error: call.error,
                              provider: call.provider,
                              prompt_tokens: call.prompt_tokens,
                              completion_tokens: call.completion_tokens,
                              total_tokens: call.total_tokens,
                            }))}
                            onHoverCallId={setHoveredCallId}
                            highlightedCallId={hoveredCallId}
                          />
                          <Table size="small" sx={{ bgcolor: 'rgba(0, 0, 0, 0.2)' }}>
                            <TableHead>
                              <TableRow>
                                <TableCell>Timestamp</TableCell>
                                <TableCell>Step</TableCell>
                                <TableCell>Token Usage</TableCell>
                                <TableCell>Duration (ms)</TableCell>
                                <TableCell>Model</TableCell>
                                <TableCell>Details</TableCell>
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
                                    <Tooltip title={`${call.prompt_tokens || 0} prompt tokens`}>
                                      <span>
                                        <TokenUsageIcon promptTokens={call.prompt_tokens || 0} />
                                      </span>
                                    </Tooltip>
                                  </TableCell>
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
                                    <IconButton
                                      onClick={() => handleOpenLLMCallDialog(call)}
                                      size="small"
                                      color="primary"
                                    >
                                      <VisibilityIcon />
                                    </IconButton>
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
      
      <LLMCallDialog
        open={llmCallDialogOpen}
        onClose={handleCloseLLMCallDialog}
        llmCall={selectedLLMCall ? convertToLLMCall(selectedLLMCall) : null}
      />
    </div>
  );
};

export default AppLogsTable; 