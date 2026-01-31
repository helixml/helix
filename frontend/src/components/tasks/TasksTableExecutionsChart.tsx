import React, { useState, useEffect } from 'react';
import { Box, Tooltip } from '@mui/material';
import { TypesTriggerExecution, TypesTriggerExecutionStatus } from '../../api/api';
import { useListAppTriggerExecutions } from '../../services/appService';
import useTheme from '@mui/material/styles/useTheme';

interface TasksTableExecutionsChartProps {
  taskId: string;
}

const EXECUTION_LIMIT = 20

const TasksTableExecutionsChart: React.FC<TasksTableExecutionsChartProps> = ({ taskId }) => {
  const theme = useTheme();
  
  // Fetch last 50 executions for this task
  const { data: triggerExecutions, isLoading } = useListAppTriggerExecutions(taskId, { limit: EXECUTION_LIMIT });

  // Helper function to get status color
  const getStatusColor = (status?: TypesTriggerExecutionStatus) => {
    switch (status) {
      case TypesTriggerExecutionStatus.TriggerExecutionStatusSuccess:
        return '#10B981'; // Green for success
      case TypesTriggerExecutionStatus.TriggerExecutionStatusError:
        return '#EF4444'; // Red for error
      case TypesTriggerExecutionStatus.TriggerExecutionStatusRunning:
        return '#F59E0B'; // Orange for running
      case TypesTriggerExecutionStatus.TriggerExecutionStatusPending:
        return '#6B7280'; // Gray for pending
      default:
        return '#6B7280';
    }
  };

  // Helper function to format date and time
  const formatDateTime = (dateString?: string) => {
    if (!dateString) return '';
    const date = new Date(dateString);
    return date.toLocaleString('en-US', { 
      month: 'short', 
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
      hour12: true
    });
  };

  // Helper function to format duration
  const formatDuration = (durationMs?: number) => {
    if (!durationMs) return '0s';
    const seconds = Math.floor(durationMs / 1000);
    const minutes = Math.floor(seconds / 60);
    const remainingSeconds = seconds % 60;
    
    if (minutes > 0) {
      return `${minutes}m ${remainingSeconds}s`;
    }
    return `${seconds}s`;
  };

  // Process execution data for the chart
  const chartData = React.useMemo(() => {
    if (!triggerExecutions?.data || triggerExecutions.data.length === 0) {
      return { values: [], labels: [], executions: [] };
    }

    // Sort executions by creation date (oldest first) so newest appear on the right
    const sortedExecutions = [...triggerExecutions.data].sort((a, b) => {
      const dateA = new Date(a.created || '').getTime();
      const dateB = new Date(b.created || '').getTime();
      return dateA - dateB;
    });

    // Take the last 20 executions for better visualization
    const recentExecutions = sortedExecutions.slice(-20);
    
    const values = recentExecutions.map(execution => execution.duration_ms || 0);
    const labels = recentExecutions.map((execution, index) => {
      const date = new Date(execution.created || '');
      return date.toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
    });

    return { values, labels, executions: recentExecutions };
  }, [triggerExecutions?.data]);

  if (isLoading) {
    return (
      <Box sx={{ width: 200, height: 50, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
        <Box sx={{ width: 20, height: 20, borderRadius: '50%', border: '2px solid #6B7280', borderTop: '2px solid transparent', animation: 'spin 1s linear infinite' }} />
      </Box>
    );
  }

  if (!chartData.values.length) {
    return (
      <Box sx={{ width: 200, height: 50, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
        <Box sx={{ color: '#6B7280', fontSize: '12px' }}>No executions</Box>
      </Box>
    );
  }

  return (
    <Box sx={{ width: 200, height: 50 }}>
      <svg width="200" height="50" style={{ overflow: 'visible' }}>
        {chartData.values.map((value, index) => {
          const maxValue = Math.max(...chartData.values);
          const barHeight = maxValue > 0 ? (value / maxValue) * 40 : 0;
          const barWidth = 8;
          const barSpacing = 2;
          const x = index * (barWidth + barSpacing) + 10;
          const y = 45 - barHeight;
          
          // Get color based on execution status
          const execution = chartData.executions[index];
          const color = execution ? getStatusColor(execution.status) : '#6B7280';
          
          // Create tooltip content
          const tooltipContent = execution ? (
            <div>
              <div><strong>Date:</strong> {formatDateTime(execution.created)}</div>
              <div><strong>Duration:</strong> {formatDuration(execution.duration_ms)}</div>
              <div><strong>Status:</strong> {execution.status}</div>
            </div>
          ) : 'No data';
          
          return (
            <Tooltip
              key={index}
              title={tooltipContent}
              placement="top"
              arrow
            >
              <rect
                x={x}
                y={y}
                width={barWidth}
                height={barHeight}
                fill={color}
                rx="2"
                opacity={0.8}
                style={{ cursor: 'pointer' }}
              />
            </Tooltip>
          );
        })}
      </svg>
    </Box>
  );
};

export default TasksTableExecutionsChart;
