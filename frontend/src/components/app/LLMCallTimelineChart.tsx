import React, { useMemo, useRef, useState, useEffect } from 'react';
import { Box, Typography, Tooltip } from '@mui/material';

interface LLMCall {
  id: string;
  created: string;
  duration_ms: number;
  step?: string;
  model?: string;
  response?: any;
}

interface LLMCallTimelineChartProps {
  calls: LLMCall[];
  onHoverCallId?: (id: string | null) => void;
  highlightedCallId?: string | null;
}

const formatMs = (ms: number) => `${ms} ms`;

const ROW_HEIGHT = 22;
const BAR_HEIGHT = 14;
const LABEL_WIDTH = 0;
const CHART_PADDING = 24;

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

const getAssistantMessage = (response: any): string => {
  const parsed = parseResponse(response);
  return parsed?.choices?.[0]?.message?.content || 'n/a';
};

const getToolCalls = (response: any): any[] => {
  const parsed = parseResponse(response);
  return parsed?.choices?.[0]?.message?.tool_calls || [];
};

const getTooltipContent = (call: LLMCall): React.ReactNode => {
  const startTime = new Date(call.created);
  const endTime = new Date(startTime.getTime() + call.duration_ms);
  
  const formatTime = (date: Date) => {
    return date.toLocaleTimeString('en-US', { 
      hour12: false,
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit'
    });
  };

  const content: React.ReactNode[] = [
    <div key="times">
      Started: {formatTime(startTime)}<br />
      Finished: {formatTime(endTime)}
    </div>
  ];

  if (call.step === 'decide_next_action' && call.response) {
    const toolCalls = getToolCalls(call.response);
    if (toolCalls.length > 0) {
      content.push(
        <div key="toolcalls">
          <div>Tool Calls:</div>
          <ul style={{ margin: 0, paddingLeft: 18 }}>
            {toolCalls.map((tc, idx) => (
              <li key={idx}>{tc.function?.name || 'Unknown'}</li>
            ))}
          </ul>
        </div>
      );
    } else {
      const message = getAssistantMessage(call.response);
      if (message !== 'n/a') {
        content.push(<div key="message">Message: {message}</div>);
      }
    }
  }

  content.push(<div key="duration">Duration: {formatMs(call.duration_ms)}</div>);

  return <div>{content}</div>;
};

const LLMCallTimelineChart: React.FC<LLMCallTimelineChartProps> = ({ calls, onHoverCallId, highlightedCallId }) => {
  const containerRef = useRef<HTMLDivElement>(null);
  const [containerWidth, setContainerWidth] = useState(900);
  const [hoverX, setHoverX] = useState<number | null>(null);

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

  const chartData = useMemo(() => {
    if (!calls.length) return [];
    const sorted = [...calls].sort((a, b) => new Date(a.created).getTime() - new Date(b.created).getTime());
    const baseTime = new Date(sorted[0].created).getTime();
    return sorted.map((call, idx) => {
      const start = new Date(call.created).getTime() - baseTime;
      return {
        ...call,
        yOrder: idx,
        start,
        end: start + (call.duration_ms || 0),
        duration: call.duration_ms || 0,
        label: call.step || `Call ${idx + 1}`,
      };
    });
  }, [calls]);

  const minX = 0;
  const maxX = Math.max(...chartData.map(d => d.end)) * 1.1;
  const width = containerWidth;
  const height = chartData.length * ROW_HEIGHT + CHART_PADDING * 2;

  // X axis ticks
  const numTicks = 5;
  const ticks = Array.from({ length: numTicks + 1 }, (_, i) => minX + ((maxX - minX) * i) / numTicks);

  // Chart colors
  const barColor = (id: string) =>
    highlightedCallId === id
      ? 'url(#barHighlightGradient)'
      : 'url(#barGradient)';

  return (
    <Box ref={containerRef} sx={{ width: '100%', mb: 2 }}>
      <Typography variant="subtitle2" sx={{ mb: 1 }}>Timeline of LLM Calls</Typography>
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
              <stop offset="0%" stopColor="#00c8ff" stopOpacity={0.8} />
              <stop offset="100%" stopColor="#6f00ff" stopOpacity={0.8} />
            </linearGradient>
            <linearGradient id="barHighlightGradient" x1="0" y1="0" x2="1" y2="0">
              <stop offset="0%" stopColor="#ffb300" stopOpacity={0.9} />
              <stop offset="100%" stopColor="#ff4081" stopOpacity={0.9} />
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
            const barWidth = ((width - LABEL_WIDTH - CHART_PADDING) * d.duration) / (maxX - minX);
            const y = CHART_PADDING + i * ROW_HEIGHT;
            return (
              <Tooltip
                key={d.id}
                title={getTooltipContent(d)}
                placement="top"
                arrow
                slotProps={{ tooltip: { sx: { bgcolor: '#222', opacity: 1 } } }}
              >
                <g
                  onMouseOver={() => onHoverCallId?.(d.id)}
                  onMouseOut={() => onHoverCallId?.(null)}
                  style={{ cursor: 'pointer' }}
                >
                  <rect
                    x={x}
                    y={y}
                    width={barWidth}
                    height={BAR_HEIGHT}
                    rx={8}
                    fill={barColor(d.id)}
                    style={{ filter: highlightedCallId === d.id ? 'drop-shadow(0 0 8px #ffb300)' : undefined }}
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
                </g>
              </Tooltip>
            );
          })}
        </svg>
      </Box>
    </Box>
  );
};

export default LLMCallTimelineChart; 