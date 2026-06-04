/**
 * ChartsPanel - Adaptive bitrate charts for streaming diagnostics
 */

import React from 'react';
import { Box, Typography } from '@mui/material';
import {
  Area,
  AreaChart,
  CartesianGrid,
  Legend,
  Line,
  ResponsiveContainer,
  XAxis,
  YAxis,
} from 'recharts';
import {
  chartContainerStyles,
} from '../common/chartStyles';

export interface ChartsPanelProps {
  throughputHistory: number[];
  rttHistory: number[];
  bitrateHistory: number[];
  frameDriftHistory: number[];
}

const AXIS_TICK = { fontSize: 10, fill: 'rgba(255, 255, 255, 0.6)' };
const AXIS_LABEL = { fontSize: 10, fill: 'rgba(255, 255, 255, 0.6)' };
const LEGEND_TEXT = { color: 'rgba(255, 255, 255, 0.8)', fontSize: 12 };
const GRID_STROKE = 'rgba(255, 255, 255, 0.1)';

interface SeriesPoint {
  t: number;
  [k: string]: number;
}

const buildData = (lengths: number[][], keys: string[]): SeriesPoint[] => {
  const len = Math.max(...lengths.map((arr) => arr.length));
  return Array.from({ length: len }, (_, i) => {
    const point: SeriesPoint = { t: i - len + 1 };
    lengths.forEach((arr, j) => {
      point[keys[j]] = arr[i];
    });
    return point;
  });
};

const ChartsPanel: React.FC<ChartsPanelProps> = ({
  throughputHistory,
  rttHistory,
  bitrateHistory,
  frameDriftHistory,
}) => {
  const throughputData = buildData(
    [throughputHistory, bitrateHistory],
    ['actual', 'requested'],
  );

  const rttData = rttHistory.map((v, i) => ({
    t: i - rttHistory.length + 1,
    rtt: v,
    threshold: 150,
  }));
  const rttColor = rttHistory.length > 0 && rttHistory[rttHistory.length - 1] > 150 ? '#ff6b6b' : '#00c8ff';

  const driftData = frameDriftHistory.map((v, i) => ({
    t: i - frameDriftHistory.length + 1,
    drift: v,
    threshold: 200,
    onTime: 0,
  }));
  const driftColor = frameDriftHistory.length > 0 && frameDriftHistory[frameDriftHistory.length - 1] > 200 ? '#ff6b6b' : '#00c8ff';

  return (
    <Box
      sx={{
        position: 'absolute',
        bottom: 60,
        left: 10,
        right: 10,
        backgroundColor: 'rgba(0, 0, 0, 0.95)',
        borderRadius: 2,
        border: '1px solid rgba(0, 255, 0, 0.3)',
        zIndex: 1500,
        p: 2,
        maxHeight: '40%',
        overflow: 'auto',
      }}
    >
      <Typography variant="caption" sx={{ fontWeight: 'bold', display: 'block', mb: 2, color: '#00ff00' }}>
        Adaptive Bitrate Charts (60s history)
      </Typography>

      <Box sx={{ display: 'flex', gap: 2, flexWrap: 'wrap' }}>
        {/* Throughput vs Requested Bitrate Chart */}
        <Box sx={{ flex: '1 1 400px', minWidth: 300 }}>
          <Typography variant="caption" sx={{ color: '#888', display: 'block', mb: 1 }}>
            Throughput vs Requested Bitrate (Mbps)
          </Typography>
          <Box sx={{ height: 150, ...chartContainerStyles }}>
            {throughputHistory.length > 1 ? (
              <ResponsiveContainer width="100%" height="100%">
                <AreaChart data={throughputData} margin={{ left: 0, right: 10, top: 20, bottom: 5 }}>
                  <CartesianGrid stroke={GRID_STROKE} strokeDasharray="3 3" vertical={false} />
                  <XAxis
                    dataKey="t"
                    type="number"
                    domain={['dataMin', 'dataMax']}
                    tick={AXIS_TICK}
                    stroke="rgba(255,255,255,0.6)"
                    label={{ value: 'Seconds ago', position: 'insideBottom', offset: -2, style: AXIS_LABEL }}
                  />
                  <YAxis tick={AXIS_TICK} stroke="rgba(255,255,255,0.6)" />
                  <Legend wrapperStyle={LEGEND_TEXT} iconSize={10} />
                  <Line
                    dataKey="requested"
                    name="Requested"
                    type="stepAfter"
                    stroke="#888"
                    dot={false}
                    isAnimationActive={false}
                  />
                  <Area
                    dataKey="actual"
                    name="Actual"
                    type="linear"
                    stroke="#00ff00"
                    fill="#00ff00"
                    fillOpacity={0.2}
                    dot={false}
                    isAnimationActive={false}
                  />
                </AreaChart>
              </ResponsiveContainer>
            ) : (
              <Box sx={{ height: '100%', display: 'flex', alignItems: 'center', justifyContent: 'center', color: '#666' }}>
                Collecting data...
              </Box>
            )}
          </Box>
        </Box>

        {/* RTT Chart */}
        <Box sx={{ flex: '1 1 400px', minWidth: 300 }}>
          <Typography variant="caption" sx={{ color: '#888', display: 'block', mb: 1 }}>
            Round-Trip Time (ms) - Spikes indicate congestion
          </Typography>
          <Box sx={{ height: 150, ...chartContainerStyles }}>
            {rttHistory.length > 1 ? (
              <ResponsiveContainer width="100%" height="100%">
                <AreaChart data={rttData} margin={{ left: 0, right: 10, top: 20, bottom: 5 }}>
                  <CartesianGrid stroke={GRID_STROKE} strokeDasharray="3 3" vertical={false} />
                  <XAxis
                    dataKey="t"
                    type="number"
                    domain={['dataMin', 'dataMax']}
                    tick={AXIS_TICK}
                    stroke="rgba(255,255,255,0.6)"
                    label={{ value: 'Seconds ago', position: 'insideBottom', offset: -2, style: AXIS_LABEL }}
                  />
                  <YAxis tick={AXIS_TICK} stroke="rgba(255,255,255,0.6)" />
                  <Legend wrapperStyle={LEGEND_TEXT} iconSize={10} />
                  <Line
                    dataKey="threshold"
                    name="High Latency Threshold"
                    stroke="#ff9800"
                    dot={false}
                    isAnimationActive={false}
                  />
                  <Area
                    dataKey="rtt"
                    name="RTT"
                    stroke={rttColor}
                    fill={rttColor}
                    fillOpacity={0.2}
                    dot={false}
                    isAnimationActive={false}
                  />
                </AreaChart>
              </ResponsiveContainer>
            ) : (
              <Box sx={{ height: '100%', display: 'flex', alignItems: 'center', justifyContent: 'center', color: '#666' }}>
                Collecting data...
              </Box>
            )}
          </Box>
        </Box>

        {/* Frame Drift Chart */}
        <Box sx={{ flex: '1 1 400px', minWidth: 300 }}>
          <Typography variant="caption" sx={{ color: '#888', display: 'block', mb: 1 }}>
            Frame Drift (ms) - Positive = behind, triggers bitrate reduction
          </Typography>
          <Box sx={{ height: 150, ...chartContainerStyles }}>
            {frameDriftHistory.length > 1 ? (
              <ResponsiveContainer width="100%" height="100%">
                <AreaChart data={driftData} margin={{ left: 0, right: 10, top: 20, bottom: 5 }}>
                  <CartesianGrid stroke={GRID_STROKE} strokeDasharray="3 3" vertical={false} />
                  <XAxis
                    dataKey="t"
                    type="number"
                    domain={['dataMin', 'dataMax']}
                    tick={AXIS_TICK}
                    stroke="rgba(255,255,255,0.6)"
                    label={{ value: 'Seconds ago', position: 'insideBottom', offset: -2, style: AXIS_LABEL }}
                  />
                  <YAxis tick={AXIS_TICK} stroke="rgba(255,255,255,0.6)" />
                  <Legend wrapperStyle={LEGEND_TEXT} iconSize={10} />
                  <Line dataKey="threshold" name="Reduction Threshold" stroke="#ff6b6b" dot={false} isAnimationActive={false} />
                  <Line dataKey="onTime" name="On Time" stroke="#4caf50" dot={false} isAnimationActive={false} />
                  <Area
                    dataKey="drift"
                    name="Frame Drift"
                    stroke={driftColor}
                    fill={driftColor}
                    fillOpacity={0.2}
                    dot={false}
                    isAnimationActive={false}
                  />
                </AreaChart>
              </ResponsiveContainer>
            ) : (
              <Box sx={{ height: '100%', display: 'flex', alignItems: 'center', justifyContent: 'center', color: '#666' }}>
                Collecting data...
              </Box>
            )}
          </Box>
        </Box>
      </Box>
    </Box>
  );
};

export default ChartsPanel;
