import React from 'react';
import {
  Box,
  Typography,
  Chip,
  Tooltip,
  CircularProgress,
  Grid,
} from '@mui/material';
import { Copy, Check, AlertCircle, Play } from 'lucide-react';
import { useCloneGroupProgress } from '../../services/specTaskService';
import TaskCard from '../tasks/TaskCard';
import useRouter from '../../hooks/useRouter';
import { TypesSpecTaskWithProject } from '../../api/api';

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

// Map API status to TaskCard phase
const statusToPhase = (status: string): 'backlog' | 'planning' | 'review' | 'implementation' | 'pull_request' | 'completed' => {
  const mapping: Record<string, 'backlog' | 'planning' | 'review' | 'implementation' | 'pull_request' | 'completed'> = {
    backlog: 'backlog',
    queued_spec_generation: 'backlog',
    spec_generation: 'planning',
    spec_review: 'review',
    spec_revision: 'review',
    spec_approved: 'implementation',
    queued_implementation: 'implementation',
    implementation_queued: 'implementation',
    implementation: 'implementation',
    implementation_review: 'implementation',
    pull_request: 'pull_request',
    done: 'completed',
    spec_failed: 'planning',
    implementation_failed: 'implementation',
  };
  return mapping[status] || 'backlog';
};

// Transform API task to TaskCard format
const transformTaskForCard = (task: TypesSpecTaskWithProject) => ({
  id: task.id || '',
  name: task.name || '',
  status: task.status || 'backlog',
  phase: statusToPhase(task.status || 'backlog'),
  planningStatus: undefined,
  planning_session_id: task.planning_session_id,
  archived: task.archived,
  metadata: task.metadata,
  merged_to_main: task.merged_to_main,
  just_do_it_mode: task.just_do_it_mode,
  started_at: task.started_at,
  design_docs_pushed_at: task.design_docs_pushed_at,
  clone_group_id: task.clone_group_id,
  cloned_from_id: task.cloned_from_id,
  pull_request_id: task.pull_request_id,
  pull_request_url: task.pull_request_url,
  implementation_approved_at: task.implementation_approved_at,
  base_branch: task.base_branch,
  branch_name: task.branch_name,
  session_updated_at: task.session_updated_at,
  agent_work_state: task.agent_work_state as 'idle' | 'working' | 'done' | undefined,
  // Extra field for display
  _projectName: task.project_name,
  _projectId: task.project_id,
});

// Stacked bar component showing all tasks as segments
const StackedProgressBar: React.FC<{
  tasks: Array<{ task_id: string; project_name: string; name: string; status: string; project_id?: string }>;
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
                onClick={() => onTaskClick?.(task.task_id, task.project_id || '')}
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

// Full version for dialogs - uses TaskCard components
const CloneGroupProgressFull: React.FC<CloneGroupProgressProps> = ({
  groupId,
}) => {
  const { navigate } = useRouter();
  const { data: progress, isLoading, error } = useCloneGroupProgress(groupId);

  const handleTaskClick = (taskId: string, projectId: string) => {
    navigate('project-task-detail', { id: projectId, taskId });
  };

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

  // Transform full tasks for TaskCard
  const cardTasks = (progress.full_tasks || []).map(transformTaskForCard);

  return (
    <Box>
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
        onTaskClick={handleTaskClick}
      />

      {/* Task cards grid */}
      <Box sx={{ mt: 3 }}>
        <Typography variant="subtitle2" sx={{ mb: 2 }}>
          All Tasks
        </Typography>
        <Grid container spacing={2}>
          {cardTasks.map((task, index) => (
            <Grid item xs={12} sm={6} key={task.id}>
              {/* Project name header */}
              <Typography
                variant="caption"
                color="text.secondary"
                sx={{ mb: 0.5, display: 'block' }}
              >
                {(task as any)._projectName}
              </Typography>
              <TaskCard
                task={task}
                index={index}
                columns={[]} // Empty columns - read-only view
                onTaskClick={() => handleTaskClick(task.id, (task as any)._projectId)}
                projectId={(task as any)._projectId}
                showMetrics={false}
                hideCloneOption={true}
              />
            </Grid>
          ))}
        </Grid>
      </Box>
    </Box>
  );
};

export default CloneGroupProgressFull;
