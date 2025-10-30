import React, { FC } from 'react';
import {
  Box,
  Card,
  CardContent,
  CardHeader,
  Button,
  Typography,
  Chip,
  Alert,
  AlertTitle,
  Stack,
  LinearProgress,
  CircularProgress,
  Tooltip,
  IconButton,
} from '@mui/material';
import {
  PlayArrow as StartIcon,
  Stop as StopIcon,
  Refresh as RefreshIcon,
  Computer as AgentIcon,
  Warning as WarningIcon,
  CheckCircle as CheckIcon,
  Error as ErrorIcon,
} from '@mui/icons-material';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import useApi from '../../hooks/useApi';
import useSnackbar from '../../hooks/useSnackbar';

interface ExternalAgentControlProps {
  specTaskId: string;
}

interface ExternalAgentStatus {
  exists: boolean;
  external_agent_id?: string;
  status?: string;
  wolf_app_id?: string;
  workspace_dir?: string;
  helix_session_ids?: string[];
  session_count?: number;
  created?: string;
  last_activity?: string;
  idle_minutes?: number;
  will_terminate_in?: number;
  warning_threshold?: boolean;
}

const ExternalAgentControl: FC<ExternalAgentControlProps> = ({ specTaskId }) => {
  const api = useApi();
  const snackbar = useSnackbar();
  const queryClient = useQueryClient();

  // Fetch external agent status
  const { data: agentStatus, isLoading, error } = useQuery<ExternalAgentStatus>({
    queryKey: ['spec-task-external-agent-status', specTaskId],
    queryFn: async () => {
      const response = await fetch(`/api/v1/spec-tasks/${specTaskId}/external-agent/status`, {
        headers: {
          'Authorization': `Bearer ${api.getToken()}`,
        },
      });
      if (!response.ok) {
        throw new Error('Failed to fetch external agent status');
      }
      return response.json();
    },
    refetchInterval: 30000, // Refetch every 30 seconds for idle time tracking
    enabled: !!specTaskId,
  });

  // Start external agent mutation
  const startMutation = useMutation({
    mutationFn: async () => {
      const response = await fetch(`/api/v1/spec-tasks/${specTaskId}/external-agent/start`, {
        method: 'POST',
        headers: {
          'Authorization': `Bearer ${api.getToken()}`,
        },
      });
      if (!response.ok) {
        const error = await response.json();
        throw new Error(error.message || 'Failed to start external agent');
      }
      return response.json();
    },
    onSuccess: (data) => {
      snackbar.success('External agent started successfully!');
      queryClient.invalidateQueries({ queryKey: ['spec-task-external-agent-status', specTaskId] });
    },
    onError: (error: Error) => {
      snackbar.error(`Failed to start external agent: ${error.message}`);
    },
  });

  // Stop external agent mutation
  const stopMutation = useMutation({
    mutationFn: async () => {
      const response = await fetch(`/api/v1/spec-tasks/${specTaskId}/external-agent/stop`, {
        method: 'POST',
        headers: {
          'Authorization': `Bearer ${api.getToken()}`,
        },
      });
      if (!response.ok) {
        const error = await response.json();
        throw new Error(error.message || 'Failed to stop external agent');
      }
      return response.json();
    },
    onSuccess: (data) => {
      snackbar.success('External agent stopped successfully. Workspace preserved.');
      queryClient.invalidateQueries({ queryKey: ['spec-task-external-agent-status', specTaskId] });
    },
    onError: (error: Error) => {
      snackbar.error(`Failed to stop external agent: ${error.message}`);
    },
  });

  if (isLoading) {
    return (
      <Card>
        <CardContent sx={{ display: 'flex', justifyContent: 'center', py: 3 }}>
          <CircularProgress size={24} />
        </CardContent>
      </Card>
    );
  }

  if (!agentStatus?.exists) {
    return (
      <Card>
        <CardHeader
          avatar={<AgentIcon />}
          title="External Agent"
          subheader="Not created yet"
        />
        <CardContent>
          <Alert severity="info">
            External agent will be created automatically when this SpecTask enters planning phase.
          </Alert>
        </CardContent>
      </Card>
    );
  }

  const isRunning = agentStatus.status === 'running';
  const isStopped = agentStatus.status === 'stopped' || agentStatus.status === 'terminated';
  const idleMinutes = agentStatus.idle_minutes || 0;
  const showWarning = agentStatus.warning_threshold;
  const willTerminateIn = agentStatus.will_terminate_in || 0;

  return (
    <Card>
      <CardHeader
        avatar={<AgentIcon color={isRunning ? 'success' : 'disabled'} />}
        title={
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
            <Typography variant="h6">External Agent</Typography>
            <Chip
              label={agentStatus.status}
              color={isRunning ? 'success' : 'default'}
              size="small"
              icon={isRunning ? <CheckIcon /> : isStopped ? <StopIcon /> : <ErrorIcon />}
            />
          </Box>
        }
        subheader={`${agentStatus.session_count || 0} Helix sessions • ${idleMinutes}min idle`}
        action={
          <Stack direction="row" spacing={1}>
            <Tooltip title="Refresh status">
              <IconButton
                onClick={() => queryClient.invalidateQueries({ queryKey: ['spec-task-external-agent-status', specTaskId] })}
                size="small"
              >
                <RefreshIcon />
              </IconButton>
            </Tooltip>
            {isRunning ? (
              <Button
                variant="outlined"
                color="error"
                startIcon={stopMutation.isPending ? <CircularProgress size={16} /> : <StopIcon />}
                onClick={() => stopMutation.mutate()}
                disabled={stopMutation.isPending}
                size="small"
              >
                Stop Agent
              </Button>
            ) : (
              <Button
                variant="contained"
                color="success"
                startIcon={startMutation.isPending ? <CircularProgress size={16} /> : <StartIcon />}
                onClick={() => startMutation.mutate()}
                disabled={startMutation.isPending}
                size="small"
              >
                Start Agent
              </Button>
            )}
          </Stack>
        }
      />
      <CardContent>
        <Stack spacing={2}>
          {/* Idle warning banner */}
          {isRunning && showWarning && (
            <Alert severity="warning" icon={<WarningIcon />}>
              <AlertTitle>Idle Session Warning</AlertTitle>
              This external agent has been idle for {idleMinutes} minutes.
              It will be automatically terminated in {willTerminateIn} minutes to free GPU resources.
              <br /><strong>Send a message to any session to keep the agent alive.</strong>
            </Alert>
          )}

          {/* Status information */}
          <Box>
            <Typography variant="body2" color="text.secondary" gutterBottom>
              <strong>Agent ID:</strong> {agentStatus.external_agent_id}
            </Typography>
            <Typography variant="body2" color="text.secondary" gutterBottom>
              <strong>Workspace:</strong> {agentStatus.workspace_dir}
            </Typography>
            <Typography variant="body2" color="text.secondary" gutterBottom>
              <strong>Wolf App:</strong> {agentStatus.wolf_app_id}
            </Typography>
            {agentStatus.helix_session_ids && agentStatus.helix_session_ids.length > 0 && (
              <Typography variant="body2" color="text.secondary">
                <strong>Sessions:</strong> {agentStatus.helix_session_ids.join(', ')}
              </Typography>
            )}
          </Box>

          {/* Info about state persistence */}
          {isStopped && (
            <Alert severity="info">
              <AlertTitle>Workspace Preserved</AlertTitle>
              The external agent container is stopped, but all work is preserved in the filestore:
              <ul style={{ marginTop: 8, marginBottom: 0 }}>
                <li>All git repositories and commits</li>
                <li>All Zed threads and conversations</li>
                <li>All extensions and settings</li>
                <li>Design documents in helix-design-docs branch</li>
              </ul>
              Click <strong>Start Agent</strong> to resume with exact same state.
            </Alert>
          )}

          {/* Running status with idle progress bar */}
          {isRunning && (
            <Box>
              <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', mb: 1 }}>
                <Typography variant="caption" color="text.secondary">
                  Idle Time
                </Typography>
                <Typography variant="caption" color={showWarning ? 'warning.main' : 'text.secondary'}>
                  {idleMinutes} / 30 minutes
                </Typography>
              </Box>
              <LinearProgress
                variant="determinate"
                value={(idleMinutes / 30) * 100}
                color={showWarning ? 'warning' : 'primary'}
              />
            </Box>
          )}
        </Stack>
      </CardContent>
    </Card>
  );
};

export default ExternalAgentControl;
