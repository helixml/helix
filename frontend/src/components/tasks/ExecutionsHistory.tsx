import React from 'react';
import {
  Box,
  Typography,
  CircularProgress,
} from '@mui/material';
import { TypesTriggerExecution, TypesTriggerExecutionStatus } from '../../api/api';
import { useListAppTriggerExecutions } from '../../services/appService';

interface ExecutionsHistoryProps {
  taskId?: string;
  taskName: string;
}

const ExecutionsHistory: React.FC<ExecutionsHistoryProps> = ({ taskId, taskName }) => {
  // Fetch trigger executions if we have a task ID
  const { data: triggerExecutions, isLoading, error: triggerExecutionsError } = useListAppTriggerExecutions(taskId || '');

  // Helper function to format date
  const formatDate = (dateString?: string) => {
    if (!dateString) return '';
    const date = new Date(dateString);
    return date.toLocaleDateString('en-US', { 
      weekday: 'short', 
      month: 'short', 
      day: 'numeric',
      year: 'numeric'
    });
  };

  // Helper function to get status color
  const getStatusColor = (status?: TypesTriggerExecutionStatus) => {
    switch (status) {
      case TypesTriggerExecutionStatus.TriggerExecutionStatusSuccess:
        return '#10B981';
      case TypesTriggerExecutionStatus.TriggerExecutionStatusError:
        return '#EF4444';
      case TypesTriggerExecutionStatus.TriggerExecutionStatusRunning:
        return '#F59E0B';
      case TypesTriggerExecutionStatus.TriggerExecutionStatusPending:
        return '#6B7280';
      default:
        return '#6B7280';
    }
  };

  // Helper function to format duration
  const formatDuration = (startTime?: string, endTime?: string) => {
    if (!startTime) return '';
    const start = new Date(startTime);
    const end = endTime ? new Date(endTime) : new Date();
    const durationMs = end.getTime() - start.getTime();
    const seconds = Math.floor(durationMs / 1000);
    const minutes = Math.floor(seconds / 60);
    const remainingSeconds = seconds % 60;
    
    if (minutes > 0) {
      return `${minutes}m ${remainingSeconds}s`;
    }
    return `${seconds}s`;
  };

  if (!taskId) {
    return null;
  }

  return (
    <Box sx={{ 
      width: 300, 
      borderLeft: '1px solid #374151',
      pl: 3,
      display: 'flex',
      flexDirection: 'column'
    }}>
      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 2 }}>
        <Typography variant="h6" sx={{ color: '#F1F1F1' }}>
          History
        </Typography>
        {triggerExecutions?.data && triggerExecutions.data.length > 0 && (
          <Typography variant="caption" sx={{ color: '#A0AEC0' }}>
            {triggerExecutions.data.length} records
          </Typography>
        )}
      </Box>
      
      {isLoading ? (
        <Box sx={{ display: 'flex', justifyContent: 'center', py: 2 }}>
          <CircularProgress size={24} />
        </Box>
      ) : triggerExecutionsError ? (
        <Typography variant="body2" sx={{ color: '#EF4444' }}>
          Failed to load executions
        </Typography>
      ) : triggerExecutions?.data && triggerExecutions.data.length > 0 ? (
        <Box>
          <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
            {triggerExecutions.data.map((execution: TypesTriggerExecution, index: number) => (
              <Box key={execution.id || index} sx={{ 
                display: 'flex', 
                alignItems: 'flex-start', 
                gap: 2,
                p: 2,
                backgroundColor: '#1F2937',
                borderRadius: 1,
                border: '1px solid #374151'
              }}>
                <Box sx={{ 
                  width: 8, 
                  height: 8, 
                  borderRadius: '50%', 
                  backgroundColor: getStatusColor(execution.status),
                  mt: 0.5,
                  flexShrink: 0
                }} />
                
                <Box sx={{ flex: 1, minWidth: 0 }}>
                  <Typography variant="body2" sx={{ 
                    color: '#F1F1F1', 
                    fontWeight: 500,
                    mb: 0.5,
                    overflow: 'hidden',
                    textOverflow: 'ellipsis',
                    whiteSpace: 'nowrap'
                  }}>
                    {taskName}
                  </Typography>
                  
                  <Typography variant="caption" sx={{ color: '#A0AEC0' }}>
                    {formatDate(execution.created)} · Ran for {formatDuration(execution.created, execution.updated)}
                  </Typography>
                </Box>
              </Box>
            ))}
          </Box>
        </Box>
      ) : (
        <Typography variant="body2" sx={{ color: '#A0AEC0', fontStyle: 'italic' }}>
          No executions yet
        </Typography>
      )}
    </Box>
  );
};

export default ExecutionsHistory; 