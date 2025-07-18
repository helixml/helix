import React, { useState, useEffect } from 'react';
import {
  Box,
  Typography,
  CircularProgress,
} from '@mui/material';
import { TypesTriggerExecution, TypesTriggerExecutionStatus } from '../../api/api';
import { useListAppTriggerExecutions } from '../../services/appService';
import useAccount from '../../hooks/useAccount';

interface ExecutionsHistoryProps {
  taskId?: string;
  taskName: string;
}

const ExecutionsHistory: React.FC<ExecutionsHistoryProps> = ({ taskId, taskName }) => {
  const account = useAccount()
  const [currentTime, setCurrentTime] = useState(new Date());

  // Update current time every second for running executions
  useEffect(() => {
    const interval = setInterval(() => {
      setCurrentTime(new Date());
    }, 1000);

    return () => clearInterval(interval);
  }, []);

  // Fetch trigger executions if we have a task ID - load last 100 items
  const { data: triggerExecutions, isLoading, error: triggerExecutionsError } = useListAppTriggerExecutions(taskId || '', { limit: 100 });

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
  const formatDuration = (startTime?: string, endTime?: string, status?: TypesTriggerExecutionStatus) => {
    if (!startTime) return '';
    const start = new Date(startTime);
    const end = status === TypesTriggerExecutionStatus.TriggerExecutionStatusRunning 
      ? currentTime 
      : endTime ? new Date(endTime) : new Date();
    const durationMs = end.getTime() - start.getTime();
    const seconds = Math.floor(durationMs / 1000);
    const minutes = Math.floor(seconds / 60);
    const remainingSeconds = seconds % 60;
    
    if (minutes > 0) {
      return `${minutes}m ${remainingSeconds}s`;
    }
    return `${seconds}s`;
  };

  // Helper function to get duration text
  const getDurationText = (execution: TypesTriggerExecution) => {
    const duration = formatDuration(execution.created, execution.updated, execution.status);
    
    if (execution.status === TypesTriggerExecutionStatus.TriggerExecutionStatusRunning) {
      return `Running for ${duration}`;
    } else {
      return `Ran for ${duration}`;
    }
  };

  // Helper function to handle execution click
  const handleExecutionClick = (execution: TypesTriggerExecution) => {
    if (execution.session_id) {
      // Open session in new tab
      let sessionUrl = `/session/${execution.session_id}`;

      // if we are in an organization, need to have the prefix with /org/<org name>
      if (account.organizationTools.organization) {
        sessionUrl = `/org/${account.organizationTools.organization.name}/${sessionUrl}`;
      }

      window.open(sessionUrl, '_blank');
    }
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
      flexDirection: 'column',
      height: '100%'
    }}>
      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 2, flexShrink: 0 }}>
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
        <Box sx={{ display: 'flex', justifyContent: 'center', py: 2, flexShrink: 0 }}>
          <CircularProgress size={24} />
        </Box>
      ) : triggerExecutionsError ? (
        <Typography variant="body2" sx={{ color: '#EF4444', flexShrink: 0 }}>
          Failed to load executions
        </Typography>
      ) : triggerExecutions?.data && triggerExecutions.data.length > 0 ? (
        <Box sx={{ 
          flex: 1, 
          overflow: 'hidden',
          display: 'flex',
          flexDirection: 'column'
        }}>
          <Box sx={{ 
            display: 'flex', 
            flexDirection: 'column', 
            gap: 2,
            overflowY: 'auto',
            height: '400px',
            '&::-webkit-scrollbar': {
              width: '6px',
            },
            '&::-webkit-scrollbar-track': {
              background: '#374151',
              borderRadius: '3px',
            },
            '&::-webkit-scrollbar-thumb': {
              background: '#6B7280',
              borderRadius: '3px',
              '&:hover': {
                background: '#9CA3AF',
              },
            },
          }}>
            {triggerExecutions.data.map((execution: TypesTriggerExecution, index: number) => (
              <Box 
                key={execution.id || index} 
                sx={{ 
                  display: 'flex', 
                  alignItems: 'flex-start', 
                  gap: 2,
                  p: 2,
                  borderRadius: 1,
                  cursor: execution.session_id ? 'pointer' : 'default',
                  '&:hover': execution.session_id ? {
                    backgroundColor: '#374151',
                    transition: 'background-color 0.2s ease'
                  } : {},
                  transition: 'background-color 0.2s ease',
                  flexShrink: 0
                }}
                onClick={() => handleExecutionClick(execution)}
              >
                <Box sx={{ 
                  width: 8, 
                  height: 8, 
                  borderRadius: '50%', 
                  backgroundColor: getStatusColor(execution.status),
                  mt: 0.9,
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
                    {formatDate(execution.created)} · {getDurationText(execution)}
                  </Typography>
                </Box>
              </Box>
            ))}
          </Box>
        </Box>
      ) : (
        <Typography variant="body2" sx={{ color: '#A0AEC0', fontStyle: 'italic', flexShrink: 0 }}>
          No executions yet
        </Typography>
      )}
    </Box>
  );
};

export default ExecutionsHistory; 