import React, { useState } from 'react';
import {
  Box,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Paper,
  Chip,
  Button,
  IconButton,
  Typography,
  LinearProgress,
  Collapse,
  Grid,
  Card,
  CardContent,
  Badge,
  Tooltip,
  Menu,
  MenuItem,
  Alert,
  CircularProgress,
  Stack,
} from '@mui/material';
import {
  ExpandMore as ExpandMoreIcon,
  ExpandLess as ExpandLessIcon,
  PlayArrow as PlayIcon,
  Visibility as ViewIcon,
  Computer as ZedIcon,
  Timeline as TimelineIcon,
  MoreVert as MoreIcon,
  CheckCircle as CheckIcon,
  Warning as WarningIcon,
  Error as ErrorIcon,
  Pending as PendingIcon,
} from '@mui/icons-material';

import {
  useMultiSessionOverview,
  useZedInstanceStatus,
  useUpdateSpecTaskStatus,
  useApproveSpecTask,
  getSpecTaskStatusColor,
  formatTimestamp,
  type SpecTask,
} from '../../services/specTaskService';

interface SpecTaskTableProps {
  tasks: SpecTask[];
  loading?: boolean;
  onTaskSelect?: (task: SpecTask) => void;
  onRefresh?: () => void;
  compact?: boolean;
}

interface ExpandedRowProps {
  task: SpecTask;
  onClose: () => void;
}

const ExpandedRow: React.FC<ExpandedRowProps> = ({ task, onClose }) => {
  const { data: overview, isLoading: overviewLoading } = useMultiSessionOverview(task.id || '');
  const { data: zedStatus, isLoading: zedLoading } = useZedInstanceStatus(task.id || '');

  const isLoading = overviewLoading || zedLoading;

  if (isLoading) {
    return (
      <TableRow>
        <TableCell colSpan={8}>
          <Box display="flex" justifyContent="center" py={2}>
            <CircularProgress size={24} />
          </Box>
        </TableCell>
      </TableRow>
    );
  }

  return (
    <TableRow>
      <TableCell colSpan={8} sx={{ p: 0 }}>
        <Collapse in timeout="auto" unmountOnExit>
          <Box sx={{ p: 3, bgcolor: 'grey.50' }}>
            <Grid container spacing={3}>
              {/* Multi-Session Overview */}
              <Grid item xs={12} md={4}>
                <Card variant="outlined" sx={{ height: '100%' }}>
                  <CardContent>
                    <Typography variant="subtitle2" gutterBottom sx={{ display: 'flex', alignItems: 'center' }}>
                      <TimelineIcon sx={{ mr: 1, fontSize: 16 }} />
                      Session Overview
                    </Typography>
                    {overview ? (
                      <Stack spacing={1}>
                        <Box display="flex" justifyContent="space-between">
                          <Typography variant="body2">Total Sessions:</Typography>
                          <Typography variant="body2" fontWeight="medium">
                            {overview.work_session_count || 0}
                          </Typography>
                        </Box>
                        <Box display="flex" justifyContent="space-between">
                          <Typography variant="body2">Active:</Typography>
                          <Typography variant="body2" color="success.main" fontWeight="medium">
                            {overview.active_sessions || 0}
                          </Typography>
                        </Box>
                        <Box display="flex" justifyContent="space-between">
                          <Typography variant="body2">Completed:</Typography>
                          <Typography variant="body2" color="info.main" fontWeight="medium">
                            {overview.completed_sessions || 0}
                          </Typography>
                        </Box>
                        {(overview?.work_session_count || 0) > 0 && (
                          <Box>
                            <Typography variant="body2" sx={{ mb: 0.5 }}>
                              Progress: {Math.round(((overview?.completed_sessions || 0) / (overview?.work_session_count || 1)) * 100)}%
                            </Typography>
                            <LinearProgress
                              variant="determinate"
                              value={((overview?.completed_sessions || 0) / (overview?.work_session_count || 1)) * 100}
                              sx={{ height: 6, borderRadius: 3 }}
                            />
                          </Box>
                        )}
                      </Stack>
                    ) : (
                      <Typography variant="body2" color="text.secondary">
                        No session data available
                      </Typography>
                    )}
                  </CardContent>
                </Card>
              </Grid>

              {/* Zed Integration Status */}
              <Grid item xs={12} md={4}>
                <Card variant="outlined" sx={{ height: '100%' }}>
                  <CardContent>
                    <Typography variant="subtitle2" gutterBottom sx={{ display: 'flex', alignItems: 'center' }}>
                      <ZedIcon sx={{ mr: 1, fontSize: 16 }} />
                      Zed Integration
                    </Typography>
                    {zedStatus ? (
                      <Stack spacing={1}>
                        <Box display="flex" alignItems="center" gap={1}>
                          <Badge color="success" variant="dot">
                            <Typography variant="body2">Instance Active</Typography>
                          </Badge>
                        </Box>
                        <Box display="flex" justifyContent="space-between">
                          <Typography variant="body2">Threads:</Typography>
                          <Typography variant="body2" fontWeight="medium">
                            {zedStatus.active_threads}/{zedStatus.thread_count}
                          </Typography>
                        </Box>
                        {zedStatus.last_activity && (
                          <Typography variant="caption" color="text.secondary">
                            Last activity: {formatTimestamp(zedStatus.last_activity)}
                          </Typography>
                        )}
                        <Button
                          size="small"
                          variant="outlined"
                          startIcon={<ZedIcon />}
                          onClick={() => window.open(`/zed/${zedStatus.zed_instance_id}`, '_blank')}
                          disabled={!zedStatus.zed_instance_id}
                        >
                          Open Zed
                        </Button>
                      </Stack>
                    ) : (
                      <Typography variant="body2" color="text.secondary">
                        No Zed integration
                      </Typography>
                    )}
                  </CardContent>
                </Card>
              </Grid>

              {/* Quick Actions */}
              <Grid item xs={12} md={4}>
                <Card variant="outlined" sx={{ height: '100%' }}>
                  <CardContent>
                    <Typography variant="subtitle2" gutterBottom>
                      Quick Actions
                    </Typography>
                    <Stack spacing={1}>
                      <Button
                        size="small"
                        variant="outlined"
                        startIcon={<ViewIcon />}
                        onClick={() => window.open(`/spec-tasks/${task.id}`, '_blank')}
                      >
                        View Details
                      </Button>
                      {task.project_path && (
                        <Button
                          size="small"
                          variant="outlined"
                          onClick={() => window.open(`/git/${task.id}/specs`, '_blank')}
                        >
                          View in Git
                        </Button>
                      )}
                      <Button
                        size="small"
                        variant="outlined"
                        onClick={onClose}
                      >
                        Close Details
                      </Button>
                    </Stack>
                  </CardContent>
                </Card>
              </Grid>
            </Grid>
          </Box>
        </Collapse>
      </TableCell>
    </TableRow>
  );
};

const SpecTaskRow: React.FC<{
  task: SpecTask;
  expanded: boolean;
  onToggleExpand: () => void;
  onTaskSelect?: (task: SpecTask) => void;
}> = ({ task, expanded, onToggleExpand, onTaskSelect }) => {
  const [anchorEl, setAnchorEl] = useState<null | HTMLElement>(null);
  const updateStatus = useUpdateSpecTaskStatus();
  const approveTask = useApproveSpecTask();

  const handleMenuOpen = (event: React.MouseEvent<HTMLElement>) => {
    setAnchorEl(event.currentTarget);
  };

  const handleMenuClose = () => {
    setAnchorEl(null);
  };

  const handleStatusUpdate = async (newStatus: string) => {
    try {
      await updateStatus.mutateAsync({ taskId: task.id || '', status: newStatus });
      handleMenuClose();
    } catch (error) {
      console.error('Failed to update status:', error);
    }
  };

  const handleApprove = async () => {
    try {
      await approveTask.mutateAsync(task.id || '');
      handleMenuClose();
    } catch (error) {
      console.error('Failed to approve task:', error);
    }
  };

  const getStatusIcon = (status: string) => {
    switch (status) {
      case 'active':
      case 'implementing':
        return <PlayIcon color="success" fontSize="small" />;
      case 'completed':
        return <CheckIcon color="primary" fontSize="small" />;
      case 'failed':
      case 'cancelled':
        return <ErrorIcon color="error" fontSize="small" />;
      case 'blocked':
      case 'pending_approval':
        return <WarningIcon color="warning" fontSize="small" />;
      default:
        return <PendingIcon color="action" fontSize="small" />;
    }
  };

  const getPriorityColor = (priority: string) => {
    switch (priority) {
      case 'critical':
        return 'error';
      case 'high':
        return 'warning';
      case 'medium':
        return 'info';
      case 'low':
        return 'default';
      default:
        return 'default';
    }
  };

  return (
    <>
      <TableRow hover>
        <TableCell>
          <IconButton size="small" onClick={onToggleExpand}>
            {expanded ? <ExpandLessIcon /> : <ExpandMoreIcon />}
          </IconButton>
        </TableCell>
        
        <TableCell>
          <Box>
            <Typography variant="body2" fontWeight="medium" noWrap>
              {task.name}
            </Typography>
            <Typography variant="caption" color="text.secondary" noWrap>
              {task.description}
            </Typography>
          </Box>
        </TableCell>

        <TableCell>
          <Stack direction="row" spacing={1}>
            <Chip
              label={task.type}
              size="small"
              variant="outlined"
              color="primary"
            />
            <Chip
              label={task.priority}
              size="small"
              color={getPriorityColor(task.priority || 'medium') as any}
            />
          </Stack>
        </TableCell>

        <TableCell>
          <Box display="flex" alignItems="center" gap={1}>
            {getStatusIcon(task.status || 'draft')}
            <Chip
              label={(task.status || 'draft').replace('_', ' ')}
              size="small"
              color={getSpecTaskStatusColor(task.status || 'draft') as any}
            />
          </Box>
        </TableCell>

        <TableCell>
          <Typography variant="body2">
            {task.created_by}
          </Typography>
        </TableCell>

        <TableCell>
          <Typography variant="body2">
            {formatTimestamp(task.created_at)}
          </Typography>
        </TableCell>

        <TableCell>
          {task.completed_at ? (
            <Typography variant="body2">
              {formatTimestamp(task.completed_at)}
            </Typography>
          ) : (
            <Typography variant="body2" color="text.secondary">
              In progress
            </Typography>
          )}
        </TableCell>

        <TableCell>
          <Box display="flex" alignItems="center" gap={1}>
            <Button
              size="small"
              onClick={() => onTaskSelect?.(task)}
            >
              View
            </Button>
            <IconButton size="small" onClick={handleMenuOpen}>
              <MoreIcon />
            </IconButton>
          </Box>
        </TableCell>
      </TableRow>

      {expanded && <ExpandedRow task={task} onClose={onToggleExpand} />}

      <Menu
        anchorEl={anchorEl}
        open={Boolean(anchorEl)}
        onClose={handleMenuClose}
      >
        {task.status === 'pending_approval' && (
          <MenuItem onClick={handleApprove} disabled={approveTask.isPending}>
            <CheckIcon sx={{ mr: 1 }} fontSize="small" />
            Approve Task
          </MenuItem>
        )}
        {task.status === 'draft' && (
          <MenuItem onClick={() => handleStatusUpdate('active')}>
            <PlayIcon sx={{ mr: 1 }} fontSize="small" />
            Start Task
          </MenuItem>
        )}
        {['active', 'implementing'].includes(task.status || '') && (
          <MenuItem onClick={() => handleStatusUpdate('completed')}>
            <CheckIcon sx={{ mr: 1 }} fontSize="small" />
            Mark Complete
          </MenuItem>
        )}
        <MenuItem onClick={() => window.open(`/spec-tasks/${task.id}`, '_blank')}>
          <ViewIcon sx={{ mr: 1 }} fontSize="small" />
          View Details
        </MenuItem>
      </Menu>
    </>
  );
};

const SpecTaskTable: React.FC<SpecTaskTableProps> = ({
  tasks,
  loading = false,
  onTaskSelect,
  onRefresh,
  compact = false,
}) => {
  const [expandedRows, setExpandedRows] = useState<Set<string>>(new Set());

  const handleToggleExpand = (taskId: string) => {
    setExpandedRows(prev => {
      const newSet = new Set(prev);
      if (newSet.has(taskId)) {
        newSet.delete(taskId);
      } else {
        newSet.add(taskId);
      }
      return newSet;
    });
  };

  const getSummaryStats = () => {
    const total = tasks.length;
    const active = tasks.filter(t => ['active', 'implementing'].includes(t.status || '')).length;
    const completed = tasks.filter(t => t.status === 'done' || t.status === 'completed').length;
    const pendingApproval = tasks.filter(t => t.status === 'pending_approval').length;

    return { total, active, completed, pendingApproval };
  };

  const stats = getSummaryStats();

  if (loading) {
    return (
      <Box display="flex" justifyContent="center" py={4}>
        <CircularProgress />
      </Box>
    );
  }

  if (tasks.length === 0) {
    return (
      <Alert severity="info">
        No SpecTasks found. Create a new SpecTask to get started with spec-driven development.
      </Alert>
    );
  }

  return (
    <Box>
      {/* Summary Stats */}
      {!compact && (
        <Grid container spacing={2} sx={{ mb: 3 }}>
          <Grid item xs={6} sm={3}>
            <Card variant="outlined">
              <CardContent sx={{ textAlign: 'center', py: 2 }}>
                <Typography variant="h4" color="primary">
                  {stats.total}
                </Typography>
                <Typography variant="body2" color="text.secondary">
                  Total Tasks
                </Typography>
              </CardContent>
            </Card>
          </Grid>
          <Grid item xs={6} sm={3}>
            <Card variant="outlined">
              <CardContent sx={{ textAlign: 'center', py: 2 }}>
                <Typography variant="h4" color="success.main">
                  {stats.active}
                </Typography>
                <Typography variant="body2" color="text.secondary">
                  Active
                </Typography>
              </CardContent>
            </Card>
          </Grid>
          <Grid item xs={6} sm={3}>
            <Card variant="outlined">
              <CardContent sx={{ textAlign: 'center', py: 2 }}>
                <Typography variant="h4" color="info.main">
                  {stats.completed}
                </Typography>
                <Typography variant="body2" color="text.secondary">
                  Completed
                </Typography>
              </CardContent>
            </Card>
          </Grid>
          <Grid item xs={6} sm={3}>
            <Card variant="outlined">
              <CardContent sx={{ textAlign: 'center', py: 2 }}>
                <Typography variant="h4" color="warning.main">
                  {stats.pendingApproval}
                </Typography>
                <Typography variant="body2" color="text.secondary">
                  Pending Approval
                </Typography>
              </CardContent>
            </Card>
          </Grid>
        </Grid>
      )}

      {/* Table */}
      <TableContainer component={Paper} variant="outlined">
        <Table size={compact ? "small" : "medium"}>
          <TableHead>
            <TableRow>
              <TableCell width={48}></TableCell>
              <TableCell>Task</TableCell>
              <TableCell>Type & Priority</TableCell>
              <TableCell>Status</TableCell>
              <TableCell>Created By</TableCell>
              <TableCell>Created</TableCell>
              <TableCell>Completed</TableCell>
              <TableCell>Actions</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {tasks.map((task) => (
              <SpecTaskRow
                key={task.id}
                task={task}
                expanded={expandedRows.has(task.id || '')}
                onToggleExpand={() => handleToggleExpand(task.id || '')}
                onTaskSelect={onTaskSelect}
              />
            ))}
          </TableBody>
        </Table>
      </TableContainer>
    </Box>
  );
};

export default SpecTaskTable;