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
  ListItemButton,
  ListItemIcon,
  ListItemText,
  ListItemSecondaryAction,
  IconButton,
  Divider,
  Checkbox,
  FormControlLabel,
  Tooltip,
  Avatar,
} from '@mui/material';
import {
  Add as AddIcon,
  Refresh as RefreshIcon,
  Settings as SettingsIcon,
  Close as CloseIcon,
  Explore as ExploreIcon,
  Stop as StopIcon,
  SmartToy as SmartToyIcon,
} from '@mui/icons-material';

import Page from '../components/system/Page';
import SpecTaskKanbanBoard from '../components/tasks/SpecTaskKanbanBoard';
import SpecTaskDetailDialog from '../components/tasks/SpecTaskDetailDialog';
import { AdvancedModelPicker } from '../components/create/AdvancedModelPicker';
import { CodeAgentRuntime, generateAgentName, ICreateAgentParams } from '../contexts/apps';
import { AGENT_TYPE_ZED_EXTERNAL, IApp } from '../types';

// Recommended models for zed_external agents (state-of-the-art coding models)
const RECOMMENDED_MODELS = [
  // Anthropic
  'claude-opus-4-5-20251101',
  'claude-sonnet-4-5-20250929',
  'claude-haiku-4-5-20251001',
  // OpenAI
  'openai/gpt-5.1-codex',
  'openai/gpt-oss-120b',
  // Google Gemini
  'gemini-2.5-pro',
  'gemini-2.5-flash',
  // Zhipu GLM
  'glm-4.6',
  // Qwen (Coder + Large)
  'Qwen/Qwen3-Coder-480B-A35B-Instruct',
  'Qwen/Qwen3-Coder-30B-A3B-Instruct',
  'Qwen/Qwen3-235B-A22B-fp8-tput',
];

import useAccount from '../hooks/useAccount';
import useApi from '../hooks/useApi';
import useSnackbar from '../hooks/useSnackbar';
import useRouter from '../hooks/useRouter';
import useApps from '../hooks/useApps';
import { useFloatingModal } from '../contexts/floatingModal';
import EditIcon from '@mui/icons-material/Edit';
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

  // Inline agent creation state (same UX as AgentSelectionModal)
  const [showCreateAgentForm, setShowCreateAgentForm] = useState(false);
  const [codeAgentRuntime, setCodeAgentRuntime] = useState<CodeAgentRuntime>('zed_agent');
  const [selectedProvider, setSelectedProvider] = useState('');
  const [selectedModel, setSelectedModel] = useState('');
  const [newAgentName, setNewAgentName] = useState('-');
  const [userModifiedName, setUserModifiedName] = useState(false);
  const [creatingAgent, setCreatingAgent] = useState(false);
  const [agentError, setAgentError] = useState('');
  // Repository configuration moved to project level - no task-level repo selection needed

  // Task detail windows state - array to support multiple windows
  const [openTaskWindows, setOpenTaskWindows] = useState<TypesSpecTask[]>([]);

  // Track newly created task ID for focusing "Start Planning" button
  const [focusTaskId, setFocusTaskId] = useState<string | undefined>(undefined);

  // Ref for task prompt text field to manually focus
  const taskPromptRef = useRef<HTMLTextAreaElement>(null);

  // Sort apps: project default first, then zed_external, then others
  const sortedApps = useMemo(() => {
    if (!apps.apps) return [];
    const zedExternalApps: IApp[] = [];
    const otherApps: IApp[] = [];
    let defaultApp: IApp | null = null;
    const projectDefaultId = project?.default_helix_app_id;

    apps.apps.forEach((app) => {
      // Pull out the project's default agent to pin at top
      if (projectDefaultId && app.id === projectDefaultId) {
        defaultApp = app;
        return;
      }

      const hasZedExternal = app.config?.helix?.assistants?.some(
        (assistant) => assistant.agent_type === AGENT_TYPE_ZED_EXTERNAL
      ) || app.config?.helix?.default_agent_type === AGENT_TYPE_ZED_EXTERNAL;
      if (hasZedExternal) {
        zedExternalApps.push(app);
      } else {
        otherApps.push(app);
      }
    });

    // Project default first, then zed_external, then others
    const result: IApp[] = [];
    if (defaultApp) result.push(defaultApp);
    result.push(...zedExternalApps, ...otherApps);
    return result;
  }, [apps.apps, project?.default_helix_app_id]);

  // Create a map of app ID -> app name for displaying agent names
  const appNamesMap = useMemo(() => {
    const map: Record<string, string> = {};
    if (apps.apps) {
      apps.apps.forEach((app) => {
        if (app.id) {
          map[app.id] = app.config?.helix?.name || 'Unnamed Agent';
        }
      });
    }
    return map;
  }, [apps.apps]);

  // Auto-generate agent name when model or runtime changes (if user hasn't modified it)
  useEffect(() => {
    if (!userModifiedName && showCreateAgentForm) {
      setNewAgentName(generateAgentName(selectedModel, codeAgentRuntime));
    }
  }, [selectedModel, codeAgentRuntime, userModifiedName, showCreateAgentForm]);

  // Load tasks and apps on mount
  useEffect(() => {
    if (account.user?.id) {
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
        setShowCreateAgentForm(false);
      } else if (sortedApps.length === 0) {
        // No agents exist, show create form
        setShowCreateAgentForm(true);
        setSelectedHelixAgent('');
      } else {
        // Agents exist but project has no default, select first zed_external
        setSelectedHelixAgent(sortedApps[0]?.id || '');
        setShowCreateAgentForm(false);
      }

      // Focus the text field when dialog opens
      setTimeout(() => {
        if (taskPromptRef.current) {
          taskPromptRef.current.focus();
        }
      }, 100);
    }
  }, [createDialogOpen, sortedApps, project?.default_helix_app_id]);

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
  // Note: dependencies include justDoItMode to ensure the handler captures the current value
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
  }, [createDialogOpen, taskPrompt, justDoItMode, selectedHelixAgent, useHostDocker]);

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

  // Keyboard shortcut: Ctrl/Cmd+J to toggle Just Do It mode
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key === 'j') {
        if (createDialogOpen) {
          e.preventDefault();
          setJustDoItMode(prev => !prev);
        }
      }
    };

    if (createDialogOpen) {
      window.addEventListener('keydown', handleKeyDown);
      return () => window.removeEventListener('keydown', handleKeyDown);
    }
  }, [createDialogOpen]);

  // Handle inline agent creation (same pattern as CreateProjectDialog)
  const handleCreateAgent = async (): Promise<string | null> => {
    if (!newAgentName.trim()) {
      setAgentError('Please enter a name for the agent');
      return null;
    }
    if (!selectedModel) {
      setAgentError('Please select a model');
      return null;
    }

    setCreatingAgent(true);
    setAgentError('');

    try {
      const params: ICreateAgentParams = {
        name: newAgentName.trim(),
        description: 'Code development agent for spec tasks',
        agentType: AGENT_TYPE_ZED_EXTERNAL,
        codeAgentRuntime,
        model: selectedModel,
        generationModelProvider: selectedProvider,
        generationModel: selectedModel,
        reasoningModelProvider: '',
        reasoningModel: '',
        reasoningModelEffort: 'none',
        smallReasoningModelProvider: '',
        smallReasoningModel: '',
        smallReasoningModelEffort: 'none',
        smallGenerationModelProvider: '',
        smallGenerationModel: '',
      };

      const newApp = await apps.createAgent(params);
      if (newApp) {
        return newApp.id;
      }
      setAgentError('Failed to create agent');
      return null;
    } catch (err) {
      console.error('Failed to create agent:', err);
      setAgentError(err instanceof Error ? err.message : 'Failed to create agent');
      return null;
    } finally {
      setCreatingAgent(false);
    }
  };

  // Handle task creation - SIMPLIFIED
  const handleCreateTask = async () => {
    if (!checkLoginStatus()) return;

    try {
      if (!taskPrompt.trim()) {
        snackbar.error('Please describe what you want to get done');
        return;
      }

      let agentId = selectedHelixAgent;
      setAgentError('');

      // Create agent inline if showing create form
      if (showCreateAgentForm) {
        const newAgentId = await handleCreateAgent();
        if (!newAgentId) {
          // Error already set in handleCreateAgent
          return;
        }
        agentId = newAgentId;
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
        // Reset inline agent creation form
        setShowCreateAgentForm(false);
        setCodeAgentRuntime('zed_agent');
        setSelectedProvider('');
        setSelectedModel('');
        setNewAgentName('-');
        setUserModifiedName(false);
        setAgentError('');

        // Set focusTaskId to focus the Start Planning button on the new task
        const newTaskId = response.data.id;
        if (newTaskId) {
          setFocusTaskId(newTaskId);
          // Clear focus after a few seconds so it doesn't persist
          setTimeout(() => setFocusTaskId(undefined), 5000);
        }

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
          {/* Project's default agent lozenge */}
          {project?.default_helix_app_id && appNamesMap[project.default_helix_app_id] && (
            <Tooltip title="Default agent for this project. Click to configure MCPs, skills, and knowledge.">
              <Chip
                label={appNamesMap[project.default_helix_app_id]}
                size="small"
                onClick={() => {
                  if (project.default_helix_app_id) {
                    account.orgNavigate('app', { app_id: project.default_helix_app_id });
                  }
                }}
                sx={{
                  flexShrink: 0,
                  cursor: 'pointer',
                  background: 'linear-gradient(145deg, rgba(120, 120, 140, 0.9) 0%, rgba(90, 90, 110, 0.95) 50%, rgba(70, 70, 90, 0.9) 100%)',
                  color: 'rgba(255, 255, 255, 0.9)',
                  fontWeight: 500,
                  fontSize: '0.75rem',
                  border: '1px solid rgba(255,255,255,0.12)',
                  boxShadow: 'inset 0 1px 0 rgba(255,255,255,0.15), 0 1px 3px rgba(0,0,0,0.2)',
                  '&:hover': {
                    background: 'linear-gradient(145deg, rgba(130, 130, 150, 0.95) 0%, rgba(100, 100, 120, 1) 50%, rgba(80, 80, 100, 0.95) 100%)',
                    boxShadow: 'inset 0 1px 0 rgba(255,255,255,0.2), 0 2px 4px rgba(0,0,0,0.25)',
                  },
                }}
              />
            </Tooltip>
          )}
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
              focusTaskId={focusTaskId}
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
          <Box sx={{ flex: 1, overflow: 'auto', p: 2 }}>
            <Stack spacing={2}>
              {/* Priority Selector */}
              <FormControl fullWidth size="small">
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

              {/* Single text box for everything */}
              <TextField
                label="Describe what you want to get done"
                fullWidth
                required
                multiline
                rows={9}
                value={taskPrompt}
                onChange={(e) => setTaskPrompt(e.target.value)}
                onKeyDown={(e) => {
                  // If user presses Enter in empty text box, close panel
                  if (e.key === 'Enter' && !taskPrompt.trim() && !e.shiftKey && !e.ctrlKey && !e.metaKey) {
                    e.preventDefault()
                    setCreateDialogOpen(false)
                  }
                }}
                placeholder={justDoItMode
                  ? "Describe what you want the agent to do. It will start immediately without planning."
                  : "Describe the task - the AI will generate specs from this."
                }
                helperText={justDoItMode
                  ? "Agent will start working immediately"
                  : "Planning agent extracts task name, description, and generates specifications"
                }
                inputRef={taskPromptRef}
                size="small"
              />

              {/* Agent Selection (compact) */}
              <Box>
                <Typography variant="caption" color="text.secondary" sx={{ mb: 0.5, display: 'block' }}>
                  Agent
                </Typography>
                {!showCreateAgentForm ? (
                  <>
                    {/* Existing agents list - compact style */}
                    {sortedApps.length > 0 ? (
                      <List dense sx={{ pt: 0, maxHeight: 120, overflow: 'auto' }}>
                        {sortedApps.map((app) => {
                          const isSelected = selectedHelixAgent === app.id;

                          return (
                            <ListItem
                              key={app.id}
                              disablePadding
                              secondaryAction={
                                <IconButton
                                  edge="end"
                                  size="small"
                                  onClick={(e) => {
                                    e.stopPropagation();
                                    account.orgNavigate('app', { app_id: app.id });
                                  }}
                                  sx={{ p: 0.5 }}
                                >
                                  <EditIcon sx={{ fontSize: 14 }} />
                                </IconButton>
                              }
                            >
                              <ListItemButton
                                selected={isSelected}
                                onClick={() => setSelectedHelixAgent(app.id)}
                                sx={{
                                  borderRadius: 0.5,
                                  py: 0.5,
                                  minHeight: 36,
                                  border: isSelected ? '1px solid' : '1px solid transparent',
                                  borderColor: isSelected ? 'primary.main' : 'transparent',
                                  bgcolor: isSelected ? 'action.selected' : 'transparent',
                                  pr: 4,
                                }}
                              >
                                <ListItemIcon sx={{ minWidth: 32 }}>
                                  <Avatar
                                    src={app.config?.helix?.avatar}
                                    sx={{ width: 24, height: 24, fontSize: 12 }}
                                  >
                                    <SmartToyIcon sx={{ fontSize: 14 }} />
                                  </Avatar>
                                </ListItemIcon>
                                <ListItemText
                                  primary={app.config?.helix?.name || 'Unnamed Agent'}
                                  primaryTypographyProps={{ variant: 'body2', noWrap: true }}
                                />
                              </ListItemButton>
                            </ListItem>
                          );
                        })}
                      </List>
                    ) : (
                      <Typography variant="caption" color="text.secondary">
                        No agents. Create one below.
                      </Typography>
                    )}
                    <Button
                      size="small"
                      onClick={() => setShowCreateAgentForm(true)}
                      sx={{ mt: 0.5, fontSize: '0.75rem' }}
                    >
                      + Create new agent
                    </Button>
                  </>
                ) : (
                  <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
                    <Typography variant="body2" color="text.secondary">
                      Code Agent Runtime
                    </Typography>
                    <FormControl fullWidth size="small">
                      <Select
                        value={codeAgentRuntime}
                        onChange={(e) => setCodeAgentRuntime(e.target.value as CodeAgentRuntime)}
                        disabled={creatingAgent}
                      >
                        <MenuItem value="zed_agent">
                          <Box>
                            <Typography variant="body2">Zed Agent (Built-in)</Typography>
                            <Typography variant="caption" color="text.secondary">
                              Uses Zed's native agent panel with direct API integration
                            </Typography>
                          </Box>
                        </MenuItem>
                        <MenuItem value="qwen_code">
                          <Box>
                            <Typography variant="body2">Qwen Code</Typography>
                            <Typography variant="caption" color="text.secondary">
                              Uses qwen-code CLI as a custom agent server (OpenAI-compatible)
                            </Typography>
                          </Box>
                        </MenuItem>
                      </Select>
                    </FormControl>

                    <Typography variant="body2" color="text.secondary">
                      Code Agent Model
                    </Typography>
                    <AdvancedModelPicker
                      recommendedModels={RECOMMENDED_MODELS}
                      hint="Choose a capable model for agentic coding."
                      selectedProvider={selectedProvider}
                      selectedModelId={selectedModel}
                      onSelectModel={(provider, model) => {
                        setSelectedProvider(provider);
                        setSelectedModel(model);
                      }}
                      currentType="text"
                      displayMode="short"
                      disabled={creatingAgent}
                    />

                    <Typography variant="body2" color="text.secondary">
                      Agent Name
                    </Typography>
                    <TextField
                      value={newAgentName}
                      onChange={(e) => {
                        setNewAgentName(e.target.value);
                        setUserModifiedName(true);
                      }}
                      size="small"
                      fullWidth
                      disabled={creatingAgent}
                      helperText="Auto-generated from model and runtime. Edit to customize."
                    />

                    {agentError && (
                      <Alert severity="error">{agentError}</Alert>
                    )}

                    {sortedApps.length > 0 && (
                      <Button
                        size="small"
                        onClick={() => setShowCreateAgentForm(false)}
                        sx={{ alignSelf: 'flex-start' }}
                        disabled={creatingAgent}
                      >
                        Back to agent list
                      </Button>
                    )}
                  </Box>
                )}
              </Box>

              {/* Just Do It Mode Checkbox */}
              <FormControl fullWidth>
                <Tooltip title={`Skip writing a spec and just get the agent to immediately start doing what you ask (${navigator.platform.includes('Mac') ? '⌘J' : 'Ctrl+J'})`} placement="top">
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
                        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                          <Typography variant="body2" sx={{ fontWeight: 600 }}>
                            Just Do It
                          </Typography>
                          <Box component="span" sx={{ fontSize: '0.65rem', opacity: 0.6, fontFamily: 'monospace', border: '1px solid', borderColor: 'divider', borderRadius: '3px', px: 0.5 }}>
                            {navigator.platform.includes('Mac') ? '⌘J' : 'Ctrl+J'}
                          </Box>
                        </Box>
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
              // Reset inline agent creation form
              setShowCreateAgentForm(false);
              setCodeAgentRuntime('zed_agent');
              setSelectedProvider('');
              setSelectedModel('');
              setNewAgentName('-');
              setUserModifiedName(false);
              setAgentError('');
            }}>
              Cancel
            </Button>
            <Button
              onClick={handleCreateTask}
              variant="contained"
              color="secondary"
              disabled={!taskPrompt.trim() || creatingAgent || (showCreateAgentForm && !selectedModel)}
              startIcon={creatingAgent ? <CircularProgress size={16} /> : <AddIcon />}
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
