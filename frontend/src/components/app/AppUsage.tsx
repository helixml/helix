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
  TextField,
  InputAdornment,
  Grid,
  Select,
  MenuItem,
  FormControl,
  InputLabel,
} from '@mui/material';
import RefreshIcon from '@mui/icons-material/Refresh';
import KeyboardArrowDownIcon from '@mui/icons-material/KeyboardArrowDown';
import KeyboardArrowUpIcon from '@mui/icons-material/KeyboardArrowUp';
import WarningIcon from '@mui/icons-material/Warning';
import VisibilityIcon from '@mui/icons-material/Visibility';
import { ThumbsUp, ThumbsDown } from 'lucide-react'
// import SearchIcon from '@mui/icons-material/Search';
import { CircleCheck, Cog, OctagonX, Search, ExternalLink, Eye } from 'lucide-react';
import { TypesLLMCall, TypesInteraction, TypesInteractionState, TypesFeedback } from '../../api/api';
import { TypesUsersAggregatedUsageMetric } from '../../api/api';
import useAccount from '../../hooks/useAccount';
import LLMCallTimelineChart from './LLMCallTimelineChart';
import LLMCallDialog from './LLMCallDialog';
import InteractionDialog from './InteractionDialog';
import TokenUsage from '../usage/TokenUsage';
import TotalRequests from '../usage/TotalRequests';
import TotalCost from '../usage/TotalCost';

import { useGetAppUsage } from '../../services/appService';
import { useListAppInteractions } from '../../services/interactionsService';
import { useListAppLLMCalls } from '../../services/llmCallsService';
import { useListAppSteps } from '../../services/appService';

interface AppLogsTableProps {
  appId: string;
}

type PeriodType = '1d' | '7d' | '1m' | '6m';

const win = (window as any)

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

const AppLogsTable: FC<AppLogsTableProps> = ({ appId }) => {
  const theme = useTheme();
  const [page, setPage] = useState(0);
  const [rowsPerPage, setRowsPerPage] = useState(100);
  const [expandedRows, setExpandedRows] = useState<Set<string>>(new Set());
  const [hoveredCallId, setHoveredCallId] = useState<string | null>(null);
  const [selectedLLMCall, setSelectedLLMCall] = useState<TypesLLMCall | null>(null);
  const [llmCallDialogOpen, setLlmCallDialogOpen] = useState(false);
  const [selectedInteraction, setSelectedInteraction] = useState<TypesInteraction | null>(null);
  const [interactionDialogOpen, setInteractionDialogOpen] = useState(false);
  const [selectedPeriod, setSelectedPeriod] = useState<PeriodType>('7d');
  const [searchQuery, setSearchQuery] = useState('');
  const [feedback, setFeedback] = useState<string>('');

  // Load interactions at the top level
  const { data: interactionsData, isLoading: interactionsLoading, refetch: refetchInteractions } = useListAppInteractions(appId, '', '', feedback, page + 1, rowsPerPage);

  // Auto-reload logic for waiting interactions
  useEffect(() => {
    if (!interactionsData?.interactions) return;

    // Check if there are any interactions in "waiting" state
    const hasWaitingInteractions = interactionsData.interactions.some(
      interaction => interaction.state === 'waiting'
    );

    if (hasWaitingInteractions) {
      // Set up interval to reload every 5 seconds
      const interval = setInterval(() => {
        refetchInteractions();
      }, 5000);

      // Cleanup interval on unmount or when no more waiting interactions
      return () => clearInterval(interval);
    }
  }, [interactionsData?.interactions, refetchInteractions]);

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

  const handleChangePage = (event: unknown, newPage: number) => {
    setPage(newPage);
  };

  const handleChangeRowsPerPage = (event: React.ChangeEvent<HTMLInputElement>) => {
    setRowsPerPage(parseInt(event.target.value, 10));
    setPage(0);
  };

  const handleRefresh = () => {
    refetchInteractions();
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







  // Filter interactions based on search query
  const filteredInteractions = useMemo(() => {
    if (!interactionsData?.interactions || !searchQuery.trim()) {
      return interactionsData?.interactions || [];
    }

    const query = searchQuery.trim().toLowerCase();
    
    if (query.startsWith('ses_')) {
      // Filter by session ID
      const sessionId = query.substring(4); // Remove 'ses_' prefix
      return interactionsData.interactions.filter(interaction => 
        interaction.session_id?.toLowerCase().includes(sessionId)
      );
    } else if (query.startsWith('int_')) {
      // Filter by interaction ID
      const interactionId = query.substring(4); // Remove 'int_' prefix
      return interactionsData.interactions.filter(interaction => 
        interaction.id?.toLowerCase().includes(interactionId)
      );
    } else {
      // General search across session_id, interaction.id, and prompt_message
      return interactionsData.interactions.filter(interaction => 
        interaction.session_id?.toLowerCase().includes(query) ||
        interaction.id?.toLowerCase().includes(query) ||
        interaction.prompt_message?.toLowerCase().includes(query)
      );
    }
  }, [interactionsData?.interactions, searchQuery]);

  const handleOpenLLMCallDialog = (call: TypesLLMCall) => {
    setSelectedLLMCall(call);
    setLlmCallDialogOpen(true);
  };

  const handleCloseLLMCallDialog = () => {
    setLlmCallDialogOpen(false);
    setSelectedLLMCall(null);
  };

  const handleOpenInteractionDialog = (interaction: TypesInteraction) => {
    setSelectedInteraction(interaction);
    setInteractionDialogOpen(true);
  };

  const handleCloseInteractionDialog = () => {
    setInteractionDialogOpen(false);
    setSelectedInteraction(null);
  };

  // State to store LLM calls for the selected interaction
  const [selectedInteractionLLMCalls, setSelectedInteractionLLMCalls] = useState<TypesLLMCall[]>([]);

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

  // Helper function to truncate text
  const truncateText = (text: string, maxLength: number = 80): string => {
    if (!text) return '';
    return text.length > maxLength ? text.substring(0, maxLength) + '...' : text;
  };

  // Helper function to get status display
  const getStatusDisplay = (state: string) => {
    if (state === 'waiting') {
      return (
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
          <Cog 
            size={16} 
            style={{ 
              animation: 'spin 1s linear infinite',
              color: theme.palette.secondary.main
            }} 
          />
          <span>Running</span>
        </Box>
      );
    }

    // If completed, show green checkmark
    if (state === 'complete') {
      return (
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
          <CircleCheck
            size={16}
          />
          <span>Completed</span>
        </Box>
      );
    }

    if (state === 'error') {
      return (
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
          <OctagonX
            size={16}
          />
          <span>Error</span>
        </Box>
      );
    }


    return state || 'unknown';
  };

  // Helper function to get feedback icon
  const getFeedbackIcon = (feedback: TypesFeedback | undefined) => {
    if (!feedback) return '-';
    if (feedback === TypesFeedback.FeedbackLike) {
      return <ThumbsUp size={16} strokeWidth={0} fill="#4caf50" />;
    } 
    
    if (feedback === TypesFeedback.FeedbackDislike) {
      return <ThumbsDown size={16} strokeWidth={0} fill="#f44336" />;
    }
  };

  if (!interactionsData) return null;

  return (
    <div>
      <style>
        {`
          @keyframes spin {
            from {
              transform: rotate(0deg);
            }
            to {
              transform: rotate(360deg);
            }
          }
        `}
      </style>
      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mt: 2, mr: 2 }}>
        <Typography variant="h6">Usage</Typography>
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
      <Box sx={{ p: 2 }}> 
        {usageLoading ? (
          <Typography variant="body1" textAlign="center">Loading usage data...</Typography>
        ) : usageData && usageData.length > 0 ? (
          <Grid container spacing={2}>
            {/* Tokens Chart */}
            <Grid item xs={12} md={4}>
              <TokenUsage usageData={usageData as TypesUsersAggregatedUsageMetric[]} isLoading={usageLoading} />
            </Grid>

            {/* Request Count Chart */}
            <Grid item xs={12} md={4}>
              <TotalRequests usageData={usageData as TypesUsersAggregatedUsageMetric[]} isLoading={usageLoading} />
            </Grid>

            {/* Costs Chart */}
            <Grid item xs={12} md={4}>
              <TotalCost usageData={usageData as TypesUsersAggregatedUsageMetric[]} isLoading={usageLoading} />
            </Grid>
          </Grid>
        ) : (
          <Typography variant="body1" textAlign="center">No usage data available</Typography>
        )}
      </Box>

      <Divider sx={{ my: 2 }} />

      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', p: 2,  mr: 2 }}>
        <Typography variant="h6">Agent Interactions</Typography>

        <Box sx={{ display: 'flex', alignItems: 'center', gap: 2 }}>
          <FormControl size="small" sx={{ minWidth: 150 }}>
            <InputLabel>Rating</InputLabel>
            <Select
              value={feedback}
              onChange={(e) => setFeedback(e.target.value)}
              label="By feedback"
              sx={{
                '& .MuiOutlinedInput-root': {
                  bgcolor: 'rgba(0, 0, 0, 0.2)',
                  borderRadius: 1,
                  '& fieldset': { border: 'none' },
                  '&:hover fieldset': { border: 'none' },
                  '&.Mui-focused fieldset': { border: 'none' },
                },
                '& .MuiInputBase-input': {
                  color: 'white',
                  fontSize: '0.875rem',
                },
                '& .MuiSelect-icon': {
                  color: 'white',
                },
              }}
            >
              <MenuItem value="">All</MenuItem>
              <MenuItem value="like">Love it</MenuItem>
              <MenuItem value="dislike">Needs improvement</MenuItem>
            </Select>
          </FormControl>

          <TextField
            variant="outlined"
            size="small"
            placeholder="Session or interaction ID"
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            InputProps={{
              startAdornment: (
                <InputAdornment position="start">
                  <Search size={16} />
                </InputAdornment>
              ),
            }}
            sx={{
              width: 300,
              '& .MuiOutlinedInput-root': {
                bgcolor: 'rgba(0, 0, 0, 0.2)',
                borderRadius: 1,
                '& fieldset': { border: 'none' },
                '&:hover fieldset': { border: 'none' },
                '&.Mui-focused fieldset': { border: 'none' },
              },
              '& .MuiInputBase-input': {
                color: 'white',
                fontSize: '0.875rem',
              },
            }}
          />
        </Box>
      </Box>
      {searchQuery.trim() && (
        <Box sx={{ px: 2, mb: 1 }}>
          <Typography variant="caption" color="text.secondary">
            Showing {filteredInteractions.length} of {interactionsData?.interactions?.length || 0} interactions
          </Typography>
        </Box>
      )}
      <TableContainer sx={{ mt: 2, mr: 2 }}>
        <Table stickyHeader aria-label="Interactions table">
          <TableHead>
            <TableRow>
              <TableCell sx={headerCellStyle} width="50px"></TableCell>
              <TableCell sx={headerCellStyle}>Time</TableCell>
              <TableCell sx={headerCellStyle}>User Prompt</TableCell>
              <TableCell sx={headerCellStyle}>Status</TableCell>
              <TableCell sx={headerCellStyle} align="center">Rating</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            { win.DISABLE_LLM_CALL_LOGGING ? (
              <TableRow>
                <TableCell colSpan={4}>LLM call logging is disabled by the administrator.</TableCell>
              </TableRow>
            ) : (
              filteredInteractions.map((interaction) => (
                <React.Fragment key={interaction.id}>
                  <TableRow 
                    sx={{
                      ...(interaction.state === 'error' && {
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
                    onClick={() => interaction.id && toggleRow(interaction.id)}
                  >
                    <TableCell>
                      <IconButton
                        size="small"
                        onClick={(e) => {
                          e.stopPropagation();
                          interaction.id && toggleRow(interaction.id);
                        }}
                      >
                        {interaction.id && expandedRows.has(interaction.id) ? 
                          <KeyboardArrowUpIcon /> : 
                          <KeyboardArrowDownIcon />
                        }
                      </IconButton>
                    </TableCell>
                    <TableCell>{interaction.created ? new Date(interaction.created).toLocaleString() : ''}</TableCell>
                    <TableCell>
                      <Tooltip title={interaction.prompt_message || 'No prompt message'}>
                        <span>{truncateText(interaction.prompt_message || '', 80)}</span>
                      </Tooltip>
                    </TableCell>
                    <TableCell>
                      <Button 
                        color={interaction.state === 'error' ? 'error' : 'primary'}
                        disabled
                      >
                        {getStatusDisplay(interaction.state || 'unknown')}
                      </Button>
                    </TableCell>
                    <TableCell>
                      <Box sx={{ display: 'flex', justifyContent: 'center', alignItems: 'center' }}>
                        {getFeedbackIcon(interaction.feedback)}
                      </Box>
                    </TableCell>
                  </TableRow>
                  <TableRow>
                    <TableCell style={{ paddingBottom: 0, paddingTop: 0 }} colSpan={4}>
                      <Collapse in={interaction.id ? expandedRows.has(interaction.id) : false} timeout="auto" unmountOnExit>
                        <InteractionDetails 
                          appId={appId} 
                          interaction={interaction}
                          onHoverCallId={setHoveredCallId}
                          highlightedCallId={hoveredCallId}
                          onOpenLLMCallDialog={handleOpenLLMCallDialog}
                          onOpenInteractionDialog={(interaction, llmCalls) => {
                            console.log('interaction', interaction)
                            console.log('llmCalls', llmCalls)
                            setSelectedInteraction(interaction);
                            setSelectedInteractionLLMCalls(llmCalls);
                            setInteractionDialogOpen(true);
                          }}
                        />
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
        count={searchQuery.trim() ? filteredInteractions.length : (interactionsData.totalCount || 0)}
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
      
      <InteractionDialog
        open={interactionDialogOpen}
        onClose={handleCloseInteractionDialog}
        interaction={selectedInteraction}
        llmCalls={selectedInteractionLLMCalls}
      />
    </div>
  );
};

// New component to handle interaction details and LLM calls loading
interface InteractionDetailsProps {
  appId: string;
  interaction: TypesInteraction;
  onHoverCallId: (callId: string | null) => void;
  highlightedCallId: string | null;
  onOpenLLMCallDialog: (call: TypesLLMCall) => void;
  onOpenInteractionDialog: (interaction: TypesInteraction, llmCalls: TypesLLMCall[]) => void;
}

const InteractionDetails: FC<InteractionDetailsProps> = ({ 
  appId, 
  interaction, 
  onHoverCallId, 
  highlightedCallId, 
  onOpenLLMCallDialog,
  onOpenInteractionDialog
}) => {
  const account = useAccount();
  const [hoveredCallId, setHoveredCallId] = useState<string | null>(null);

  // Load LLM calls for this specific interaction when expanded
  const { data: llmCallsData, isLoading: llmCallsLoading } = useListAppLLMCalls(
    appId,
    interaction.session_id || '',
    interaction.id || '',
    1, // page    
    100, // pageSize
    interaction.id ? true : false, // enabled only when we have an interaction ID
    interaction.state === TypesInteractionState.InteractionStateWaiting ? 3000 : undefined // If interaction is in waiting state, keep refetching every 5 seconds
  );

  // Just refreshing the steps to get the latest step info
  useListAppSteps(appId, interaction.id || '', {
    refetchInterval: interaction.state === TypesInteractionState.InteractionStateWaiting ? 3000 : undefined
  });

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

  // TokenUsageIcon component (reused from parent)
  const TokenUsageIcon = ({ promptTokens, completionTokens }: { promptTokens: number, completionTokens: number }) => {
    const getBars = () => {
      if (promptTokens < 100) {
        return (
          <Box sx={{ display: 'flex', alignItems: 'flex-end', height: 16, gap: 0.5 }}>
            <Box sx={{ width: 3, height: 8, bgcolor: 'info.main' }} />
          </Box>
        )
      } else if (promptTokens < 2000) {
        return (
          <Box sx={{ display: 'flex', alignItems: 'flex-end', height: 16, gap: 0.5 }}>
            <Box sx={{ width: 3, height: 8, bgcolor: 'success.main' }} />
            <Box sx={{ width: 3, height: 12, bgcolor: 'success.main' }} />
          </Box>
        )
      } else if (promptTokens < 10000) {
        return (
          <Box sx={{ display: 'flex', alignItems: 'flex-end', height: 16, gap: 0.5 }}>
            <Box sx={{ width: 3, height: 8, bgcolor: 'warning.main' }} />
            <Box sx={{ width: 3, height: 12, bgcolor: 'warning.main' }} />
            <Box sx={{ width: 3, height: 16, bgcolor: 'warning.main' }} />
          </Box>
        )
      } else {
        return (
          <Box sx={{ display: 'flex', alignItems: 'flex-end', height: 16, gap: 0.5 }}>
            <Box sx={{ width: 3, height: 12, bgcolor: 'error.main' }} />
            <Box sx={{ width: 3, height: 16, bgcolor: 'error.main' }} />
            <Box sx={{ width: 3, height: 20, bgcolor: 'error.main' }} />
          </Box>
        )
      }
    }

    return (
      <Box sx={{ width: 32, display: 'flex', justifyContent: 'center', alignItems: 'center' }}>
        {getBars()}
      </Box>
    )
  };

  return (
    <Box sx={{ margin: 1 }}>
      <Box sx={{ mb: 2, p: 2, bgcolor: 'rgba(0, 0, 0, 0.2)', borderRadius: 1 }}>
        <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 2 }}>
          <Typography variant="subtitle2">Interaction Details</Typography>
          <Box sx={{ display: 'flex', gap: 1 }}>
            <Button
              variant="outlined"
              color="secondary"
              onClick={() => onOpenInteractionDialog(interaction, llmCallsData?.calls || [])}
              startIcon={<Eye size={16} />}
            >
              View Details
            </Button>     
            {interaction.user_id === account.user?.id && interaction.session_id && (
              <Button
                variant="outlined"
                color="secondary"
                href={`/session/${interaction.session_id}`}
                target="_blank"
                rel="noopener noreferrer"
                startIcon={<ExternalLink size={16} />}
              >
                View Session
              </Button>
            )}
          </Box>     
        </Box>
        <Box sx={{ display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: 2 }}>
          <Box>
            <Typography variant="caption" color="text.secondary">Session ID</Typography>
            <Typography variant="body2">
              {interaction.session_id ? (
                <Link href={`/session/${interaction.session_id}`} target="_blank" rel="noopener noreferrer">
                  {interaction.session_id}
                </Link>
              ) : 'N/A'}
            </Typography>
          </Box>
          <Box>
            <Typography variant="caption" color="text.secondary">Interaction ID</Typography>
            <Typography variant="body2">{interaction.id}</Typography>
          </Box>
          <Box>
            <Typography variant="caption" color="text.secondary">User ID</Typography>
            <Typography variant="body2">{interaction.user_id || 'N/A'}</Typography>
          </Box>
          <Box>
            <Typography variant="caption" color="text.secondary">Duration</Typography>
            <Typography variant="body2">
              {interaction.duration_ms ? formatDuration(interaction.duration_ms) : 'N/A'}
            </Typography>
          </Box>
          <Box>
            <Typography variant="caption" color="text.secondary">Total Cost</Typography>
            <Typography variant="body2">
              {(() => {
                if (!llmCallsData?.calls || llmCallsData.calls.length === 0) return 'N/A';
                
                const totalCost = llmCallsData.calls.reduce((sum, call) => {
                  return sum + (call.total_cost || 0);
                }, 0);
                
                return totalCost > 0 ? `$${totalCost.toFixed(4)}` : 'N/A';
              })()}
            </Typography>
          </Box>
        </Box>
      </Box>

      {llmCallsLoading ? (
        <Typography variant="body2" textAlign="center" sx={{ py: 2 }}>Loading LLM calls...</Typography>
      ) : llmCallsData?.calls && llmCallsData.calls.length > 0 ? (
        <>
          <LLMCallTimelineChart
            appId={appId}
            interactionId={interaction.id || ''}
            calls={llmCallsData.calls.map(call => ({
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
            onHoverCallId={onHoverCallId}
            highlightedCallId={highlightedCallId}
          />
          <Table size="small" sx={{ bgcolor: 'rgba(0, 0, 0, 0.2)' }}>
            <TableHead>
              <TableRow>
                <TableCell>Timestamp</TableCell>
                <TableCell>Step</TableCell>                                
                <TableCell>Duration (ms)</TableCell>
                <TableCell>Model</TableCell>
                <TableCell>Token Usage</TableCell>
                <TableCell>Cost</TableCell>
                <TableCell>Details</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {llmCallsData.calls.map((call) => (
                <TableRow 
                  key={call.id} 
                  sx={highlightedCallId === call.id ? { bgcolor: 'rgba(0,200,255,0.12)' } : {}}
                  onMouseEnter={() => call.id && onHoverCallId(call.id)}
                  onMouseLeave={() => onHoverCallId(null)}
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
                    <Tooltip
                      placement='right'
                      enterDelay={50}
                      enterNextDelay={50}
                      title={
                        <div style={{ minWidth: '200px' }}>
                          <div style={{ display: 'flex', justifyContent: 'space-between' }}>
                            <span>Prompt tokens:</span>
                            <span>{call.prompt_tokens || 0}</span>
                          </div>
                          <div style={{ display: 'flex', justifyContent: 'space-between' }}>
                            <span>Completion tokens:</span>
                            <span>{call.completion_tokens || 0}</span>
                          </div>
                        </div>
                      }
                      
                      slotProps={{ tooltip: { sx: { bgcolor: '#222', opacity: 1 } } }}
                    >
                      <span>
                        <TokenUsageIcon promptTokens={call.prompt_tokens || 0} completionTokens={call.completion_tokens || 0} />
                      </span>
                    </Tooltip>
                  </TableCell>
                  <TableCell>
                    <Tooltip 
                      placement='right'
                      enterDelay={50}
                      enterNextDelay={50}
                      title={
                        call.total_cost === 0 || call.prompt_cost === 0 || call.completion_cost === 0 ? (
                          <div style={{ minWidth: '200px' }}>
                            Pricing is not available for this call
                          </div>
                        ) : (
                          <div style={{ minWidth: '200px' }}>
                            <div style={{ display: 'flex', justifyContent: 'space-between' }}>
                              <span>Prompt cost:</span>
                              <span>{call.prompt_cost ? `$${call.prompt_cost.toFixed(6)}` : 'n/a'}</span>
                            </div>
                            <div style={{ display: 'flex', justifyContent: 'space-between' }}>
                              <span>Completion cost:</span>
                              <span>{call.completion_cost ? `$${call.completion_cost.toFixed(6)}` : 'n/a'}</span>
                            </div>
                          </div>
                        )
                      }
                      
                      slotProps={{ tooltip: { sx: { bgcolor: '#222', opacity: 1 } } }}
                    >
                      <span>
                      {call.total_cost && call.total_cost > 0 ? `$${call.total_cost.toFixed(2)}` : '-'}
                      </span>
                    </Tooltip>                    
                  </TableCell>
                  <TableCell>
                    <IconButton
                      onClick={() => onOpenLLMCallDialog(call)}
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
        </>
      ) : (
        <Typography variant="body2" textAlign="center" sx={{ py: 2 }}>No LLM calls found for this interaction</Typography>
      )}
    </Box>
  );
};

export default AppLogsTable; 