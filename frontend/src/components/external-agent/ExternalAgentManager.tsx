import React, { useState, useEffect } from 'react';
import {
  Box,
  Typography,
  Button,
  Card,
  CardContent,
  CardActions,
  Grid,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  TextField,
  FormControl,
  InputLabel,
  Select,
  MenuItem,
  Chip,
  IconButton,
  Tooltip,
  Alert,
  CircularProgress,
  List,
  ListItem,
  ListItemText,
  ListItemSecondaryAction,
} from '@mui/material';
import {
  Add,
  PlayArrow,
  Stop,
  Delete,
  Visibility,
  Refresh,
  Computer,
  Settings,
  Info,
  ArrowBack,
  CheckCircle,
  Error as ErrorIcon,
  Sync,
} from '@mui/icons-material';
import ScreenshotViewer from './ScreenshotViewer';
import { getBrowserLocale } from '../../hooks/useBrowserLocale';

interface ExternalAgent {
  session_id: string;
  rdp_url: string;
  status: string;
  pid: number;
  retries?: number;
  error?: string;
}

interface CreateAgentRequest {
  input: string;
  project_path?: string;
  work_dir?: string;
  env?: string[];
}

interface AgentStats {
  session_id: string;
  pid: number;
  start_time: string;
  last_access: string;
  uptime: number;
  workspace_dir: string;
  display_num: number;
  rdp_port: number;
  status: string;
}

const ExternalAgentManager: React.FC = () => {
  const [agents, setAgents] = useState<ExternalAgent[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [createDialogOpen, setCreateDialogOpen] = useState(false);
  const [viewerRunnerId, setViewerRunnerId] = useState<string | null>(null);
  const [selectedAgent, setSelectedAgent] = useState<ExternalAgent | null>(null);
  const [agentStats, setAgentStats] = useState<AgentStats | null>(null);
  const [statsDialogOpen, setStatsDialogOpen] = useState(false);

  // Create agent form state
  const [createForm, setCreateForm] = useState<CreateAgentRequest>({
    input: '',
    project_path: '',
    work_dir: '',
    env: [],
  });

  // Fetch all external agents
  const fetchAgents = async () => {
    setLoading(true);
    try {
      const response = await fetch('/api/v1/external-agents');
      if (!response.ok) {
        throw new Error(`Failed to fetch agents: ${response.statusText}`);
      }
      const data = await response.json();
      setAgents(Array.isArray(data) ? data : []);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to fetch external agents');
      setAgents([]);
    } finally {
      setLoading(false);
    }
  };

  // Create new external agent
  const createAgent = async (request: CreateAgentRequest) => {
    setLoading(true);
    try {
      // Add browser locale env vars for automatic keyboard layout detection
      // See: design/2025-12-17-keyboard-layout-option.md
      const { keyboardLayout, timezone } = getBrowserLocale();
      const envWithLocale = [
        ...(request.env || []),
        `XKB_DEFAULT_LAYOUT=${keyboardLayout}`,
        `TZ=${timezone}`,
      ];

      const response = await fetch('/api/v1/external-agents', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ ...request, env: envWithLocale }),
      });

      if (!response.ok) {
        throw new Error(`Failed to create agent: ${response.statusText}`);
      }

      const newAgent = await response.json();
      setAgents(prev => [...prev, newAgent]);
      setCreateDialogOpen(false);
      setCreateForm({
        input: '',
        project_path: '',
        work_dir: '',
        env: [],
      });
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create external agent');
    } finally {
      setLoading(false);
    }
  };

  // Stop external agent
  const stopAgent = async (sessionId: string) => {
    try {
      const response = await fetch(`/api/v1/external-agents/${sessionId}`, {
        method: 'DELETE',
      });

      if (!response.ok) {
        throw new Error(`Failed to stop agent: ${response.statusText}`);
      }

      setAgents(prev => prev.filter(agent => agent.session_id !== sessionId));
      
      // Close viewer if it was open for this agent
      if (viewerRunnerId === sessionId) {
        setViewerRunnerId(null);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to stop external agent');
    }
  };

  // Get agent statistics
  const getAgentStats = async (sessionId: string) => {
    try {
      const response = await fetch(`/api/v1/external-agents/${sessionId}/stats`);
      if (!response.ok) {
        throw new Error(`Failed to get agent stats: ${response.statusText}`);
      }
      const stats = await response.json();
      setAgentStats(stats);
      setStatsDialogOpen(true);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to get agent statistics');
    }
  };


  // Handle create form submission
  const handleCreateSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!createForm.input.trim()) {
      setError('Input is required');
      return;
    }
    createAgent(createForm);
  };

  // Handle environment variable input
  const handleEnvChange = (value: string) => {
    const envVars = value.split('\n').filter(line => line.trim() && line.includes('='));
    setCreateForm(prev => ({ ...prev, env: envVars }));
  };


  // Initialize and set up polling
  useEffect(() => {
    fetchAgents();

    // Poll for updates every 30 seconds
    const interval = setInterval(fetchAgents, 30000);

    return () => clearInterval(interval);
  }, []);


  if (viewerRunnerId) {
    return (
      <Box sx={{ position: 'relative', width: '100%', height: '100vh' }}>
        {/* Close button */}
        <IconButton
          onClick={() => setViewerRunnerId(null)}
          sx={{
            position: 'absolute',
            top: 10,
            left: 10,
            zIndex: 1000,
            backgroundColor: 'rgba(0,0,0,0.7)',
            color: 'white',
            '&:hover': {
              backgroundColor: 'rgba(0,0,0,0.9)',
            }
          }}
        >
          <ArrowBack />
        </IconButton>
        
        <ScreenshotViewer
          sessionId={viewerRunnerId}
          isRunner={true}
          onConnectionChange={(connected) => {
            console.log('Runner RDP connection status:', connected);
          }}
          onError={(error) => {
            console.error('Runner RDP error:', error);
          }}
        />
      </Box>
    );
  }

  return (
    <Box sx={{ p: 3 }}>
      {/* Header */}
      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 3 }}>
        <Typography variant="h4" component="h1">
          External Agents
        </Typography>
        <Box sx={{ display: 'flex', gap: 1 }}>
          <Button
            variant="outlined"
            startIcon={<Refresh />}
            onClick={fetchAgents}
            disabled={loading}
          >
            Refresh
          </Button>
          <Button
            variant="contained"
            startIcon={<Add />}
            onClick={() => setCreateDialogOpen(true)}
            disabled={loading}
          >
            New Agent
          </Button>
        </Box>
      </Box>

      {/* Error Alert */}
      {error && (
        <Alert severity="error" sx={{ mb: 2 }} onClose={() => setError(null)}>
          {error}
        </Alert>
      )}

      {/* Loading State */}
      {loading && agents.length === 0 && (
        <Box sx={{ display: 'flex', justifyContent: 'center', py: 4 }}>
          <CircularProgress />
        </Box>
      )}

      {/* Agents Grid */}
      {agents.length > 0 ? (
        <Grid container spacing={3}>
          {agents.map((agent) => (
            <Grid item xs={12} sm={6} md={4} key={agent.session_id}>
              <Card>
                <CardContent>
                  <Box sx={{ display: 'flex', alignItems: 'center', mb: 2 }}>
                    <Computer sx={{ mr: 1, color: 'primary.main' }} />
                    <Typography variant="h6" component="div" sx={{ flexGrow: 1 }}>
                      Zed Editor
                    </Typography>
                    <Chip
                      label={agent.status}
                      color={agent.status === 'running' ? 'success' : 'default'}
                      size="small"
                    />
                  </Box>

                  <Typography variant="body2" color="text.secondary" noWrap sx={{ mt: 1 }}>
                    Session: {agent.session_id}
                  </Typography>
                  
                  {agent.pid && (
                    <Typography variant="body2" color="text.secondary">
                      PID: {agent.pid}
                    </Typography>
                  )}
                  
                  {agent.rdp_url && (
                    <Typography variant="body2" color="text.secondary" noWrap>
                      RDP: {agent.rdp_url}
                    </Typography>
                  )}

                  {agent.error && (
                    <Alert severity="error" sx={{ mt: 1 }}>
                      {agent.error}
                    </Alert>
                  )}
                </CardContent>
                
                <CardActions>
                  <Tooltip title="Open RDP Viewer">
                    <IconButton
                      size="small"
                      onClick={() => setViewerRunnerId(agent.session_id)}
                      disabled={agent.status !== 'running'}
                    >
                      <Visibility />
                    </IconButton>
                  </Tooltip>
                  
                  <Tooltip title="View Statistics">
                    <IconButton
                      size="small"
                      onClick={() => getAgentStats(agent.session_id)}
                    >
                      <Info />
                    </IconButton>
                  </Tooltip>
                  
                  <Tooltip title="Stop Agent">
                    <IconButton
                      size="small"
                      onClick={() => stopAgent(agent.session_id)}
                      color="error"
                    >
                      <Stop />
                    </IconButton>
                  </Tooltip>
                </CardActions>
              </Card>
            </Grid>
          ))}
        </Grid>
      ) : !loading && (
        <Box sx={{ textAlign: 'center', py: 8 }}>
          <Computer sx={{ fontSize: 64, color: 'text.secondary', mb: 2 }} />
          <Typography variant="h6" color="text.secondary" gutterBottom>
            No External Agents Running
          </Typography>
          <Typography variant="body2" color="text.secondary" paragraph>
            Create a new external agent to start using Zed editor in the cloud.
          </Typography>
          <Button
            variant="contained"
            startIcon={<Add />}
            onClick={() => setCreateDialogOpen(true)}
          >
            Create Your First Agent
          </Button>
        </Box>
      )}

      {/* Create Agent Dialog */}
      <Dialog open={createDialogOpen} onClose={() => setCreateDialogOpen(false)} maxWidth="md" fullWidth>
        <form onSubmit={handleCreateSubmit}>
          <DialogTitle>Create New External Agent</DialogTitle>
          <DialogContent>
            <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2, pt: 1 }}>
              <TextField
                label="Initial Task/Prompt"
                multiline
                rows={3}
                fullWidth
                required
                value={createForm.input}
                onChange={(e) => setCreateForm(prev => ({ ...prev, input: e.target.value }))}
                placeholder="e.g., Create a new Rust web server project"
                helperText="Describe what you want the Zed editor to help you with"
              />
              
              <TextField
                label="Project Path (Optional)"
                fullWidth
                value={createForm.project_path}
                onChange={(e) => setCreateForm(prev => ({ ...prev, project_path: e.target.value }))}
                placeholder="e.g., my-project"
                helperText="Relative path for the project directory"
              />
              
              <TextField
                label="Working Directory (Optional)"
                fullWidth
                value={createForm.work_dir}
                onChange={(e) => setCreateForm(prev => ({ ...prev, work_dir: e.target.value }))}
                placeholder="e.g., /workspace/custom-path"
                helperText="Custom working directory (defaults to generated path)"
              />
              
              <TextField
                label="Environment Variables (Optional)"
                multiline
                rows={4}
                fullWidth
                placeholder="NODE_ENV=development&#10;API_KEY=your-key"
                helperText="One variable per line in KEY=VALUE format"
                onChange={(e) => handleEnvChange(e.target.value)}
              />
            </Box>
          </DialogContent>
          <DialogActions>
            <Button onClick={() => setCreateDialogOpen(false)}>
              Cancel
            </Button>
            <Button type="submit" variant="contained" disabled={loading}>
              Create Agent
            </Button>
          </DialogActions>
        </form>
      </Dialog>

      {/* Agent Statistics Dialog */}
      <Dialog open={statsDialogOpen} onClose={() => setStatsDialogOpen(false)} maxWidth="sm" fullWidth>
        <DialogTitle>Agent Statistics</DialogTitle>
        <DialogContent>
          {agentStats && (
            <List>
              <ListItem>
                <ListItemText 
                  primary="Session ID" 
                  secondary={agentStats.session_id} 
                />
              </ListItem>
              <ListItem>
                <ListItemText 
                  primary="Process ID" 
                  secondary={agentStats.pid} 
                />
              </ListItem>
              <ListItem>
                <ListItemText 
                  primary="Status" 
                  secondary={<Chip label={agentStats.status} size="small" />} 
                />
              </ListItem>
              <ListItem>
                <ListItemText 
                  primary="Uptime" 
                  secondary={`${Math.round(agentStats.uptime)} seconds`} 
                />
              </ListItem>
              <ListItem>
                <ListItemText 
                  primary="RDP Port" 
                  secondary={agentStats.rdp_port} 
                />
              </ListItem>
              <ListItem>
                <ListItemText 
                  primary="Display" 
                  secondary={`:${agentStats.display_num}`} 
                />
              </ListItem>
              <ListItem>
                <ListItemText 
                  primary="Workspace Directory" 
                  secondary={agentStats.workspace_dir} 
                />
              </ListItem>
              <ListItem>
                <ListItemText 
                  primary="Started" 
                  secondary={new Date(agentStats.start_time).toLocaleString()} 
                />
              </ListItem>
              <ListItem>
                <ListItemText 
                  primary="Last Access" 
                  secondary={new Date(agentStats.last_access).toLocaleString()} 
                />
              </ListItem>
            </List>
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setStatsDialogOpen(false)}>
            Close
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
};

export default ExternalAgentManager;