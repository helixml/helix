import React from 'react';
import {
  Box,
  Typography,
  Chip,
  Tooltip,
  CircularProgress,
  IconButton,
} from '@mui/material';
import { Copy, Check, Clock, AlertCircle, Play, ExternalLink } from 'lucide-react';
import { useCloneGroupProgress } from '../../services/specTaskService';

interface CloneGroupProgressProps {
  groupId: string;
  onTaskClick?: (taskId: string, projectId: string) => void;
  compact?: boolean; // For inline display in TaskCard
}

// Status colors for the stacked bar
const STATUS_COLORS: Record<string, string> = {
  done: '#10b981',
  completed: '#10b981',
  implementation: '#3b82f6',
  spec_generation: '#f59e0b',
  spec_approved: '#8b5cf6',
  spec_review: '#eab308',
  implementation_review: '#eab308',
  backlog: '#94a3b8',
  failed: '#ef4444',
  spec_failed: '#ef4444',
  implementation_failed: '#ef4444',
};

const getStatusColor = (status: string): string => {
  return STATUS_COLORS[status] || '#cbd5e1';
};

const getStatusLabel = (status: string): string => {
  const labels: Record<string, string> = {
    done: 'Done',
    completed: 'Completed',
    implementation: 'Implementing',
    spec_generation: 'Planning',
    spec_approved: 'Approved',
    spec_review: 'In Review',
    implementation_review: 'Impl. Review',
    backlog: 'Backlog',
    failed: 'Failed',
    spec_failed: 'Spec Failed',
    implementation_failed: 'Impl. Failed',
  };
  return labels[status] || status;
};

// Stacked bar component showing all tasks as segments
const StackedProgressBar: React.FC<{
  tasks: Array<{ task_id: string; project_name: string; name: string; status: string }>;
  onTaskClick?: (taskId: string, projectId: string) => void;
}> = ({ tasks, onTaskClick }) => {
  if (!tasks || tasks.length === 0) return null;

  const totalTasks = tasks.length;

  return (
    <Box sx={{ width: '100%' }}>
      {/* Stacked bar */}
      <Box
        sx={{
          display: 'flex',
          height: 24,
          borderRadius: 1,
          overflow: 'hidden',
          border: '1px solid',
          borderColor: 'divider',
        }}
      >
        {tasks.map((task, index) => {
          const widthPercent = 100 / totalTasks;
          const color = getStatusColor(task.status);

          return (
            <Tooltip
              key={task.task_id}
              title={
                <Box>
                  <Typography variant="body2" fontWeight={600}>
                    {task.project_name}
                  </Typography>
                  <Typography variant="caption" display="block">
                    {task.name}
                  </Typography>
                  <Chip
                    label={getStatusLabel(task.status)}
                    size="small"
                    sx={{
                      mt: 0.5,
                      height: 18,
                      fontSize: '0.65rem',
                      backgroundColor: color,
                      color: 'white',
                    }}
                  />
                </Box>
              }
              arrow
            >
              <Box
                onClick={() => onTaskClick?.(task.task_id, task.project_id)}
                sx={{
                  width: `${widthPercent}%`,
                  backgroundColor: color,
                  cursor: onTaskClick ? 'pointer' : 'default',
                  transition: 'all 0.15s ease',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  borderRight: index < tasks.length - 1 ? '1px solid rgba(255,255,255,0.3)' : 'none',
                  '&:hover': onTaskClick ? {
                    filter: 'brightness(1.1)',
                    transform: 'scaleY(1.1)',
                  } : {},
                }}
              >
                {/* Show icon for certain statuses */}
                {(task.status === 'done' || task.status === 'completed') && (
                  <Check size={12} color="white" />
                )}
                {(task.status === 'implementation' || task.status === 'spec_generation') && (
                  <Play size={10} color="white" />
                )}
                {(task.status === 'failed' || task.status.includes('failed')) && (
                  <AlertCircle size={10} color="white" />
                )}
              </Box>
            </Tooltip>
          );
        })}
      </Box>

      {/* Legend */}
      <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 1, mt: 1 }}>
        {Object.entries(
          tasks.reduce((acc, task) => {
            acc[task.status] = (acc[task.status] || 0) + 1;
            return acc;
          }, {} as Record<string, number>)
        ).map(([status, count]) => (
          <Box key={status} sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
            <Box
              sx={{
                width: 10,
                height: 10,
                borderRadius: 0.5,
                backgroundColor: getStatusColor(status),
              }}
            />
            <Typography variant="caption" color="text.secondary">
              {getStatusLabel(status)}: {count}
            </Typography>
          </Box>
        ))}
      </Box>
    </Box>
  );
};

// Compact version for TaskCard
export const CloneGroupProgressCompact: React.FC<CloneGroupProgressProps> = ({
  groupId,
  onTaskClick,
}) => {
  const { data: progress, isLoading, error } = useCloneGroupProgress(groupId);

  if (isLoading) {
    return (
      <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
        <CircularProgress size={14} />
        <Typography variant="caption" color="text.secondary">
          Loading...
        </Typography>
      </Box>
    );
  }

  if (error || !progress) {
    return null;
  }

  const completedCount = progress.completed_tasks || 0;
  const totalCount = progress.total_tasks || 0;

  return (
    <Box sx={{ mt: 1 }}>
      {/* Mini header */}
      <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', mb: 0.5 }}>
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
          <Copy size={12} />
          <Typography variant="caption" color="text.secondary" fontWeight={500}>
            Clone batch: {completedCount}/{totalCount}
          </Typography>
        </Box>
        <Typography variant="caption" color="text.secondary">
          {progress.progress_pct || 0}%
        </Typography>
      </Box>

      {/* Stacked bar */}
      <StackedProgressBar
        tasks={progress.tasks || []}
        onTaskClick={onTaskClick}
      />
    </Box>
  );
};

// Full version for dialogs
const CloneGroupProgressFull: React.FC<CloneGroupProgressProps> = ({
  groupId,
  onTaskClick,
}) => {
  const { data: progress, isLoading, error } = useCloneGroupProgress(groupId);

  if (isLoading) {
    return (
      <Box sx={{ p: 2, display: 'flex', justifyContent: 'center' }}>
        <CircularProgress size={24} />
      </Box>
    );
  }

  if (error || !progress) {
    return (
      <Box sx={{ p: 2 }}>
        <Typography color="error">Failed to load clone progress</Typography>
      </Box>
    );
  }

  return (
    <Box>
      {/* Header with source info */}
      <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 2 }}>
        <Copy size={20} />
        <Typography variant="h6">Clone Batch Progress</Typography>
      </Box>

      {/* Source task info */}
      {progress.source_task && (
        <Box
          sx={{
            mb: 2,
            p: 1.5,
            bgcolor: 'action.hover',
            borderRadius: 1,
            border: '1px solid',
            borderColor: 'divider',
          }}
        >
          <Typography variant="caption" color="text.secondary" display="block">
            Cloned from
          </Typography>
          <Typography variant="body2" fontWeight={500}>
            {progress.source_task.name}
          </Typography>
          <Typography variant="caption" color="text.secondary">
            {progress.source_task.project_name}
          </Typography>
        </Box>
      )}

      {/* Progress summary */}
      <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, mb: 2 }}>
        <Typography variant="body2">
          <strong>{progress.completed_tasks}</strong> / {progress.total_tasks} completed
        </Typography>
        <Chip
          label={`${progress.progress_pct || 0}%`}
          size="small"
          color={progress.progress_pct === 100 ? 'success' : 'primary'}
        />
      </Box>

      {/* Stacked bar */}
      <StackedProgressBar
        tasks={progress.tasks || []}
        onTaskClick={onTaskClick}
      />

      {/* Task list */}
      <Box sx={{ mt: 2, maxHeight: 300, overflow: 'auto' }}>
        <Typography variant="subtitle2" sx={{ mb: 1 }}>
          All Tasks
        </Typography>
        {(progress.tasks || []).map((task) => (
          <Box
            key={task.task_id}
            onClick={() => onTaskClick?.(task.task_id, task.project_id)}
            sx={{
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'space-between',
              p: 1,
              mb: 0.5,
              borderRadius: 1,
              border: '1px solid',
              borderColor: 'divider',
              cursor: onTaskClick ? 'pointer' : 'default',
              '&:hover': onTaskClick ? { bgcolor: 'action.hover' } : {},
            }}
          >
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, minWidth: 0 }}>
              <Box
                sx={{
                  width: 8,
                  height: 8,
                  borderRadius: '50%',
                  backgroundColor: getStatusColor(task.status),
                  flexShrink: 0,
                }}
              />
              <Box sx={{ minWidth: 0 }}>
                <Typography variant="body2" noWrap>
                  {task.project_name}
                </Typography>
                <Typography variant="caption" color="text.secondary" noWrap>
                  {task.name}
                </Typography>
              </Box>
            </Box>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
              <Chip
                label={getStatusLabel(task.status)}
                size="small"
                sx={{
                  height: 20,
                  fontSize: '0.65rem',
                  backgroundColor: getStatusColor(task.status),
                  color: 'white',
                }}
              />
              {onTaskClick && (
                <IconButton size="small">
                  <ExternalLink size={14} />
                </IconButton>
              )}
            </Box>
          </Box>
        ))}
      </Box>
    </Box>
  );
};

export default CloneGroupProgressFull;
