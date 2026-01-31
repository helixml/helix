import React, { useMemo, useRef, useState, useEffect } from 'react';
import { Box, Typography, Tooltip, useTheme } from '@mui/material';
import { TypesStepInfo } from '../../api/api';
import { useListAppSteps } from '../../services/appService';
import SkillExecutionDialog from './SkillExecutionDialog';
import LLMCallDialog from './LLMCallDialog';

interface LLMCall {
  id: string;
  created: string;
  duration_ms: number;
  step?: string;
  model?: string;
  response?: any;
  request?: any;
  provider?: string;
  prompt_tokens?: number;
  completion_tokens?: number;
  total_tokens?: number;
  error?: string;
}

// RowData contains row data information. It can have set either llm_call or action_info (not both).
// All rows contain created and duration_ms.
interface RowData {
  name: string; // Step name for action_info, or "step" for llm calls

  created: string;
  duration_ms: number;
  
  llm_call: LLMCall;          // Helix LLM calls with requests and responses. When response includes a tool call, next thing that's going to happen is a tool execution by Helix.
  action_info: TypesStepInfo; // Helix taken actions, for example used browser, calculator, ran python code, called API, etc.
}

interface LLMCallTimelineChartProps {
  calls: LLMCall[];  
  appId: string;
  interactionId: string;
  onHoverCallId?: (id: string | null) => void;
  highlightedCallId?: string | null;
}

const formatMs = (ms: number) => `${ms} ms`;

const ROW_HEIGHT = 22;
const BAR_HEIGHT = 14;
const LABEL_WIDTH = 0;
const CHART_PADDING = 24;
const MIN_BAR_WIDTH = 12;

const parseResponse = (response: any): any => {
  try {
    if (typeof response === 'string') {
      return JSON.parse(response);
    }
    return response;
  } catch (e) {
    return response;
  }
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

const getAssistantMessage = (response: any): string => {
  const parsed = parseResponse(response);
  return parsed?.choices?.[0]?.message?.content || 'n/a';
};

const getToolCalls = (response: any): any[] => {
  const parsed = parseResponse(response);
  return parsed?.choices?.[0]?.message?.tool_calls || [];
};

// sortRowDataByCreated - oldest to newest, when sorting include
// up to milliseconds precision 
const sortRowDataByCreated = (rows: RowData[]): RowData[] => {
  return [...rows].sort((a, b) => {
    const aTime = new Date(a.created).getTime();
    const bTime = new Date(b.created).getTime();
    return aTime - bTime;
  });
};

const getTooltipContent = (row: RowData): React.ReactNode => {
  const formatTime = (date: Date) => {
    return date.toLocaleTimeString();
  };


  const startTime = new Date(row.created);
  const endTime = new Date(startTime.getTime() + row.duration_ms);

  if (row.action_info) {
    return (
      <div >
        <div style={{ fontWeight: 'bold', display: 'flex', justifyContent: 'space-between' }}>
          <span>Skill execution:</span>
          <span>{row.name}</span>
        </div>
        <div style={{ display: 'flex', justifyContent: 'space-between' }}>
          <span>Started:</span>
          <span>{formatTime(startTime)}</span>
        </div>
        <div style={{ display: 'flex', justifyContent: 'space-between' }}>
          <span>Finished:</span>
          <span>{formatTime(endTime)}</span>
        </div>
        <div style={{ display: 'flex', justifyContent: 'space-between' }}>
          <span>Duration:</span>
          <span>{formatMs(row.duration_ms)}</span>
        </div>
        {row.action_info.details?.arguments && Object.keys(row.action_info.details.arguments).length > 0 && (
          <>
            <div style={{ marginTop: '4px', fontWeight: 'bold' }}>Arguments:</div>
            {Object.entries(row.action_info.details.arguments).map(([key, value]) => (
              <div key={key} style={{ marginLeft: '8px', display: 'flex', justifyContent: 'space-between' }}>
                <span>- {key}:</span>
                <span>{JSON.stringify(value)}</span>
              </div>
            ))}
          </>
        )}
        {row.action_info.error ? (
          <div style={{ marginTop: '4px', color: 'red', display: 'flex', justifyContent: 'space-between' }}>
            <span>Error:</span>
            <span>{row.action_info.error}</span>
          </div>
        ) : row.action_info.message && (
          <div style={{ marginTop: '4px', display: 'flex', justifyContent: 'space-between' }}>
            <span>Response:</span>
            <span>{row.action_info.message.length > 100 ? `${row.action_info.message.substring(0, 100)}...` : row.action_info.message}</span>
          </div>
        )}
      </div>
    );
  }

  const content: React.ReactNode[] = [
    <div key="times" style={{ display: 'flex', justifyContent: 'space-between' }}>
      <span>Started:</span>
      <span>{formatTime(startTime)}</span>
    </div>,
    <div key="times2" style={{ display: 'flex', justifyContent: 'space-between' }}>
      <span>Finished:</span>
      <span>{formatTime(endTime)}</span>
    </div>
  ];

  // Add token information for LLM calls
  if (row.llm_call) {
    if (row.llm_call.prompt_tokens) {
      content.push(
        <div key="prompt_tokens" style={{ display: 'flex', justifyContent: 'space-between' }}>
          <span>Prompt Tokens:</span>
          <span>{row.llm_call.prompt_tokens}</span>
        </div>
      );
    }
    if (row.llm_call.completion_tokens) {
      content.push(
        <div key="completion_tokens" style={{ display: 'flex', justifyContent: 'space-between' }}>
          <span>Completion Tokens:</span>
          <span>{row.llm_call.completion_tokens}</span>
        </div>
      );
    }
  }

  content.push(
    <div key="duration" style={{ display: 'flex', justifyContent: 'space-between' }}>
      <span>Duration:</span>
      <span>{formatMs(row.duration_ms)}</span>
    </div>
  );

  if (row.llm_call) {
    if (row.llm_call.step?.startsWith('skill_context_runner') && row.llm_call.request) {
      const reasoningEffort = getReasoningEffort(row.llm_call.request);
      content.push(
        <div key="reasoning" style={{ display: 'flex', justifyContent: 'space-between' }}>
          <span>Reasoning Effort:</span>
          <span>{reasoningEffort}</span>
        </div>
      );
    }

    if ((row.llm_call.step === 'decide_next_action' || row.llm_call.step === 'summarize_multiple_tool_results') && row.llm_call.response) {
      const toolCalls = getToolCalls(row.llm_call.response);
      if (toolCalls.length > 0) {
        content.push(
          <div key="toolcalls">
            <div style={{ fontWeight: 'bold' }}>Tool Calls:</div>
            <ul style={{ margin: 0, paddingLeft: 18 }}>
              {toolCalls.map((tc, idx) => (
                <li key={idx}>{tc.function?.name || 'Unknown'}</li>
              ))}
            </ul>
          </div>
        );
              } else {
          const message = getAssistantMessage(row.llm_call.response);
          if (message !== 'n/a') {
            content.push(
              <div key="message">
                <div style={{ fontWeight: 'bold' }}>Message:</div>
                <div style={{ marginTop: '4px' }}>{message.length > 100 ? `${message.substring(0, 100)}...` : message}</div>
              </div>
            );
          }
        }
    }
  }

  return <div style={{ minWidth: '300px' }}>{content}</div>;
};

const LLMCallTimelineChart: React.FC<LLMCallTimelineChartProps> = ({ calls, onHoverCallId, highlightedCallId, appId, interactionId }) => {
  const containerRef = useRef<HTMLDivElement>(null);
  const [containerWidth, setContainerWidth] = useState(900);
  const [hoverX, setHoverX] = useState<number | null>(null);
  const [selectedStepInfo, setSelectedStepInfo] = useState<TypesStepInfo | null>(null);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [selectedLLMCall, setSelectedLLMCall] = useState<LLMCall | null>(null);
  const [llmCallDialogOpen, setLlmCallDialogOpen] = useState(false);
  const theme = useTheme();

  const { data: steps, isLoading: isLoadingSteps } = useListAppSteps(appId, interactionId);

  useEffect(() => {
    const handleResize = () => {
      if (containerRef.current) {
        setContainerWidth(containerRef.current.offsetWidth);
      }
    };
    handleResize();
    window.addEventListener('resize', handleResize);
    return () => window.removeEventListener('resize', handleResize);
  }, []);

  const rows = useMemo(() => {
    const llmRows = calls.map((call) => ({
      name: call.step,
      created: call.created,
      duration_ms: call.duration_ms,
      llm_call: call,
    } as RowData));

    const stepRows = steps && steps.data
      ? steps.data.map((step: TypesStepInfo) => ({
          name: step.name,
          created: step.created,
          duration_ms: step.duration_ms,
          action_info: step,
        } as RowData))
      : [];

    return [...llmRows, ...stepRows];
  }, [calls, steps]);

  const chartData = useMemo(() => {
    if (!rows.length) return [];    
    
    // Sort rows by created time    
    const sorted = sortRowDataByCreated(rows);
    const baseTime = new Date(sorted[0].created).getTime();  

    // Add a row for each step
    return sorted.map((row, idx) => {
      const start = new Date(row.created).getTime() - baseTime;
      return {
        ...row,
        yOrder: idx,
        start,
        end: start + (row.duration_ms || 0),
        duration: row.duration_ms || 0,
        label: row.llm_call?.step || `Skill execution: ${row.action_info?.name}`,
      };
    });
  }, [rows]);

  const minX = 0;
  const maxX = Math.max(...chartData.map(d => d.end)) * 1.1;
  const width = containerWidth;
  const height = chartData.length * ROW_HEIGHT + CHART_PADDING * 2;

  // X axis ticks
  const numTicks = 5;
  const ticks = Array.from({ length: numTicks + 1 }, (_, i) => minX + ((maxX - minX) * i) / numTicks);

  // Chart colors
  const barColor = (row: RowData) => {
    if (row.action_info) {
      if (row.action_info.error) {
        return 'url(#barErrorGradient)';
      }
      return 'url(#barActionGradient)';
    }
    return highlightedCallId === row.llm_call?.id
      ? 'url(#barHighlightGradient)'
      : 'url(#barGradient)';
  };

  const handleStepClick = (row: RowData) => {
    if (row.action_info) {
      setSelectedStepInfo(row.action_info);
      setDialogOpen(true);
    } else if (row.llm_call) {
      setSelectedLLMCall(row.llm_call);
      setLlmCallDialogOpen(true);
    }
  };

  const handleCloseDialog = () => {
    setDialogOpen(false);
    setSelectedStepInfo(null);
  };

  const handleCloseLLMCallDialog = () => {
    setLlmCallDialogOpen(false);
    setSelectedLLMCall(null);
  };

  const [hoveredStepId, setHoveredStepId] = useState<string | null>(null);

  const handleStepHover = (row: RowData, isHovering: boolean) => {
    if (row.action_info) {
      setHoveredStepId(isHovering ? row.action_info.id || '' : null);
    }
  };

  return (
    <Box ref={containerRef} sx={{ width: '100%', mb: 2 }}>
      <Typography variant="subtitle2" sx={{ mb: 1 }}>Agent Execution Timeline</Typography>
      <Box sx={{ width: '100%', overflowX: 'auto', bgcolor: 'transparent' }}>
        <svg
          viewBox={`0 0 ${width} ${height}`}
          width={width}
          height={height}
          style={{ display: 'block', width: '100%' }}
          preserveAspectRatio="none"
          onMouseMove={(e) => {
            const rect = e.currentTarget.getBoundingClientRect();
            const x = e.clientX - rect.left;
            setHoverX(x);
          }}
          onMouseLeave={() => setHoverX(null)}
        >
          <defs>
            <linearGradient id="barGradient" x1="0" y1="0" x2="1" y2="0">
              <stop offset="0%" stopColor={theme.chartGradientStart} stopOpacity={theme.chartGradientStartOpacity} />
              <stop offset="100%" stopColor={theme.chartGradientEnd} stopOpacity={theme.chartGradientEndOpacity} />
            </linearGradient>
            <linearGradient id="barHighlightGradient" x1="0" y1="0" x2="1" y2="0">
              <stop offset="0%" stopColor={theme.chartHighlightGradientStart} stopOpacity={theme.chartHighlightGradientStartOpacity} />
              <stop offset="100%" stopColor={theme.chartHighlightGradientEnd} stopOpacity={theme.chartHighlightGradientEndOpacity} />
            </linearGradient>
            <linearGradient id="barActionGradient" x1="0" y1="0" x2="1" y2="0">
              <stop offset="0%" stopColor={theme.chartActionGradientStart} stopOpacity={theme.chartActionGradientStartOpacity} />
              <stop offset="100%" stopColor={theme.chartActionGradientEnd} stopOpacity={theme.chartActionGradientEndOpacity} />
            </linearGradient>
            <linearGradient id="barErrorGradient" x1="0" y1="0" x2="1" y2="0">
              <stop offset="0%" stopColor={theme.chartErrorGradientStart} stopOpacity={theme.chartErrorGradientStartOpacity} />
              <stop offset="100%" stopColor={theme.chartErrorGradientEnd} stopOpacity={theme.chartErrorGradientEndOpacity} />
            </linearGradient>
          </defs>
          {/* Hover line */}
          {hoverX !== null && (
            <line
              x1={hoverX}
              y1={CHART_PADDING}
              x2={hoverX}
              y2={height - CHART_PADDING}
              stroke="#888"
              strokeWidth={1}
              strokeDasharray="4"
              opacity={0.3}
              pointerEvents="none"
            />
          )}
          {/* X axis line and ticks */}
          <line
            x1={LABEL_WIDTH}
            y1={height - CHART_PADDING}
            x2={width - CHART_PADDING}
            y2={height - CHART_PADDING}
            stroke="#888"
            strokeWidth={1}
          />
          {ticks.map((tick, i) => {
            const x = LABEL_WIDTH + ((width - LABEL_WIDTH - CHART_PADDING) * (tick - minX)) / (maxX - minX);
            return (
              <g key={i}>
                <line
                  x1={x}
                  y1={height - CHART_PADDING}
                  x2={x}
                  y2={height - CHART_PADDING + 8}
                  stroke="#888"
                  strokeWidth={1}
                />
                <text
                  x={x}
                  y={height - CHART_PADDING + 16}
                  textAnchor="middle"
                  fill="#aaa"
                  fontSize={11}
                  fontFamily="inherit"
                >
                  {formatMs(Math.round(tick))}
                </text>
              </g>
            );
          })}
          {/* Bars */}
          {chartData.map((d, i) => {
            const x = LABEL_WIDTH + ((width - LABEL_WIDTH - CHART_PADDING) * (d.start - minX)) / (maxX - minX);
            const barWidth = Math.max(MIN_BAR_WIDTH, ((width - LABEL_WIDTH - CHART_PADDING) * d.duration) / (maxX - minX));
            const y = CHART_PADDING + i * ROW_HEIGHT;
            return (
              <Tooltip
                key={d.llm_call?.id || d.action_info?.id || i}
                title={getTooltipContent(d)}
                placement="top"
                arrow
                slotProps={{ tooltip: { sx: { bgcolor: '#222', opacity: 1 } } }}
              >
                <g
                  onMouseOver={(e) => {
                    e.stopPropagation();
                    onHoverCallId?.(d.llm_call?.id || null);
                    handleStepHover(d, true);
                  }}
                  onMouseOut={(e) => {
                    e.stopPropagation();
                    onHoverCallId?.(null);
                    handleStepHover(d, false);
                  }}
                  onClick={(e) => {
                    e.stopPropagation();
                    handleStepClick(d);
                  }}
                  style={{ cursor: 'pointer' }}
                >
                  <rect
                    x={x}
                    y={y}
                    width={barWidth}
                    height={BAR_HEIGHT}
                    rx={8}
                    fill={barColor(d)}
                    style={{ 
                      filter: highlightedCallId === d.llm_call?.id ? 'drop-shadow(0 0 8px #ffb300)' : 
                              (d.action_info && hoveredStepId === d.action_info.id) ? 'drop-shadow(0 0 4px rgba(255, 255, 255, 0.3))' : undefined,
                      transition: 'filter 0.2s ease-in-out',
                      opacity: d.action_info && hoveredStepId === d.action_info.id ? 0.9 : 1,
                    }}
                  />
                  <text
                    x={x + 6}
                    y={y + BAR_HEIGHT / 2 + 4}
                    fill="#fff"
                    fontSize={11}
                    fontFamily="inherit"
                    pointerEvents="none"
                  >
                    {d.label}
                  </text>
                  {d.action_info && (
                    <text
                      x={x + barWidth - 8}
                      y={y + BAR_HEIGHT / 2 + 4}
                      fill="#fff"
                      fontSize={9}
                      fontFamily="inherit"
                      pointerEvents="none"
                      textAnchor="end"
                      opacity={0.7}
                    >
                    </text>
                  )}
                  {d.llm_call && (
                    <text
                      x={x + barWidth - 8}
                      y={y + BAR_HEIGHT / 2 + 4}
                      fill="#fff"
                      fontSize={9}
                      fontFamily="inherit"
                      pointerEvents="none"
                      textAnchor="end"
                      opacity={0.7}
                    >
                    </text>
                  )}
                </g>
              </Tooltip>
            );
          })}
        </svg>
      </Box>
      
      <SkillExecutionDialog
        open={dialogOpen}
        onClose={handleCloseDialog}
        stepInfo={selectedStepInfo}
      />

      <LLMCallDialog
        open={llmCallDialogOpen}
        onClose={handleCloseLLMCallDialog}
        llmCall={selectedLLMCall}
      />
    </Box>
  );
};

export default LLMCallTimelineChart; 