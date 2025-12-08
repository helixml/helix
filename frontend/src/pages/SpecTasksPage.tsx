import React, { FC, useState, useEffect, useRef, useMemo } from 'react';
import { useQuery } from '@tanstack/react-query';
import {
  Box,
  Button,
  Typography,
  Alert,
  Chip,
  Stack,
  Drawer,
  TextField,
  FormControl,
  InputLabel,
  Select,
  MenuItem,
  CircularProgress,
  List,
  ListItem,
  ListItemText,
  ListItemSecondaryAction,
  IconButton,
  Divider,
  Checkbox,
  FormControlLabel,
  Tooltip,
} from '@mui/material';
import {
  Add as AddIcon,
  Refresh as RefreshIcon,
  Settings as SettingsIcon,
  Close as CloseIcon,
  Explore as ExploreIcon,
  Stop as StopIcon,
} from '@mui/icons-material';

import Page from '../components/system/Page';
import SpecTaskKanbanBoard from '../components/tasks/SpecTaskKanbanBoard';
import SpecTaskDetailDialog from '../components/tasks/SpecTaskDetailDialog';

import useAccount from '../hooks/useAccount';
import useApi from '../hooks/useApi';
import useSnackbar from '../hooks/useSnackbar';
import useRouter from '../hooks/useRouter';
import useApps from '../hooks/useApps';
import { useSpecTasks } from '../hooks/useSpecTasks';
import { useFloatingModal } from '../contexts/floatingModal';
import {
  useGetProject,
  useGetProjectRepositories,
  useGetProjectExploratorySession,
  useStartProjectExploratorySession,
  useStopProjectExploratorySession,
  useResumeProjectExploratorySession,
} from '../services';
import { TypesSpecTask, ServicesCreateTaskRequest } from '../api/api';

const SpecTasksPage: FC = () => {
  const account = useAccount();
  const api = useApi();
  const snackbar = useSnackbar();
  const router = useRouter();
  const apps = useApps();
  const floatingModal = useFloatingModal();

  // Get project ID from URL if in project context
  const projectId = router.params.id as string | undefined;

  // Fetch project data for breadcrumbs and title
  const { data: project } = useGetProject(projectId || '', !!projectId);

  // Fetch project repositories for display in topbar (filters out internal repos)
  const { data: projectRepositories = [] } = useGetProjectRepositories(projectId || '', !!projectId);

  // Exploratory session hooks
  const { data: exploratorySessionData } = useGetProjectExploratorySession(projectId || '', !!projectId);
  const startExploratorySessionMutation = useStartProjectExploratorySession(projectId || '');
  const stopExploratorySessionMutation = useStopProjectExploratorySession(projectId || '');
  const resumeExploratorySessionMutation = useResumeProjectExploratorySession(projectId || '');

  // Query wolf instances to check for privileged mode availability
  const { data: wolfInstances } = useQuery({
    queryKey: ['wolf-instances'],
    queryFn: async () => {
      const response = await api.getApiClient().v1WolfInstancesList();
      return response.data;
    },
    staleTime: 60000, // Cache for 1 minute
  });

  // Check if any sandbox has privileged mode enabled
  const hasPrivilegedSandbox = useMemo(() => {
    return wolfInstances?.some(instance => instance.privileged_mode_enabled) ?? false;
  }, [wolfInstances]);

  // Redirect to projects list if no project selected (new architecture: must select project first)
  // Exception: if user is trying to create a new task (new=true param), allow it for backward compat
  useEffect(() => {
    const isCreatingNew = router.params.new === 'true';
    if (!projectId && !isCreatingNew) {
      console.log('No project ID in route - redirecting to projects list');
      account.orgNavigate('projects');
    }
  }, [projectId, router.params.new, account]);

  // State for view management
  const [createDialogOpen, setCreateDialogOpen] = useState(false);
  const [refreshing, setRefreshing] = useState(false);
  const [refreshTrigger, setRefreshTrigger] = useState(0);

  // Create task form state (SIMPLIFIED)
  const [taskPrompt, setTaskPrompt] = useState(''); // Single text box for everything
  const [taskPriority, setTaskPriority] = useState('medium');
  const [selectedHelixAgent, setSelectedHelixAgent] = useState('');
  const [justDoItMode, setJustDoItMode] = useState(false); // Just Do It mode: skip spec, go straight to implementation
  const [useHostDocker, setUseHostDocker] = useState(false); // Use host Docker socket (requires privileged sandbox)
  // Repository configuration moved to project level - no task-level repo selection needed

  // Task detail windows state - array to support multiple windows
  const [openTaskWindows, setOpenTaskWindows] = useState<TypesSpecTask[]>([]);

  // Ref for task prompt text field to manually focus
  const taskPromptRef = useRef<HTMLTextAreaElement>(null);

  // Data hooks
  const { data: tasks, loading: tasksLoading, listTasks } = useSpecTasks();

  // Load tasks and apps on mount
  useEffect(() => {
    const loadTasks = async () => {
      try {
        const result = await api.getApiClient().v1SpecTasksList();
        // The hook will handle updating the data automatically
      } catch (error) {
        console.error('Error loading spec tasks:', error);
      }
    };

    if (account.user?.id) {
      loadTasks();
      apps.loadApps(); // Load available agents
    }
  }, []);

  // Blur task prompt input when dialog closes to prevent hidden typing
  useEffect(() => {
    if (!createDialogOpen && taskPromptRef.current) {
      taskPromptRef.current.blur();
    }
  }, [createDialogOpen]);

  // Auto-select default agent when dialog opens
  useEffect(() => {
    if (createDialogOpen) {
      // First priority: use project's default agent if set
      if (project?.default_helix_app_id) {
        setSelectedHelixAgent(project.default_helix_app_id);
      } else if (apps.apps.length === 0) {
        // No agents exist, default to create option
        setSelectedHelixAgent('__create_default__');
      } else {
        // Agents exist but project has no default, select first one
        setSelectedHelixAgent(apps.apps[0]?.id || '__create_default__');
      }

      // Focus the text field when dialog opens
      setTimeout(() => {
        if (taskPromptRef.current) {
          taskPromptRef.current.focus();
        }
      }, 100);
    }
  }, [createDialogOpen, apps.apps, project?.default_helix_app_id]);

  // Handle URL parameters for opening dialog
  useEffect(() => {
    if (router.params.new === 'true') {
      setCreateDialogOpen(true);
      // Clear URL parameter after handling
      router.removeParams(['new']);
    }
  }, [router.params.new]);

  // Check authentication
  const checkLoginStatus = (): boolean => {
    if (!account.user) {
      account.setShowLoginWindow(true);
      return false;
    }
    return true;
  };

  // Keyboard shortcut: Enter to toggle new task dialog
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Enter' && !e.ctrlKey && !e.metaKey && !e.altKey && !e.shiftKey) {
        // Only trigger if not typing in an input field or focused interactive element
        const target = e.target as HTMLElement;
        if (target.tagName === 'INPUT' ||
            target.tagName === 'TEXTAREA' ||
            target.isContentEditable ||
            target.hasAttribute('tabindex')) { // Exclude focusable elements like stream viewer
          return;
        }
        e.preventDefault();
        // Toggle behavior: open if closed, close if open and no focus
        setCreateDialogOpen(prev => !prev);
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [createDialogOpen]);

  // Keyboard shortcut: Ctrl/Cmd+Enter to submit task (when dialog is open)
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') {
        if (createDialogOpen && taskPrompt.trim()) {
          e.preventDefault();
          handleCreateTask();
        }
      }
    };

    if (createDialogOpen) {
      window.addEventListener('keydown', handleKeyDown);
      return () => window.removeEventListener('keydown', handleKeyDown);
    }
  }, [createDialogOpen, taskPrompt]);

  // Keyboard shortcut: ESC to close create task panel
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        if (createDialogOpen) {
          e.preventDefault();
          setCreateDialogOpen(false);
        }
      }
    };

    if (createDialogOpen) {
      window.addEventListener('keydown', handleKeyDown);
      return () => window.removeEventListener('keydown', handleKeyDown);
    }
  }, [createDialogOpen]);

  // Handle task creation - SIMPLIFIED
  const handleCreateTask = async () => {
    if (!checkLoginStatus()) return;

    try {
      if (!taskPrompt.trim()) {
        snackbar.error('Please describe what you want to get done');
        return;
      }

      let agentId = selectedHelixAgent;

      // Create default agent if requested
      if (selectedHelixAgent === '__create_default__') {
        try {
          snackbar.info('Creating default agent...');

          const newAgent = await apps.createAgent({
            name: 'Default Spec Agent',
            systemPrompt: 'You are a software development agent that helps with planning and implementation. For planning tasks, analyze user requirements and create detailed design documents. For implementation tasks, write high-quality code based on specifications.',
            agentType: 'zed_external',
            reasoningModelProvider: '',
            reasoningModel: '',
            reasoningModelEffort: '',
            generationModelProvider: '',
            generationModel: '',
            smallReasoningModelProvider: '',
            smallReasoningModel: '',
            smallReasoningModelEffort: '',
            smallGenerationModelProvider: '',
            smallGenerationModel: '',
          });

          if (!newAgent || !newAgent.id) {
            throw new Error('Failed to create default agent');
          }

          agentId = newAgent.id;
          console.log('Created default agent with ID:', agentId);
          // Note: apps.createAgent() already reloads the apps list internally
        } catch (err: any) {
          console.error('Failed to create default agent:', err);
          const errorMessage = err?.response?.data?.message
            || err?.message
            || 'Failed to create default agent. Please try again.';
          snackbar.error(errorMessage);
          return;
        }
      }

      // Create SpecTask with simplified single-field approach
      // Repository configuration is managed at the project level - no task-level repo selection
      const createTaskRequest: ServicesCreateTaskRequest = {
        prompt: taskPrompt, // Just the raw user input!
        priority: taskPriority,
        project_id: projectId || 'default', // Use project ID from route, or 'default'
        app_id: agentId || undefined, // Include selected or created agent if provided
        just_do_it_mode: justDoItMode, // Just Do It mode: skip spec, go straight to implementation
        use_host_docker: useHostDocker, // Use host Docker socket (requires privileged sandbox)
        // Repositories inherited from parent project - no task-level repo configuration
      };

      console.log('Creating SpecTask with simplified prompt:', createTaskRequest);

      const response = await api.getApiClient().v1SpecTasksFromPromptCreate(createTaskRequest);

      if (response.data) {
        console.log('SpecTask created successfully:', response.data);
        snackbar.success('SpecTask created! Planning agent will generate specifications.');
        setCreateDialogOpen(false);
        setTaskPrompt('');
        setTaskPriority('medium');
        setSelectedHelixAgent(''); // Reset agent selection
        setJustDoItMode(false); // Reset Just Do It mode
        setUseHostDocker(false); // Reset host Docker mode

        // Trigger immediate refresh of Kanban board
        setRefreshTrigger(prev => prev + 1);
      }
    } catch (error: any) {
      console.error('Failed to create SpecTask:', error);
      const errorMessage = error?.response?.data?.message
        || error?.message
        || 'Failed to create SpecTask. Please try again.';
      snackbar.error(errorMessage);
      // Dialog stays open so user doesn't lose their input
    }
  };

  const handleStartExploratorySession = async () => {
    try {
      const session = await startExploratorySessionMutation.mutateAsync();
      snackbar.success('Exploratory session started');
      // Open in floating window instead of navigating
      floatingModal.showFloatingModal({
        type: 'exploratory_session',
        sessionId: session.id,
        wolfLobbyId: session.config?.wolf_lobby_id || session.id
      }, { x: window.innerWidth - 400, y: 100 });
    } catch (err: any) {
      // Extract error message from API response
      const errorMessage = err?.response?.data?.error || err?.message || 'Failed to start exploratory session';
      snackbar.error(errorMessage);
    }
  };

  const handleResumeExploratorySession = async (e: React.MouseEvent) => {
    if (!exploratorySessionData) return;

    try {
      // Use the mutation hook which properly invalidates the cache
      const session = await resumeExploratorySessionMutation.mutateAsync();
      snackbar.success('Exploratory session resumed');
      // Open floating window
      floatingModal.showFloatingModal({
        type: 'exploratory_session',
        sessionId: session.id,
        wolfLobbyId: session.config?.wolf_lobby_id || session.id
      }, { x: e.clientX, y: e.clientY });
    } catch (err) {
      snackbar.error('Failed to resume exploratory session');
    }
  };

  const handleStopExploratorySession = async () => {
    try {
      await stopExploratorySessionMutation.mutateAsync();
      snackbar.success('Exploratory session stopped');
    } catch (err) {
      snackbar.error('Failed to stop exploratory session');
    }
  };

  return (
    <Page
      breadcrumbs={project ? [
        {
          title: 'Projects',
          routeName: 'projects',
        },
        {
          title: project.name,
        },
      ] : undefined}
      breadcrumbTitle={project ? undefined : "SpecTasks"}
      orgBreadcrumbs={true}
      showDrawerButton={false}
      topbarContent={
        <Stack direction="row" spacing={2} sx={{ justifyContent: 'flex-end', width: '100%', minWidth: 0, alignItems: 'center' }}>
          {!exploratorySessionData ? (
            <Button
              variant="outlined"
              color="secondary"
              startIcon={<ExploreIcon />}
              onClick={handleStartExploratorySession}
              disabled={startExploratorySessionMutation.isPending}
              sx={{ flexShrink: 0 }}
            >
              {startExploratorySessionMutation.isPending ? 'Starting...' : 'Start Exploratory Session'}
            </Button>
          ) : exploratorySessionData.config?.external_agent_status === 'stopped' ? (
            <Button
              variant="outlined"
              color="secondary"
              startIcon={<ExploreIcon />}
              onClick={handleResumeExploratorySession}
              disabled={resumeExploratorySessionMutation.isPending}
              sx={{ flexShrink: 0 }}
            >
              {resumeExploratorySessionMutation.isPending ? 'Resuming...' : 'Resume Session'}
            </Button>
          ) : (
            <>
              <Button
                variant="contained"
                color="primary"
                startIcon={<ExploreIcon />}
                onClick={(e) => {
                  floatingModal.showFloatingModal({
                    type: 'exploratory_session',
                    sessionId: exploratorySessionData.id,
                    wolfLobbyId: exploratorySessionData.config?.wolf_lobby_id || exploratorySessionData.id
                  }, { x: e.clientX, y: e.clientY });
                }}
                sx={{ flexShrink: 0 }}
              >
                View Session
              </Button>
              <Button
                variant="outlined"
                color="error"
                startIcon={<StopIcon />}
                onClick={handleStopExploratorySession}
                disabled={stopExploratorySessionMutation.isPending}
                sx={{ flexShrink: 0 }}
              >
                {stopExploratorySessionMutation.isPending ? 'Stopping...' : 'Stop Session'}
              </Button>
            </>
          )}
          <Button
            variant="outlined"
            startIcon={<SettingsIcon />}
            onClick={() => account.orgNavigate('project-settings', { id: projectId })}
            sx={{ flexShrink: 0 }}
          >
            Settings
          </Button>
        </Stack>
      }
    >
      <Box sx={{
        display: 'flex',
        flexDirection: 'row',
        width: '100%',
        height: 'calc(100vh - 120px)',
        overflow: 'hidden',
        position: 'relative',
      }}>

        {/* MAIN CONTENT */}
        <Box sx={{
          flex: 1,
          display: 'flex',
          flexDirection: 'column',
          minWidth: 0,
          overflow: 'hidden',
          transition: 'all 0.3s ease-in-out',
          px: 3,
        }}>
          {/* No repositories warning */}
          {projectRepositories.length === 0 && (
            <Alert
              severity="info"
              sx={{ mb: 2 }}
              action={
                <Button
                  color="inherit"
                  size="small"
                  variant="outlined"
                  onClick={() => account.orgNavigate('project-settings', { id: projectId })}
                >
                  Go to Settings
                </Button>
              }
            >
              No code repositories attached. Attach a repository in Settings to give agents access to your code.
            </Alert>
          )}

          {/* Kanban Board */}
          <Box sx={{ flex: 1, minHeight: 0, minWidth: 0, display: 'flex', flexDirection: 'column', overflowX: 'hidden' }}>
            <SpecTaskKanbanBoard
              userId={account.user?.id}
              projectId={projectId}
              onCreateTask={() => setCreateDialogOpen(true)}
              onTaskClick={(task) => {
                // Add to array of open windows if not already open
                setOpenTaskWindows(prev => {
                  const alreadyOpen = prev.some(t => t.id === task.id);
                  if (alreadyOpen) return prev;
                  return [...prev, task];
                });
              }}
              onRefresh={() => {
                setRefreshing(true);
                setTimeout(() => setRefreshing(false), 2000);
              }}
              refreshing={refreshing}
              refreshTrigger={refreshTrigger}
            />
          </Box>
        </Box>

        {/* RIGHT PANEL: New Spec Task - slides in from right */}
        <Box
          sx={{
            width: createDialogOpen ? { xs: '100%', sm: '450px', md: '500px' } : 0,
            flexShrink: 0,
            overflow: 'hidden',
            transition: 'width 0.3s ease-in-out',
            borderLeft: createDialogOpen ? 1 : 0,
            borderColor: 'divider',
            display: 'flex',
            flexDirection: 'column',
            backgroundColor: 'background.paper',
          }}
        >
        <Box sx={{ height: '100%', display: 'flex', flexDirection: 'column' }}>
          {/* Header */}
          <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', p: 2, borderBottom: 1, borderColor: 'divider' }}>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
              <AddIcon />
              <Typography variant="h6">New SpecTask</Typography>
            </Box>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
              <Box
                component="span"
                sx={{
                  fontSize: '0.75rem',
                  opacity: 0.6,
                  fontFamily: 'monospace',
                  backgroundColor: 'rgba(0, 0, 0, 0.1)',
                  px: 0.75,
                  py: 0.25,
                  borderRadius: '4px',
                  border: '1px solid',
                  borderColor: 'divider',
                }}
              >
                Esc
              </Box>
              <IconButton onClick={() => setCreateDialogOpen(false)}>
                <CloseIcon />
              </IconButton>
            </Box>
          </Box>

          {/* Content */}
          <Box sx={{ flex: 1, overflow: 'auto', p: 3 }}>
            <Stack spacing={3}>
              {/* Priority Selector */}
              <FormControl fullWidth>
                <InputLabel>Priority</InputLabel>
                <Select
                  value={taskPriority}
                  onChange={(e) => setTaskPriority(e.target.value)}
                  label="Priority"
                >
                  <MenuItem value="low">Low</MenuItem>
                  <MenuItem value="medium">Medium</MenuItem>
                  <MenuItem value="high">High</MenuItem>
                  <MenuItem value="critical">Critical</MenuItem>
                </Select>
              </FormControl>

              {/* Single large text box for everything */}
              <TextField
                label="Describe what you want to get done"
                fullWidth
                required
                multiline
                rows={12}
                value={taskPrompt}
                onChange={(e) => setTaskPrompt(e.target.value)}
                onKeyDown={(e) => {
                  // If user presses Enter in empty text box, close panel
                  if (e.key === 'Enter' && !taskPrompt.trim() && !e.shiftKey && !e.ctrlKey && !e.metaKey) {
                    e.preventDefault()
                    setCreateDialogOpen(false)
                  }
                }}
                placeholder="Dump everything you know here - the AI will parse this into requirements, design, and implementation plan.

Examples:
- Add dark mode toggle to settings page
- Fix the user registration bug where emails aren't validated properly
- Refactor the payment processing to use Stripe instead of PayPal"
                helperText="The planning agent will extract task name, description, type, and generate full specifications from this"
                inputRef={taskPromptRef}
              />

              {/* Helix Agent Selection */}
              <FormControl fullWidth>
                <InputLabel>Helix Agent</InputLabel>
                <Select
                  value={selectedHelixAgent}
                  onChange={(e) => setSelectedHelixAgent(e.target.value)}
                  label="Helix Agent"
                >
                  {apps.apps.map((app) => (
                    <MenuItem key={app.id} value={app.id}>
                      {app.config?.helix?.name || 'Unnamed Agent'}
                    </MenuItem>
                  ))}
                  <MenuItem value="__create_default__">
                    <em>Create new external agent...</em>
                  </MenuItem>
                </Select>
                <Typography variant="caption" color="text.secondary" sx={{ mt: 1 }}>
                  {selectedHelixAgent === '__create_default__'
                    ? 'A new external agent will be created when you submit this task.'
                    : 'Select which agent will generate specifications for this task.'}
                </Typography>
              </FormControl>

              {/* Just Do It Mode Checkbox */}
              <FormControl fullWidth>
                <Tooltip title="Skip writing a spec and just get the agent to immediately start doing what you ask" placement="top">
                  <FormControlLabel
                    control={
                      <Checkbox
                        checked={justDoItMode}
                        onChange={(e) => setJustDoItMode(e.target.checked)}
                        color="warning"
                      />
                    }
                    label={
                      <Box>
                        <Typography variant="body2" sx={{ fontWeight: 600 }}>
                          Just Do It
                        </Typography>
                        <Typography variant="caption" color="text.secondary">
                          Skip spec planning — useful for tasks that don't require planning code changes (e.g., if you don't want the agent to push code)
                        </Typography>
                      </Box>
                    }
                  />
                </Tooltip>
              </FormControl>

              {/* Use Host Docker Checkbox (for Helix-in-Helix development) - only show if a privileged sandbox is available */}
              {hasPrivilegedSandbox && (
                <FormControl fullWidth>
                  <Tooltip title="Use the host's Docker socket instead of isolated Docker-in-Docker. Requires a sandbox with privileged mode enabled." placement="top">
                    <FormControlLabel
                      control={
                        <Checkbox
                          checked={useHostDocker}
                          onChange={(e) => setUseHostDocker(e.target.checked)}
                          color="info"
                        />
                      }
                      label={
                        <Box>
                          <Typography variant="body2" sx={{ fontWeight: 600 }}>
                            Use Host Docker 🐳
                          </Typography>
                          <Typography variant="caption" color="text.secondary">
                            For Helix-in-Helix development — agent can build and run Helix containers
                          </Typography>
                        </Box>
                      }
                    />
                  </Tooltip>
                </FormControl>
              )}
            </Stack>
          </Box>

          {/* Footer Actions */}
          <Box sx={{ p: 2, borderTop: 1, borderColor: 'divider', display: 'flex', gap: 2, justifyContent: 'flex-end' }}>
            <Button onClick={() => {
              setCreateDialogOpen(false);
              setTaskPrompt('');
              setTaskPriority('medium');
              setSelectedHelixAgent('');
              setJustDoItMode(false);
              setUseHostDocker(false);
            }}>
              Cancel
            </Button>
            <Button
              onClick={handleCreateTask}
              variant="contained"
              color="secondary"
              disabled={!taskPrompt.trim()}
              startIcon={<AddIcon />}
              sx={{
                '& .MuiButton-endIcon': {
                  ml: 1,
                  opacity: 0.6,
                  fontSize: '0.75rem',
                },
              }}
              endIcon={
                <Box component="span" sx={{
                  fontSize: '0.75rem',
                  opacity: 0.6,
                  fontFamily: 'monospace',
                  ml: 1,
                }}>
                  {navigator.platform.includes('Mac') ? '⌘↵' : 'Ctrl+↵'}
                </Box>
              }
            >
              Create Task
            </Button>
          </Box>
        </Box>
        </Box>

      </Box>

      {/* Task Detail Dialogs - one per open task */}
      {openTaskWindows.map((task) => (
        <SpecTaskDetailDialog
          key={task.id}
          task={task}
          open={true}
          onClose={() => {
            setOpenTaskWindows(prev => prev.filter(t => t.id !== task.id));
          }}
          onEdit={(task) => {
            // TODO: Implement task editing
            console.log('Edit task:', task);
          }}
        />
      ))}

    </Page>
  );
};

export default SpecTasksPage;
