import React, { useState, useEffect } from 'react';
import {
  Box,
  Typography,
  Card,
  CardContent,
  CardHeader,
  Grid,
  Button,
  Chip,
  LinearProgress,
  CircularProgress,
  Alert,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  TextField,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Paper,
  IconButton,
  Collapse,
  Stack,
  Divider,
  Badge,
  Tooltip,
  List,
  ListItem,
  ListItemText,
  ListItemIcon,
} from '@mui/material';
import {
  ExpandMore as ExpandMoreIcon,
  ExpandLess as ExpandLessIcon,
  PlayArrow as PlayIcon,
  Stop as StopIcon,
  Refresh as RefreshIcon,
  Add as AddIcon,
  Timeline as TimelineIcon,
  AccountTree as TreeIcon,
  Computer as ZedIcon,
  History as HistoryIcon,
  Check as CheckIcon,
  Error as ErrorIcon,
  Warning as WarningIcon,
  Pending as PendingIcon,
} from '@mui/icons-material';

import {
  useMultiSessionOverview,
  useSpecTaskWorkSessions,
  useCoordinationEvents,
  useZedInstanceStatus,
  useCreateImplementationSessions,
  useSpecTaskRealTimeUpdates,
  getSessionStatusColor,
  getSpecTaskStatusColor,
  formatTimestamp,
  type SpecTask,
  type WorkSession,

  type ZedInstanceStatus,
  type MultiSessionOverview,
  type ImplementationSessionsCreateRequest,
} from '../../services/specTaskService';

import ExternalAgentControl from './ExternalAgentControl';

interface MultiSessionDashboardProps {
  taskId: string;
  compact?: boolean;
  showControls?: boolean;
  onSessionSelect?: (sessionId: string) => void;
}

const MultiSessionDashboard: React.FC<MultiSessionDashboardProps> = ({
  taskId,
  compact = false,
  showControls = true,
  onSessionSelect,
}) => {
  const [expanded, setExpanded] = useState<Record<string, boolean>>({});
  const [createDialogOpen, setCreateDialogOpen] = useState(false);
  const [newSessionConfig, setNewSessionConfig] = useState({
    projectPath: '',
    autoCreateSessions: true,
    workspaceConfig: {},
  });

  // Data hooks
  const { data: overview, isLoading: overviewLoading, error: overviewError } = useMultiSessionOverview(taskId);
  const { data: workSessions, isLoading: sessionsLoading } = useSpecTaskWorkSessions(taskId);
  const { data: coordinationLog, isLoading: coordinationLoading } = useCoordinationEvents(taskId);
  const { data: zedStatus, isLoading: zedLoading } = useZedInstanceStatus(taskId);

  // Real-time updates
  const realTimeData = useSpecTaskRealTimeUpdates(taskId);

  // Mutations
  const createImplementationSessions = useCreateImplementationSessions();

  const isLoading = overviewLoading || sessionsLoading || coordinationLoading || zedLoading;
  const hasError = overviewError;

  const handleExpandToggle = (section: string) => {
    setExpanded(prev => ({ ...prev, [section]: !prev[section] }));
  };

  const handleCreateSessions = async () => {
    try {
      await createImplementationSessions.mutateAsync({
        taskId,
        request: {
          ...newSessionConfig,
          spec_task_id: taskId,
        } as ImplementationSessionsCreateRequest,
      });
      setCreateDialogOpen(false);
      setNewSessionConfig({
        projectPath: '',
        autoCreateSessions: true,
        workspaceConfig: {},
      });
    } catch (error) {
      console.error('Failed to create implementation sessions:', error);
    }
  };

  const getStatusIcon = (status: string) => {
    switch (status) {
      case 'active':
        return <PlayIcon color="success" />;
      case 'completed':
        return <CheckIcon color="primary" />;
      case 'failed':
      case 'cancelled':
        return <ErrorIcon color="error" />;
      case 'blocked':
        return <WarningIcon color="warning" />;
      default:
        return <PendingIcon color="action" />;
    }
  };

  const renderOverviewCard = () => (
    <Card>
      <CardHeader
        title="Multi-Session Overview"
        action={
          showControls && (
            <Button
              variant="outlined"
              startIcon={<AddIcon />}
              onClick={() => setCreateDialogOpen(true)}
              disabled={createImplementationSessions.isPending}
            >
              Create Sessions
            </Button>
          )
        }
      />
      <CardContent>
        {overview ? (
          <Grid container spacing={2}>
            <Grid item xs={12} sm={6} md={3}>
              <Box textAlign="center">
                <Typography variant="h4" color="primary">
                  {overview.work_session_count || 0}
                </Typography>
                <Typography variant="body2" color="textSecondary">
                  Total Sessions
                </Typography>
              </Box>
            </Grid>
            <Grid item xs={12} sm={6} md={3}>
              <Box textAlign="center">
                <Typography variant="h4" color="success.main">
                  {overview.active_sessions || 0}
                </Typography>
                <Typography variant="body2" color="textSecondary">
                  Active
                </Typography>
              </Box>
            </Grid>
            <Grid item xs={12} sm={6} md={3}>
              <Box textAlign="center">
                <Typography variant="h4" color="info.main">
                  {overview.completed_sessions || 0}
                </Typography>
                <Typography variant="body2" color="textSecondary">
                  Completed
                </Typography>
              </Box>
            </Grid>
            <Grid item xs={12} sm={6} md={3}>
              <Box textAlign="center">
                <Typography variant="h4" color="warning.main">
                  {overview.zed_thread_count || 0}
                </Typography>
                <Typography variant="body2" color="textSecondary">
                  Zed Threads
                </Typography>
              </Box>
            </Grid>
            {overview.last_activity && (
              <Grid item xs={12}>
                <Typography variant="body2" color="textSecondary">
                  Last Activity: {formatTimestamp(overview.last_activity)}
                </Typography>
              </Grid>
            )}
          </Grid>
        ) : (
          <Typography variant="body2" color="textSecondary">
            No overview data available
          </Typography>
        )}
      </CardContent>
    </Card>
  );

  const renderWorkSessionsTable = () => (
    <Card>
      <CardHeader
        title="Work Sessions"
        action={
          <IconButton onClick={() => handleExpandToggle('sessions')}>
            {expanded.sessions ? <ExpandLessIcon /> : <ExpandMoreIcon />}
          </IconButton>
        }
      />
      <Collapse in={expanded.sessions} timeout="auto" unmountOnExit>
        <CardContent>
          <TableContainer component={Paper} variant="outlined">
            <Table size="small">
              <TableHead>
                <TableRow>
                  <TableCell>Name</TableCell>
                  <TableCell>Phase</TableCell>
                  <TableCell>Status</TableCell>
                  <TableCell>Task</TableCell>
                  <TableCell>Created</TableCell>
                  <TableCell>Actions</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {workSessions?.work_sessions?.map((session: any) => (
                  <TableRow key={session.id} hover>
                    <TableCell>
                      <Box display="flex" alignItems="center">
                        {getStatusIcon(session.status)}
                        <Box ml={1}>
                          <Typography variant="body2" fontWeight="medium">
                            {session.name}
                          </Typography>
                          {session.description && (
                            <Typography variant="caption" color="textSecondary">
                              {session.description}
                            </Typography>
                          )}
                        </Box>
                      </Box>
                    </TableCell>
                    <TableCell>
                      <Chip
                        label={session.phase}
                        size="small"
                        variant="outlined"
                      />
                    </TableCell>
                    <TableCell>
                      <Chip
                        label={session.status}
                        size="small"
                        color={getSessionStatusColor(session.status)}
                      />
                    </TableCell>
                    <TableCell>
                      {session.implementation_task_title && (
                        <Typography variant="body2">
                          #{session.implementation_task_index}: {session.implementation_task_title}
                        </Typography>
                      )}
                    </TableCell>
                    <TableCell>
                      <Typography variant="body2">
                        {formatTimestamp(session.created_at)}
                      </Typography>
                    </TableCell>
                    <TableCell>
                      <Button
                        size="small"
                        onClick={() => onSessionSelect?.(session.id)}
                      >
                        View
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </TableContainer>
        </CardContent>
      </Collapse>
    </Card>
  );

  const renderCoordinationLog = () => (
    <Card>
      <CardHeader
        title={
          <Box display="flex" alignItems="center">
            <TimelineIcon sx={{ mr: 1 }} />
            Coordination Events
            {coordinationLog?.events && (
              <Badge
                badgeContent={coordinationLog.events.length}
                color="primary"
                sx={{ ml: 1 }}
              />
            )}
          </Box>
        }
        action={
          <IconButton onClick={() => handleExpandToggle('coordination')}>
            {expanded.coordination ? <ExpandLessIcon /> : <ExpandMoreIcon />}
          </IconButton>
        }
      />
      <Collapse in={expanded.coordination} timeout="auto" unmountOnExit>
        <CardContent>
          <List dense>
            {coordinationLog?.events?.slice(0, 10).map((event: any, index: number) => (
              <React.Fragment key={event.id || index}>
                <ListItem>
                  <ListItemIcon>
                    {event.event_type === 'handoff' && <TreeIcon />}
                    {event.event_type === 'blocking' && <WarningIcon color="warning" />}
                    {event.event_type === 'completion' && <CheckIcon color="success" />}
                    {!['handoff', 'blocking', 'completion'].includes(event.event_type) && (
                      <HistoryIcon />
                    )}
                  </ListItemIcon>
                  <ListItemText
                    primary={
                      <Box display="flex" alignItems="center">
                        <Typography variant="body2" fontWeight="medium">
                          {event.event_type}
                        </Typography>
                        <Chip
                          label={event.acknowledged ? 'Acknowledged' : 'Pending'}
                          size="small"
                          color={event.acknowledged ? 'success' : 'default'}
                          variant="outlined"
                          sx={{ ml: 1 }}
                        />
                      </Box>
                    }
                    secondary={
                      <Box>
                        <Typography variant="body2">
                          {event.message}
                        </Typography>
                        <Typography variant="caption" color="textSecondary">
                          {formatTimestamp(event.timestamp)} • From: {event.from_session_id}
                          {event.to_session_id && ` → To: ${event.to_session_id}`}
                        </Typography>
                      </Box>
                    }
                  />
                </ListItem>
                {index < (coordinationLog?.events?.length || 0) - 1 && <Divider />}
              </React.Fragment>
            ))}
            {!coordinationLog?.events?.length && (
              <ListItem>
                <ListItemText
                  primary="No coordination events"
                  secondary="Events will appear here as sessions coordinate"
                />
              </ListItem>
            )}
          </List>
        </CardContent>
      </Collapse>
    </Card>
  );

  const renderZedStatus = () => (
    <Card>
      <CardHeader
        title={
          <Box display="flex" alignItems="center">
            <ZedIcon sx={{ mr: 1 }} />
            Zed Instance Status
          </Box>
        }
        action={
          <IconButton onClick={() => handleExpandToggle('zed')}>
            {expanded.zed ? <ExpandLessIcon /> : <ExpandMoreIcon />}
          </IconButton>
        }
      />
      <Collapse in={expanded.zed} timeout="auto" unmountOnExit>
        <CardContent>
          {zedStatus ? (
            <Grid container spacing={2}>
              <Grid item xs={12} sm={6}>
                <Typography variant="body2" color="textSecondary">
                  Instance ID:
                </Typography>
                <Typography variant="body1" fontFamily="monospace">
                  {zedStatus.zed_instance_id || 'Not assigned'}
                </Typography>
              </Grid>
              <Grid item xs={12} sm={6}>
                <Typography variant="body2" color="textSecondary">
                  Status:
                </Typography>
                <Chip
                  label={zedStatus.status}
                  color={zedStatus.status === 'active' ? 'success' : 'default'}
                />
              </Grid>
              <Grid item xs={12} sm={6}>
                <Typography variant="body2" color="textSecondary">
                  Thread Count:
                </Typography>
                <Typography variant="body1">
                  {zedStatus.active_threads}/{zedStatus.thread_count}
                </Typography>
              </Grid>
              <Grid item xs={12} sm={6}>
                <Typography variant="body2" color="textSecondary">
                  Project Path:
                </Typography>
                <Typography variant="body1" fontFamily="monospace">
                  {zedStatus.project_path || 'Not specified'}
                </Typography>
              </Grid>
              {zedStatus.last_activity && (
                <Grid item xs={12}>
                  <Typography variant="body2" color="textSecondary">
                    Last Activity: {formatTimestamp(zedStatus.last_activity)}
                  </Typography>
                </Grid>
              )}
            </Grid>
          ) : (
            <Typography variant="body2" color="textSecondary">
              No Zed instance associated with this task
            </Typography>
          )}
        </CardContent>
      </Collapse>
    </Card>
  );

  const renderCreateSessionDialog = () => (
    <Dialog open={createDialogOpen} onClose={() => setCreateDialogOpen(false)} maxWidth="sm" fullWidth>
      <DialogTitle>Create Implementation Sessions</DialogTitle>
      <DialogContent>
        <Stack spacing={3} sx={{ mt: 1 }}>
          <TextField
            label="Project Path"
            value={newSessionConfig.projectPath}
            onChange={(e) => setNewSessionConfig(prev => ({ ...prev, projectPath: e.target.value }))}
            fullWidth
            helperText="Path to the project directory for the implementation"
          />
          <Box>
            <Typography variant="body2" color="textSecondary">
              This will create work sessions based on the approved implementation plan.
              Sessions will be automatically coordinated and can spawn additional sub-sessions as needed.
            </Typography>
          </Box>
        </Stack>
      </DialogContent>
      <DialogActions>
        <Button onClick={() => setCreateDialogOpen(false)}>
          Cancel
        </Button>
        <Button
          onClick={handleCreateSessions}
          variant="contained"
          disabled={createImplementationSessions.isPending}
        >
          {createImplementationSessions.isPending ? <CircularProgress size={20} /> : 'Create Sessions'}
        </Button>
      </DialogActions>
    </Dialog>
  );

  if (isLoading) {
    return (
      <Box display="flex" justifyContent="center" alignItems="center" minHeight={200}>
        <CircularProgress />
      </Box>
    );
  }

  if (hasError) {
    return (
      <Alert severity="error">
        Failed to load multi-session dashboard data. Please try refreshing the page.
      </Alert>
    );
  }

  return (
    <Box>
      <Grid container spacing={3}>
        <Grid item xs={12}>
          {renderOverviewCard()}
        </Grid>

        {/* NEW: External Agent Control */}
        <Grid item xs={12}>
          <ExternalAgentControl specTaskId={taskId} />
        </Grid>

        <Grid item xs={12} lg={8}>
          <Stack spacing={3}>
            {renderWorkSessionsTable()}
            {renderCoordinationLog()}
          </Stack>
        </Grid>

        <Grid item xs={12} lg={4}>
          {renderZedStatus()}
        </Grid>

        {realTimeData.isLoading && (
          <Grid item xs={12}>
            <Alert severity="info" icon={<RefreshIcon />}>
              Connecting to real-time updates...
            </Alert>
          </Grid>
        )}
      </Grid>

      {renderCreateSessionDialog()}
    </Box>
  );
};

export default MultiSessionDashboard;