/**
 * ChartsPanel - Adaptive bitrate charts for streaming diagnostics
 */

import React from 'react';
import { Box, Typography } from '@mui/material';
import { LineChart } from '@mui/x-charts';
import {
  darkChartStyles,
  chartContainerStyles,
  chartLegendProps,
  axisLabelStyle,
} from '../common/chartStyles';

export interface ChartsPanelProps {
  throughputHistory: number[];
  rttHistory: number[];
  bitrateHistory: number[];
  frameDriftHistory: number[];
}

const ChartsPanel: React.FC<ChartsPanelProps> = ({
  throughputHistory,
  rttHistory,
  bitrateHistory,
  frameDriftHistory,
}) => {
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
              <LineChart
                xAxis={[{
                  data: throughputHistory.map((_, i) => i - throughputHistory.length + 1),
                  label: 'Seconds ago',
                  labelStyle: axisLabelStyle,
                }]}
                yAxis={[{
                  min: 0,
                  max: Math.max(Math.max(...throughputHistory), Math.max(...bitrateHistory), 10) * 1.2,
                  labelStyle: axisLabelStyle,
                }]}
                series={[
                  {
                    data: bitrateHistory,
                    label: 'Requested',
                    color: '#888',
                    showMark: false,
                    curve: 'stepAfter',
                  },
                  {
                    data: throughputHistory,
                    label: 'Actual',
                    color: '#00ff00',
                    showMark: false,
                    curve: 'linear',
                    area: true,
                  },
                ]}
                height={120}
                margin={{ left: 50, right: 10, top: 30, bottom: 25 }}
                grid={{ horizontal: true, vertical: false }}
                sx={darkChartStyles}
                slotProps={{ legend: chartLegendProps }}
              />
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
              <LineChart
                xAxis={[{
                  data: rttHistory.map((_, i) => i - rttHistory.length + 1),
                  label: 'Seconds ago',
                  labelStyle: axisLabelStyle,
                }]}
                yAxis={[{
                  min: 0,
                  max: Math.max(Math.max(...rttHistory), 100) * 1.2,
                  labelStyle: axisLabelStyle,
                }]}
                series={[
                  {
                    data: rttHistory.map(() => 150), // Threshold line at 150ms
                    label: 'High Latency Threshold',
                    color: '#ff9800',
                    showMark: false,
                    curve: 'linear',
                  },
                  {
                    data: rttHistory,
                    label: 'RTT',
                    color: rttHistory[rttHistory.length - 1] > 150 ? '#ff6b6b' : '#00c8ff',
                    showMark: false,
                    curve: 'linear',
                    area: true,
                  },
                ]}
                height={120}
                margin={{ left: 50, right: 10, top: 30, bottom: 25 }}
                grid={{ horizontal: true, vertical: false }}
                sx={darkChartStyles}
                slotProps={{ legend: chartLegendProps }}
              />
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
              <LineChart
                xAxis={[{
                  data: frameDriftHistory.map((_, i) => i - frameDriftHistory.length + 1),
                  label: 'Seconds ago',
                  labelStyle: axisLabelStyle,
                }]}
                yAxis={[{
                  min: Math.min(Math.min(...frameDriftHistory), -100) * 1.2,
                  max: Math.max(Math.max(...frameDriftHistory), 300) * 1.2,
                  labelStyle: axisLabelStyle,
                }]}
                series={[
                  {
                    data: frameDriftHistory.map(() => 200), // Threshold line at 200ms
                    label: 'Reduction Threshold',
                    color: '#ff6b6b',
                    showMark: false,
                    curve: 'linear',
                  },
                  {
                    data: frameDriftHistory.map(() => 0), // Zero line
                    label: 'On Time',
                    color: '#4caf50',
                    showMark: false,
                    curve: 'linear',
                  },
                  {
                    data: frameDriftHistory,
                    label: 'Frame Drift',
                    color: frameDriftHistory[frameDriftHistory.length - 1] > 200 ? '#ff6b6b' : '#00c8ff',
                    showMark: false,
                    curve: 'linear',
                    area: true,
                  },
                ]}
                height={120}
                margin={{ left: 50, right: 10, top: 30, bottom: 25 }}
                grid={{ horizontal: true, vertical: false }}
                sx={darkChartStyles}
                slotProps={{ legend: chartLegendProps }}
              />
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
