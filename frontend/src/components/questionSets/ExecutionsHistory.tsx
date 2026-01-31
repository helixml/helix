import React, { useState, useEffect } from 'react';
import {
  Box,
  Typography,
  CircularProgress,
} from '@mui/material';
import { TypesQuestionSetExecution, TypesQuestionSetExecutionStatus } from '../../api/api';
import { useListQuestionSetExecutions } from '../../services/questionSetsService';
import useAccount from '../../hooks/useAccount';

interface ExecutionsHistoryProps {
  questionSetId?: string;
  questionSetName: string;
}

const ExecutionsHistory: React.FC<ExecutionsHistoryProps> = ({ questionSetId, questionSetName }) => {
  const account = useAccount()
  const [currentTime, setCurrentTime] = useState(new Date());

  useEffect(() => {
    const interval = setInterval(() => {
      setCurrentTime(new Date());
    }, 1000);

    return () => clearInterval(interval);
  }, []);

  const { data: executions, isLoading, error: executionsError } = useListQuestionSetExecutions(
    questionSetId || '',
    { limit: 100 }
  );

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

  const getStatusColor = (status?: TypesQuestionSetExecutionStatus) => {
    switch (status) {
      case TypesQuestionSetExecutionStatus.QuestionSetExecutionStatusSuccess:
        return '#10B981';
      case TypesQuestionSetExecutionStatus.QuestionSetExecutionStatusError:
        return '#EF4444';
      case TypesQuestionSetExecutionStatus.QuestionSetExecutionStatusRunning:
        return '#F59E0B';
      case TypesQuestionSetExecutionStatus.QuestionSetExecutionStatusPending:
        return '#6B7280';
      default:
        return '#6B7280';
    }
  };

  const formatDuration = (startTime?: string, endTime?: string, status?: TypesQuestionSetExecutionStatus) => {
    if (!startTime) return '';
    const start = new Date(startTime);
    const end = status === TypesQuestionSetExecutionStatus.QuestionSetExecutionStatusRunning 
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

  const getDurationText = (execution: TypesQuestionSetExecution) => {
    const duration = formatDuration(execution.created, execution.updated, execution.status);
    
    if (execution.status === TypesQuestionSetExecutionStatus.QuestionSetExecutionStatusRunning) {
      return `Running for ${duration}`;
    } else {
      return `Ran for ${duration}`;
    }
  };

  const handleExecutionClick = (execution: TypesQuestionSetExecution) => {
    if (execution.id && questionSetId) {
      account.orgNavigate('qa-results', { 
        question_set_id: questionSetId, 
        execution_id: execution.id 
      });
    }
  };

  if (!questionSetId) {
    return null;
  }

  const executionsList = Array.isArray(executions) ? executions : [];

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
        {executionsList.length > 0 && (
          <Typography variant="caption" sx={{ color: '#A0AEC0' }}>
            {executionsList.length} records
          </Typography>
        )}
      </Box>
      
      {isLoading ? (
        <Box sx={{ display: 'flex', justifyContent: 'center', py: 2, flexShrink: 0 }}>
          <CircularProgress size={24} />
        </Box>
      ) : executionsError ? (
        <Typography variant="body2" sx={{ color: '#EF4444', flexShrink: 0 }}>
          Failed to load executions
        </Typography>
      ) : executionsList.length > 0 ? (
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
            {executionsList.map((execution: TypesQuestionSetExecution, index: number) => {
              const isClickable = !!execution.id;

              return (
                <Box 
                  key={execution.id || index} 
                  sx={{ 
                    display: 'flex', 
                    alignItems: 'flex-start', 
                    gap: 2,
                    p: 2,
                    borderRadius: 1,
                    cursor: isClickable ? 'pointer' : 'default',
                    '&:hover': isClickable ? {
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
                      {questionSetName}
                    </Typography>
                    
                    <Typography variant="caption" sx={{ color: '#A0AEC0' }}>
                      {formatDate(execution.created)} · {getDurationText(execution)}
                    </Typography>
                  </Box>
                </Box>
              );
            })}
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

