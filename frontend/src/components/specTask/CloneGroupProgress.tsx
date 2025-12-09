import React from 'react';
import {
  Box,
  Typography,
  LinearProgress,
  Chip,
  Card,
  CardContent,
  Grid,
  Tooltip,
} from '@mui/material';
import { Copy, Check, Clock, AlertCircle, Play } from 'lucide-react';
import { useCloneGroupProgress, CloneGroupProgress as CloneGroupProgressType } from '../../services/specTaskService';

interface CloneGroupProgressProps {
  groupId: string;
  onTaskClick?: (taskId: string, projectId: string) => void;
}

const getStatusIcon = (status: string) => {
  switch (status) {
    case 'done':
    case 'completed':
      return <Check size={14} color="green" />;
    case 'implementation':
    case 'spec_generation':
      return <Play size={14} color="blue" />;
    case 'backlog':
      return <Clock size={14} color="gray" />;
    case 'failed':
    case 'spec_failed':
    case 'implementation_failed':
      return <AlertCircle size={14} color="red" />;
    default:
      return <Clock size={14} color="gray" />;
  }
};

const getStatusColor = (status: string): 'success' | 'primary' | 'warning' | 'error' | 'default' => {
  switch (status) {
    case 'done':
    case 'completed':
      return 'success';
    case 'implementation':
    case 'spec_generation':
    case 'spec_approved':
      return 'primary';
    case 'spec_review':
    case 'implementation_review':
      return 'warning';
    case 'failed':
    case 'spec_failed':
    case 'implementation_failed':
      return 'error';
    default:
      return 'default';
  }
};

const CloneGroupProgressComponent: React.FC<CloneGroupProgressProps> = ({
  groupId,
  onTaskClick,
}) => {
  const { data: progress, isLoading, error } = useCloneGroupProgress(groupId);

  if (isLoading) {
    return (
      <Box sx={{ p: 2 }}>
        <LinearProgress />
        <Typography variant="body2" color="text.secondary" sx={{ mt: 1 }}>
          Loading progress...
        </Typography>
      </Box>
    );
  }

  if (error || !progress) {
    return (
      <Box sx={{ p: 2 }}>
        <Typography color="error">Failed to load progress</Typography>
      </Box>
    );
  }

  return (
    <Card variant="outlined">
      <CardContent>
        {/* Header */}
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 2 }}>
          <Copy size={20} />
          <Typography variant="h6">Clone Progress</Typography>
        </Box>

        {/* Source Task */}
        {progress.source_task && (
          <Box sx={{ mb: 2, p: 1.5, bgcolor: 'action.hover', borderRadius: 1 }}>
            <Typography variant="caption" color="text.secondary">
              Source Task
            </Typography>
            <Typography variant="body2" fontWeight={500}>
              {progress.source_task.name}
            </Typography>
            <Typography variant="caption" color="text.secondary">
              Project: {progress.source_task.project_name}
            </Typography>
          </Box>
        )}

        {/* Progress Bar */}
        <Box sx={{ mb: 2 }}>
          <Box sx={{ display: 'flex', justifyContent: 'space-between', mb: 0.5 }}>
            <Typography variant="body2">
              {progress.completed_tasks} / {progress.total_tasks} completed
            </Typography>
            <Typography variant="body2" fontWeight={500}>
              {progress.progress_pct}%
            </Typography>
          </Box>
          <LinearProgress
            variant="determinate"
            value={progress.progress_pct || 0}
            sx={{ height: 8, borderRadius: 1 }}
          />
        </Box>

        {/* Status Breakdown */}
        {progress.status_breakdown && Object.keys(progress.status_breakdown).length > 0 && (
          <Box sx={{ mb: 2, display: 'flex', flexWrap: 'wrap', gap: 0.5 }}>
            {Object.entries(progress.status_breakdown).map(([status, count]) => (
              <Chip
                key={status}
                label={`${status}: ${count}`}
                size="small"
                color={getStatusColor(status)}
                variant="outlined"
              />
            ))}
          </Box>
        )}

        {/* Task List */}
        {progress.tasks && progress.tasks.length > 0 && (
          <Box sx={{ maxHeight: 300, overflow: 'auto' }}>
            <Grid container spacing={1}>
              {progress.tasks.map((task) => (
                <Grid item xs={12} key={task.task_id}>
                  <Box
                    onClick={() => onTaskClick?.(task.task_id!, task.project_id!)}
                    sx={{
                      display: 'flex',
                      alignItems: 'center',
                      justifyContent: 'space-between',
                      p: 1,
                      borderRadius: 1,
                      border: '1px solid',
                      borderColor: 'divider',
                      cursor: onTaskClick ? 'pointer' : 'default',
                      '&:hover': onTaskClick ? {
                        bgcolor: 'action.hover',
                      } : {},
                    }}
                  >
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, minWidth: 0 }}>
                      {getStatusIcon(task.status!)}
                      <Box sx={{ minWidth: 0 }}>
                        <Typography variant="body2" noWrap>
                          {task.name}
                        </Typography>
                        <Typography variant="caption" color="text.secondary" noWrap>
                          {task.project_name}
                        </Typography>
                      </Box>
                    </Box>
                    <Tooltip title={task.status}>
                      <Chip
                        label={task.status}
                        size="small"
                        color={getStatusColor(task.status!)}
                        variant="outlined"
                      />
                    </Tooltip>
                  </Box>
                </Grid>
              ))}
            </Grid>
          </Box>
        )}
      </CardContent>
    </Card>
  );
};

export default CloneGroupProgressComponent;
