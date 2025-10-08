import React, { FC, useState, useEffect } from 'react';
import {
  Box,
  Grid,
  Card,
  CardHeader,
  CardContent,
  Typography,
  Chip,
  LinearProgress,
  Alert,
  IconButton,
  useTheme,
} from '@mui/material';
import {
  Refresh as RefreshIcon,
  CheckCircle as CheckCircleIcon,
  RadioButtonUnchecked as RadioButtonUncheckedIcon,
} from '@mui/icons-material';
import useApi from '../../hooks/useApi';

// Types matching backend API
interface AgentProgressItem {
  agent_id: string;
  task_id: string;
  task_name: string;
  current_task: TaskItemDTO | null;
  tasks_before: TaskItemDTO[];
  tasks_after: TaskItemDTO[];
  last_update: string;
  phase: string;
}

interface TaskItemDTO {
  index: number;
  description: string;
  status: string;
}

interface LiveAgentFleetProgressResponse {
  agents: AgentProgressItem[];
  timestamp: string;
}

const LiveAgentFleetDashboard: FC = () => {
  const api = useApi();
  const theme = useTheme();
  const [agentProgress, setAgentProgress] = useState<AgentProgressItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const fetchLiveProgress = async () => {
    try {
      const response = await api.get<LiveAgentFleetProgressResponse>('/api/v1/agents/fleet/live-progress');
      if (response && response.agents) {
        setAgentProgress(response.agents);
      }
      setError(null);
    } catch (err: any) {
      setError(err.message || 'Failed to load live progress');
      console.error('Failed to fetch live progress:', err);
    } finally {
      setLoading(false);
    }
  };

  // Auto-refresh every 5 seconds
  useEffect(() => {
    fetchLiveProgress();
    const interval = setInterval(fetchLiveProgress, 5000);
    return () => clearInterval(interval);
  }, []);

  if (loading && agentProgress.length === 0) {
    return <LinearProgress />;
  }

  if (error) {
    return (
      <Alert severity="error" sx={{ m: 2 }}>
        {error}
      </Alert>
    );
  }

  if (agentProgress.length === 0) {
    return (
      <Box sx={{ textAlign: 'center', py: 8 }}>
        <Typography variant="h6" color="text.secondary">
          No agents currently working
        </Typography>
        <Typography variant="body2" color="text.secondary" sx={{ mt: 1 }}>
          Create a SpecTask to start an agent
        </Typography>
      </Box>
    );
  }

  return (
    <Box sx={{ p: 3 }}>
      {/* Header */}
      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 3 }}>
        <Typography variant="h4" component="h1">
          Live Agent Fleet
        </Typography>
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
          <Chip
            label={`${agentProgress.length} agent${agentProgress.length === 1 ? '' : 's'} working`}
            color="primary"
          />
          <IconButton onClick={fetchLiveProgress} size="small">
            <RefreshIcon />
          </IconButton>
        </Box>
      </Box>

      {/* Agent Cards Grid */}
      <Grid container spacing={3}>
        {agentProgress.map((agent) => (
          <Grid item xs={12} md={6} lg={4} key={agent.agent_id}>
            <AgentTaskCard agent={agent} />
          </Grid>
        ))}
      </Grid>
    </Box>
  );
};

// AgentTaskCard shows a single agent's current progress
const AgentTaskCard: FC<{ agent: AgentProgressItem }> = ({ agent }) => {
  const theme = useTheme();

  return (
    <Card
      sx={{
        height: '100%',
        borderLeft: `4px solid ${theme.palette.warning.main}`,
        boxShadow: theme.shadows[3],
      }}
    >
      <CardHeader
        title={
          <Typography variant="h6" noWrap>
            {agent.task_name}
          </Typography>
        }
        subheader={
          <Box>
            <Typography variant="caption" display="block">
              Agent: {agent.agent_id.slice(0, 12)}...
            </Typography>
            <Chip
              label={agent.phase.replace(/_/g, ' ')}
              size="small"
              color="info"
              sx={{ mt: 0.5 }}
            />
          </Box>
        }
      />
      <CardContent>
        {/* Tasks before (completed, faded) */}
        {agent.tasks_before.map((task) => (
          <TaskListItem
            key={`before-${task.index}`}
            task={task}
            fade={0.4}
            completed
          />
        ))}

        {/* Current task (highlighted, pulsing) */}
        {agent.current_task && (
          <TaskListItem
            task={agent.current_task}
            highlight
            pulse
          />
        )}

        {/* Tasks after (upcoming, faded) */}
        {agent.tasks_after.map((task) => (
          <TaskListItem
            key={`after-${task.index}`}
            task={task}
            fade={0.6}
          />
        ))}

        {/* Progress summary */}
        <Box sx={{ mt: 2, pt: 2, borderTop: `1px solid ${theme.palette.divider}` }}>
          <Typography variant="caption" color="text.secondary">
            Last updated: {new Date(agent.last_update).toLocaleTimeString()}
          </Typography>
        </Box>
      </CardContent>
    </Card>
  );
};

// TaskListItem shows a single task with styling based on status
const TaskListItem: FC<{
  task: TaskItemDTO;
  highlight?: boolean;
  completed?: boolean;
  fade?: number;
  pulse?: boolean;
}> = ({ task, highlight, completed, fade = 1, pulse }) => {
  const theme = useTheme();

  return (
    <Box
      sx={{
        opacity: fade,
        backgroundColor: highlight ? theme.palette.warning.light : 'transparent',
        animation: pulse ? 'pulse 2s infinite' : 'none',
        borderLeft: highlight ? '4px solid' : 'none',
        borderColor: theme.palette.warning.main,
        p: 1.5,
        mb: 0.5,
        borderRadius: 1,
        transition: 'all 0.3s ease',
        '@keyframes pulse': {
          '0%, 100%': {
            opacity: fade,
          },
          '50%': {
            opacity: Math.min(fade + 0.2, 1),
          },
        },
      }}
    >
      <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5 }}>
        {completed ? (
          <CheckCircleIcon
            color="success"
            fontSize="small"
            sx={{ flexShrink: 0 }}
          />
        ) : highlight ? (
          <Box
            sx={{
              width: 20,
              height: 20,
              borderRadius: '50%',
              border: `2px solid ${theme.palette.warning.main}`,
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              animation: 'spin 2s linear infinite',
              '@keyframes spin': {
                '0%': { transform: 'rotate(0deg)' },
                '100%': { transform: 'rotate(360deg)' },
              },
            }}
          >
            <Box
              sx={{
                width: 8,
                height: 8,
                borderRadius: '50%',
                backgroundColor: theme.palette.warning.main,
              }}
            />
          </Box>
        ) : (
          <RadioButtonUncheckedIcon
            fontSize="small"
            sx={{ color: theme.palette.text.disabled, flexShrink: 0 }}
          />
        )}
        <Typography
          variant="body2"
          fontWeight={highlight ? 'bold' : 'normal'}
          sx={{
            color: completed
              ? theme.palette.text.secondary
              : highlight
              ? theme.palette.text.primary
              : theme.palette.text.secondary,
          }}
        >
          {task.description}
        </Typography>
      </Box>
    </Box>
  );
};

export default LiveAgentFleetDashboard;
