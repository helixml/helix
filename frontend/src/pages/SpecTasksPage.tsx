import React, { FC, useState, useEffect, useRef, useMemo, useCallback } from 'react';
import { useQuery } from '@tanstack/react-query';
import {
  Box,
  Button,
  Typography,
  Alert,
  Chip,
  Stack,  
  TextField,
  FormControl,
  InputLabel,
  Select,
  MenuItem,
  CircularProgress,
  IconButton,  
  Checkbox,
  FormControlLabel,
  Tooltip,
} from '@mui/material';
import {
  Add as AddIcon,
  Refresh as RefreshIcon,
  Explore as ExploreIcon,
  Stop as StopIcon,
  SmartToy as SmartToyIcon,
  ViewKanban as KanbanIcon,
  History as AuditIcon,
  Tab as TabIcon,  
} from '@mui/icons-material';
import { Plus, X, Play, Settings } from 'lucide-react';

import Page from '../components/system/Page';
import SpecTaskKanbanBoard from '../components/tasks/SpecTaskKanbanBoard';
import SpecTaskDetailDialog from '../components/tasks/SpecTaskDetailDialog';
import ProjectAuditTrail from '../components/tasks/ProjectAuditTrail';
import TabsView from '../components/tasks/TabsView';
import PreviewPanel from '../components/app/PreviewPanel';
import { AdvancedModelPicker } from '../components/create/AdvancedModelPicker';
import { CodeAgentRuntime, generateAgentName, ICreateAgentParams } from '../contexts/apps';
import { AGENT_TYPE_ZED_EXTERNAL, IApp, SESSION_TYPE_TEXT } from '../types';
import { useStreaming } from '../contexts/streaming';
import { TypesSession } from '../api/api';

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
import { useListSessions, useGetSession } from '../services/sessionService';
import { TypesSpecTask, TypesCreateTaskRequest, TypesSpecTaskPriority, TypesBranchMode } from '../api/api';
import AgentDropdown from '../components/agent/AgentDropdown';

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

  // Query sandbox instances to check for privileged mode availability
  const { data: sandboxInstances } = useQuery({
    queryKey: ['sandbox-instances'],
    queryFn: async () => {
      // API endpoint still named WolfInstances for backwards compatibility
      const response = await api.getApiClient().v1WolfInstancesList();
      return response.data;
    },
    staleTime: 60000, // Cache for 1 minute
  });

  // Check if any sandbox has privileged mode enabled
  const hasPrivilegedSandbox = useMemo(() => {
    return sandboxInstances?.some(instance => instance.privileged_mode_enabled) ?? false;
  }, [sandboxInstances]);

  // Redirect to projects list if no project selected (new architecture: must select project first)
  // Exception: if user is trying to create a new task (new=true param), allow it for backward compat
  useEffect(() => {
    const isCreatingNew = router.params.new === 'true';
    if (!projectId && !isCreatingNew) {
      console.log('No project ID in route - redirecting to projects list');
      account.orgNavigate('projects');
    }
  }, [projectId, router.params.new, account]);

  // State for view management - persist preference in localStorage
  const [viewMode, setViewMode] = useState<'kanban' | 'tabs' | 'audit'>(() => {
    const saved = localStorage.getItem('helix_spectask_view_mode');
    if (saved === 'kanban' || saved === 'tabs' || saved === 'audit') {
      return saved;
    }
    return 'kanban';
  });

  // Persist view mode preference when it changes
  useEffect(() => {
    localStorage.setItem('helix_spectask_view_mode', viewMode);
  }, [viewMode]);

  const [createDialogOpen, setCreateDialogOpen] = useState(false);
  const [refreshing, setRefreshing] = useState(false);
  const [refreshTrigger, setRefreshTrigger] = useState(0);

  // Chat panel state - persist expanded/collapsed preference
  const [chatPanelOpen, setChatPanelOpen] = useState(() => {
    const saved = localStorage.getItem('helix_chat_panel_open');
    return saved === 'true';
  });
  const [chatInputValue, setChatInputValue] = useState('');
  const [chatSession, setChatSession] = useState<TypesSession | undefined>();
  const [chatIsSearchMode, setChatIsSearchMode] = useState(false);
  const [chatLoading, setChatLoading] = useState(false);
  const { NewInference, setCurrentSessionId } = useStreaming();

  // Selected session ID for persistence across reloads
  const [selectedSessionId, setSelectedSessionId] = useState<string | undefined>(() => {
    if (!projectId) return undefined;
    return localStorage.getItem(`helix_chat_session_${projectId}`) || undefined;
  });
  const [isNewChatMode, setIsNewChatMode] = useState(false);

  // Reset selected session when project changes
  useEffect(() => {
    if (projectId) {
      const stored = localStorage.getItem(`helix_chat_session_${projectId}`);
      setSelectedSessionId(stored || undefined);
      setChatSession(undefined);
    }
  }, [projectId]);

  // Sync selectedSessionId with URL params (for page refresh or direct URL navigation)
  useEffect(() => {
    const urlSessionId = router.params.session_id;
    if (urlSessionId && urlSessionId !== selectedSessionId) {
      setSelectedSessionId(urlSessionId);
      setIsNewChatMode(false);
    }
  }, [router.params.session_id, selectedSessionId]);

  // Persist chat panel open/closed preference when it changes
  useEffect(() => {
    localStorage.setItem('helix_chat_panel_open', chatPanelOpen ? 'true' : 'false');
  }, [chatPanelOpen]);

  // Fetch tasks for TabsView
  const { data: tasksData } = useQuery({
    queryKey: ['spec-tasks', projectId, refreshTrigger],
    queryFn: async () => {
      const response = await api.getApiClient().v1SpecTasksList({
        project_id: projectId || 'default',
      });
      return response.data || [];
    },
    enabled: !!projectId && viewMode === 'tabs',
    refetchInterval: 3000, // Refresh every 3 seconds for live updates
  });

  // Create task form state (SIMPLIFIED)
  const [taskPrompt, setTaskPrompt] = useState(''); // Single text box for everything
  const [taskPriority, setTaskPriority] = useState('medium');
  const [selectedHelixAgent, setSelectedHelixAgent] = useState('');
  const [justDoItMode, setJustDoItMode] = useState(false); // Just Do It mode: skip spec, go straight to implementation
  const [useHostDocker, setUseHostDocker] = useState(false); // Use host Docker socket (requires privileged sandbox)

  // Branch configuration state
  const [branchMode, setBranchMode] = useState<TypesBranchMode>(TypesBranchMode.BranchModeNew);
  const [baseBranch, setBaseBranch] = useState('');
  const [branchPrefix, setBranchPrefix] = useState('');
  const [workingBranch, setWorkingBranch] = useState('');
  const [showBranchCustomization, setShowBranchCustomization] = useState(false);

  // Get the default repository ID from the project
  const defaultRepoId = project?.default_repo_id;

  // Fetch branches for the default repository
  const { data: branchesData } = useQuery({
    queryKey: ['repository-branches', defaultRepoId],
    queryFn: async () => {
      if (!defaultRepoId) return [];
      const response = await api.getApiClient().listGitRepositoryBranches(defaultRepoId);
      return response.data || [];
    },
    enabled: !!defaultRepoId && createDialogOpen,
    staleTime: 30000, // Cache for 30 seconds
  });

  // Get the default branch name from the repository
  const defaultBranchName = useMemo(() => {
    const defaultRepo = projectRepositories.find(r => r.id === defaultRepoId);
    return defaultRepo?.default_branch || 'main';
  }, [projectRepositories, defaultRepoId]);

  // Check if the default repo is an external repo (e.g., Azure DevOps)
  const hasExternalRepo = useMemo(() => {
    const defaultRepo = projectRepositories.find(r => r.id === defaultRepoId);
    return !!(defaultRepo?.azure_devops || defaultRepo?.external_type);
  }, [projectRepositories, defaultRepoId]);

  // Set baseBranch to default when dialog opens
  useEffect(() => {
    if (createDialogOpen && defaultBranchName && !baseBranch) {
      setBaseBranch(defaultBranchName);
    }
  }, [createDialogOpen, defaultBranchName, baseBranch]);

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

  // Get display settings from the project's default app for exploratory sessions
  const exploratoryDisplaySettings = useMemo(() => {
    if (!project?.default_helix_app_id || !apps.apps) {
      return { width: 1920, height: 1080, fps: 60 }
    }
    const defaultApp = apps.apps.find(a => a.id === project.default_helix_app_id)
    const config = defaultApp?.config?.helix?.external_agent_config
    if (!config) {
      return { width: 1920, height: 1080, fps: 60 }
    }

    // Get dimensions from resolution preset or explicit values
    let width = config.display_width || 1920
    let height = config.display_height || 1080
    if (config.resolution === '5k') {
      width = 5120
      height = 2880
    } else if (config.resolution === '4k') {
      width = 3840
      height = 2160
    } else if (config.resolution === '1080p') {
      width = 1920
      height = 1080
    }

    return {
      width,
      height,
      fps: config.display_refresh_rate || 60,
    }
  }, [project?.default_helix_app_id, apps.apps])

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
        // Also close chat panel when opening create dialog
        if (!createDialogOpen) {
          setChatPanelOpen(false);
        }
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

  // Keyboard shortcut: ESC to close create task panel or chat panel
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        if (chatPanelOpen) {
          e.preventDefault();
          setChatPanelOpen(false);
        } else if (createDialogOpen) {
          e.preventDefault();
          setCreateDialogOpen(false);
        }
      }
    };

    if (createDialogOpen || chatPanelOpen) {
      window.addEventListener('keydown', handleKeyDown);
      return () => window.removeEventListener('keydown', handleKeyDown);
    }
  }, [createDialogOpen, chatPanelOpen]);

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
      const createTaskRequest: TypesCreateTaskRequest = {
        prompt: taskPrompt, // Just the raw user input!
        priority: taskPriority as TypesSpecTaskPriority,
        project_id: projectId || 'default', // Use project ID from route, or 'default'
        app_id: agentId || undefined, // Include selected or created agent if provided
        just_do_it_mode: justDoItMode, // Just Do It mode: skip spec, go straight to implementation
        use_host_docker: useHostDocker, // Use host Docker socket (requires privileged sandbox)
        // Branch configuration
        branch_mode: branchMode,
        base_branch: branchMode === TypesBranchMode.BranchModeNew ? baseBranch : undefined,
        branch_prefix: branchMode === TypesBranchMode.BranchModeNew && branchPrefix ? branchPrefix : undefined,
        working_branch: branchMode === TypesBranchMode.BranchModeExisting ? workingBranch : undefined,
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
        // Reset branch configuration
        setBranchMode(TypesBranchMode.BranchModeNew);
        setBaseBranch(defaultBranchName);
        setBranchPrefix('');
        setWorkingBranch('');
        setShowBranchCustomization(false);
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
        displayWidth: exploratoryDisplaySettings.width,
        displayHeight: exploratoryDisplaySettings.height,
        displayFps: exploratoryDisplaySettings.fps,
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
        displayWidth: exploratoryDisplaySettings.width,
        displayHeight: exploratoryDisplaySettings.height,
        displayFps: exploratoryDisplaySettings.fps,
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

  const projectManagerAppId = project?.project_manager_helix_app_id || '';
  const projectManagerApp = useMemo(() => {
    return apps.apps.find(app => app.id === projectManagerAppId);
  }, [apps.apps, projectManagerAppId]);

  // Persist selected session ID to localStorage
  useEffect(() => {
    if (projectId && selectedSessionId) {
      localStorage.setItem(`helix_chat_session_${projectId}`, selectedSessionId);
    }
  }, [projectId, selectedSessionId]);

  // Fetch last 5 sessions for this project (filtered by project manager app)
  const { data: projectSessionsData } = useListSessions(
    undefined,
    undefined,
    undefined,
    projectId,
    0,
    5,
    { enabled: !!projectId && !!projectManagerAppId },
    projectManagerAppId
  );

  const projectSessions = projectSessionsData?.data?.sessions || [];

  // Load the selected session details
  const { data: loadedSessionData } = useGetSession(
    selectedSessionId || '',
    { enabled: !!selectedSessionId && chatPanelOpen }
  );

  // When session data loads, set it as the chat session
  useEffect(() => {
    if (loadedSessionData?.data && chatPanelOpen && selectedSessionId) {
      setChatSession(loadedSessionData.data);
    }
  }, [loadedSessionData?.data, chatPanelOpen, selectedSessionId]);

  // Auto-select the most recent session when panel opens and no session is selected (unless user wants new chat)
  useEffect(() => {
    if (chatPanelOpen && !selectedSessionId && !isNewChatMode && projectSessions.length > 0) {
      const mostRecentSession = projectSessions[0];
      if (mostRecentSession?.session_id) {
        setSelectedSessionId(mostRecentSession.session_id);
      }
    }
  }, [chatPanelOpen, selectedSessionId, isNewChatMode, projectSessions]);

  const handleChatInference = useCallback(async () => {
    if (!chatInputValue.trim() || !projectManagerAppId) return;

    setChatLoading(true);
    try {
      const messagePayloadContent = {
        content_type: "text",
        parts: [
          {
            type: "text",
            text: chatInputValue,
          }
        ],
      };

      setChatInputValue('');

      const newSessionData = await NewInference({
        message: '',
        messages: [
          {
            role: 'user',
            content: messagePayloadContent as any,
          }
        ],
        appId: projectManagerAppId,
        projectId: projectId,
        type: SESSION_TYPE_TEXT,
      });

      setChatSession(newSessionData);
    } catch (error) {
      console.error('Chat inference error:', error);
      snackbar.error('Failed to send message');
    } finally {
      setChatLoading(false);
    }
  }, [chatInputValue, projectManagerAppId, projectId, NewInference, snackbar]);

  const handleChatSessionUpdate = useCallback((session: TypesSession) => {
    setChatSession(session);
    if (session?.id) {
      setSelectedSessionId(session.id);
      setIsNewChatMode(false);
    }
  }, []);

  const handleOpenChatPanel = useCallback(() => {
    setCreateDialogOpen(false);
    setChatPanelOpen(true);
    setChatInputValue('');
  }, []);

  const handleNewChat = useCallback(() => {
    setChatSession(undefined);
    setSelectedSessionId(undefined);
    setIsNewChatMode(true);
    setChatInputValue('');
    if (projectId) {
      localStorage.removeItem(`helix_chat_session_${projectId}`);
    }
    router.removeParams(['session_id']);
  }, [projectId, router]);

  const handleSelectSession = useCallback((sessionId: string) => {
    setSelectedSessionId(sessionId);
    setIsNewChatMode(false);
    router.setParams({ session_id: sessionId });
  }, [router]);

  const truncateTitle = (title: string | undefined, maxLength: number = 15): string => {
    if (!title) return 'Untitled';
    return title.length > maxLength ? title.substring(0, maxLength) + '...' : title;
  };

  const handleOpenCreateDialog = useCallback(() => {
    setChatPanelOpen(false);
    setCreateDialogOpen(true);
  }, []);

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
          {/* View mode toggle: Kanban vs Tabs vs Audit Trail */}
          <Stack direction="row" spacing={0.5} sx={{ borderRadius: 1, p: 0.5 }}>
            <Tooltip title="Kanban View">
              <IconButton
                size="small"
                onClick={() => setViewMode('kanban')}
                sx={{
                  bgcolor: viewMode === 'kanban' ? 'background.paper' : 'transparent',
                  boxShadow: viewMode === 'kanban' ? 1 : 0,
                  '&:hover': { bgcolor: viewMode === 'kanban' ? 'background.paper' : 'action.selected' },
                }}
              >
                <KanbanIcon fontSize="small" color={viewMode === 'kanban' ? 'primary' : 'inherit'} />
              </IconButton>
            </Tooltip>
            <Tooltip title="Tabs View">
              <IconButton
                size="small"
                onClick={() => setViewMode('tabs')}
                sx={{
                  bgcolor: viewMode === 'tabs' ? 'background.paper' : 'transparent',
                  boxShadow: viewMode === 'tabs' ? 1 : 0,
                  '&:hover': { bgcolor: viewMode === 'tabs' ? 'background.paper' : 'action.selected' },
                }}
              >
                <TabIcon fontSize="small" color={viewMode === 'tabs' ? 'primary' : 'inherit'} />
              </IconButton>
            </Tooltip>
            <Tooltip title="Audit Trail">
              <IconButton
                size="small"
                onClick={() => setViewMode('audit')}
                sx={{
                  bgcolor: viewMode === 'audit' ? 'background.paper' : 'transparent',
                  boxShadow: viewMode === 'audit' ? 1 : 0,
                  '&:hover': { bgcolor: viewMode === 'audit' ? 'background.paper' : 'action.selected' },
                }}
              >
                <AuditIcon fontSize="small" color={viewMode === 'audit' ? 'primary' : 'inherit'} />
              </IconButton>
            </Tooltip>
          </Stack>

          {/* Project's default agent lozenge */}
          {project?.default_helix_app_id && appNamesMap[project.default_helix_app_id] && (
            <Tooltip title="Default agent for this project. Click to configure MCPs, skills, and knowledge.">
              <Box
                onClick={() => {
                  if (project.default_helix_app_id) {
                    account.orgNavigate('app', { app_id: project.default_helix_app_id });
                  }
                }}
                sx={{
                  px: 1.5,
                  py: 0.5,
                  fontSize: '0.75rem',
                  cursor: 'pointer',
                  backgroundColor: 'transparent',
                  borderRadius: 0,
                  '&:hover': {
                    backgroundColor: 'rgba(255, 255, 255, 0.05)',
                  },
                }}
              >
                {appNamesMap[project.default_helix_app_id]}
              </Box>
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
              startIcon={<Play size={18} />}
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
                startIcon={<Play size={18} />}
                onClick={(e) => {
                  floatingModal.showFloatingModal({
                    type: 'exploratory_session',
                    sessionId: exploratorySessionData.id,
                    displayWidth: exploratoryDisplaySettings.width,
                    displayHeight: exploratoryDisplaySettings.height,
                    displayFps: exploratoryDisplaySettings.fps,
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
          {projectManagerAppId && (
            <Tooltip title="Chat with Project Manager agent">
              <Button
                variant="outlined"
                startIcon={<Plus size={18} />}
                onClick={handleOpenChatPanel}
                sx={{ flexShrink: 0 }}
              >
                New Chat
              </Button>
            </Tooltip>
          )}
          <Button
            variant="outlined"
            startIcon={<Settings size={18} />}
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

          {/* Main Content: Kanban Board, Tabs View, or Audit Trail */}
          <Box sx={{ flex: 1, minHeight: 0, minWidth: 0, display: 'flex', flexDirection: 'column', overflowX: 'hidden' }}>
            {viewMode === 'kanban' && (
              <SpecTaskKanbanBoard
                userId={account.user?.id}
                projectId={projectId}
                onCreateTask={handleOpenCreateDialog}
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
                hasExternalRepo={hasExternalRepo}
              />
            )}
            {viewMode === 'tabs' && (
              <TabsView
                projectId={projectId}
                tasks={tasksData || []}
                onCreateTask={handleOpenCreateDialog}
                onRefresh={() => setRefreshTrigger(prev => prev + 1)}
              />
            )}
            {viewMode === 'audit' && (
              <ProjectAuditTrail
                projectId={projectId || ''}
                onTaskClick={(taskId) => {
                  // Fetch task and open in dialog
                  // For now, we just need the ID to add a placeholder
                  setOpenTaskWindows(prev => {
                    const alreadyOpen = prev.some(t => t.id === taskId);
                    if (alreadyOpen) return prev;
                    // Create minimal task object for dialog
                    return [...prev, { id: taskId } as TypesSpecTask];
                  });
                }}
              />
            )}
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
              <IconButton onClick={() => setCreateDialogOpen(false)}>
                <X size={20} />
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

              {/* Branch Configuration */}
              {defaultRepoId && (
                <Box sx={{ border: 1, borderColor: 'divider', borderRadius: 1, p: 2 }}>
                  <Typography variant="subtitle2" gutterBottom>
                    Where do you want to work?
                  </Typography>

                  {/* Mode Selection - Two Cards */}
                  <Box sx={{ display: 'flex', gap: 1, mb: 2 }}>
                    <Box
                      onClick={() => setBranchMode(TypesBranchMode.BranchModeNew)}
                      sx={{
                        flex: 1,
                        p: 1.5,
                        border: 2,
                        borderColor: branchMode === TypesBranchMode.BranchModeNew ? 'primary.main' : 'divider',
                        borderRadius: 1,
                        cursor: 'pointer',
                        bgcolor: branchMode === TypesBranchMode.BranchModeNew ? 'action.selected' : 'transparent',
                        '&:hover': { bgcolor: 'action.hover' },
                      }}
                    >
                      <Typography variant="body2" fontWeight={600}>
                        Start fresh
                      </Typography>
                      <Typography variant="caption" color="text.secondary">
                        Create a new branch
                      </Typography>
                    </Box>
                    <Box
                      onClick={() => setBranchMode(TypesBranchMode.BranchModeExisting)}
                      sx={{
                        flex: 1,
                        p: 1.5,
                        border: 2,
                        borderColor: branchMode === TypesBranchMode.BranchModeExisting ? 'primary.main' : 'divider',
                        borderRadius: 1,
                        cursor: 'pointer',
                        bgcolor: branchMode === TypesBranchMode.BranchModeExisting ? 'action.selected' : 'transparent',
                        '&:hover': { bgcolor: 'action.hover' },
                      }}
                    >
                      <Typography variant="body2" fontWeight={600}>
                        Continue existing
                      </Typography>
                      <Typography variant="caption" color="text.secondary">
                        Resume work on a branch
                      </Typography>
                    </Box>
                  </Box>

                  {/* Mode-specific options */}
                  {branchMode === TypesBranchMode.BranchModeNew ? (
                    <Stack spacing={1.5}>
                      {!showBranchCustomization ? (
                        <Box>
                          <Typography variant="caption" color="text.secondary">
                            New branch from <strong>{baseBranch || defaultBranchName}</strong>
                          </Typography>
                          <Button
                            size="small"
                            onClick={() => setShowBranchCustomization(true)}
                            sx={{ display: 'block', textTransform: 'none', fontSize: '0.75rem', p: 0, mt: 0.5 }}
                          >
                            Customize branches
                          </Button>
                        </Box>
                      ) : (
                        <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1.5 }}>
                          <Box>
                            <FormControl fullWidth size="small">
                              <InputLabel>Base branch</InputLabel>
                              <Select
                                value={baseBranch}
                                onChange={(e) => setBaseBranch(e.target.value)}
                                label="Base branch"
                              >
                                {branchesData?.map((branch: string) => (
                                  <MenuItem key={branch} value={branch}>
                                    {branch}
                                    {branch === defaultBranchName && (
                                      <Chip label="default" size="small" sx={{ ml: 1, height: 18 }} />
                                    )}
                                  </MenuItem>
                                ))}
                              </Select>
                            </FormControl>
                            <Typography variant="caption" color="text.secondary" sx={{ mt: 0.5, ml: 1.75, display: 'block' }}>
                              New branch will be created from this base. Use to build on existing work.
                            </Typography>
                          </Box>
                          <TextField
                            label="Working branch name"
                            size="small"
                            fullWidth
                            value={branchPrefix}
                            onChange={(e) => setBranchPrefix(e.target.value)}
                            placeholder="feature/user-auth"
                            helperText={branchPrefix
                              ? `Work will be done on: ${branchPrefix}-{task#}`
                              : "Leave empty to auto-generate. This is where the agent commits changes."
                            }
                          />
                          <Button
                            size="small"
                            onClick={() => {
                              setShowBranchCustomization(false);
                              setBaseBranch(defaultBranchName);
                              setBranchPrefix('');
                            }}
                            sx={{ alignSelf: 'flex-start', textTransform: 'none', fontSize: '0.75rem' }}
                          >
                            Use defaults
                          </Button>
                        </Box>
                      )}
                    </Stack>
                  ) : (
                    <FormControl fullWidth size="small">
                      <InputLabel>Select branch</InputLabel>
                      <Select
                        value={workingBranch}
                        onChange={(e) => setWorkingBranch(e.target.value)}
                        label="Select branch"
                      >
                        {branchesData
                          ?.filter((branch: string) => branch !== defaultBranchName)
                          .map((branch: string) => (
                            <MenuItem key={branch} value={branch}>
                              {branch}
                            </MenuItem>
                          ))}
                        {branchesData?.filter((branch: string) => branch !== defaultBranchName).length === 0 && (
                          <MenuItem disabled value="">
                            No feature branches available
                          </MenuItem>
                        )}
                      </Select>
                    </FormControl>
                  )}
                </Box>
              )}

              {/* Agent Selection (dropdown) */}
              <Box>
                {!showCreateAgentForm ? (
                  <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1 }}>
                    <AgentDropdown
                      value={selectedHelixAgent}
                      onChange={setSelectedHelixAgent}
                      agents={sortedApps}
                    />
                    <Button
                      size="small"
                      onClick={() => setShowCreateAgentForm(true)}
                      sx={{ alignSelf: 'flex-start', textTransform: 'none', fontSize: '0.75rem' }}
                    >
                      + Create new agent
                    </Button>
                  </Box>
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
              // Reset branch configuration
              setBranchMode(TypesBranchMode.BranchModeNew);
              setBaseBranch(defaultBranchName);
              setBranchPrefix('');
              setWorkingBranch('');
              setShowBranchCustomization(false);
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

        {/* RIGHT PANEL: Chat with Project Manager Agent */}
        {projectManagerAppId && (
          <Box
            sx={{
              width: chatPanelOpen ? { xs: '100%', sm: '450px', md: '500px' } : 0,
              flexShrink: 0,
              overflow: 'hidden',
              transition: 'width 0.3s ease-in-out',
              borderLeft: chatPanelOpen ? 1 : 0,
              borderColor: 'divider',
              display: 'flex',
              flexDirection: 'column',
              backgroundColor: 'background.paper',
            }}
          >
            <Box sx={{ height: '100%', display: 'flex', flexDirection: 'column', position: 'relative' }}>
              {/* Header with session tabs */}
              <Box sx={{ 
                display: 'flex', 
                alignItems: 'center', 
                gap: 0.5, 
                p: 1, 
                borderBottom: 1, 
                borderColor: 'divider',
                backgroundColor: 'background.paper',
              }}>
                {/* Horizontal scrollable session list */}
                <Box sx={{ 
                  flex: 1, 
                  overflow: 'hidden',
                  display: 'flex',
                  alignItems: 'center',
                }}>
                  <Box sx={{ 
                    display: 'flex', 
                    gap: 0.5, 
                    overflowX: 'auto',
                    whiteSpace: 'nowrap',
                    '&::-webkit-scrollbar': {
                      height: '4px',
                    },
                    '&::-webkit-scrollbar-track': {
                      background: 'transparent',
                    },
                    '&::-webkit-scrollbar-thumb': {
                      background: 'rgba(255, 255, 255, 0.2)',
                      borderRadius: '2px',
                    },
                    '&::-webkit-scrollbar-thumb:hover': {
                      background: 'rgba(255, 255, 255, 0.3)',
                    },
                  }}>
                    {projectSessions.map((session) => (
                      <Box
                        key={session.session_id}
                        onClick={() => session.session_id && handleSelectSession(session.session_id)}
                        sx={{
                          px: 1.5,
                          py: 0.5,
                          fontSize: '0.75rem',
                          cursor: 'pointer',
                          backgroundColor: selectedSessionId === session.session_id 
                            ? 'rgba(255, 255, 255, 0.12)' 
                            : 'transparent',
                          '&:hover': {
                            backgroundColor: selectedSessionId === session.session_id 
                              ? 'rgba(255, 255, 255, 0.15)' 
                              : 'rgba(255, 255, 255, 0.05)',
                          },
                          maxWidth: '120px',
                          overflow: 'hidden',
                          textOverflow: 'ellipsis',
                        }}
                      >
                        {truncateTitle(session.name || session.summary)}
                      </Box>
                    ))}
                  </Box>
                </Box>

                {/* New chat button */}
                <Tooltip title="New chat">
                  <IconButton 
                    onClick={handleNewChat}
                    size="small"
                    sx={{ 
                      flexShrink: 0,
                    }}
                  >
                    <Plus size={18} />
                  </IconButton>
                </Tooltip>
                                
                <Tooltip title="Close">
                  <IconButton 
                    onClick={() => setChatPanelOpen(false)}
                    size="small"
                    sx={{ 
                      flexShrink: 0,
                    }}
                  >
                    <X size={18} />
                  </IconButton>
                </Tooltip>
              </Box>

              {/* Chat Content - Use PreviewPanel */}
              <Box sx={{ flex: 1, overflow: 'hidden', display: 'flex', width: '100%' }}>
                <PreviewPanel
                  appId={projectManagerAppId}
                  loading={chatLoading}
                  name={projectManagerApp?.config?.helix?.name || 'Project Manager'}
                  avatar={projectManagerApp?.config?.helix?.avatar || ''}
                  image=""
                  isSearchMode={chatIsSearchMode}
                  setIsSearchMode={setChatIsSearchMode}
                  inputValue={chatInputValue}
                  setInputValue={setChatInputValue}
                  onInference={handleChatInference}
                  onSearch={() => {}}
                  hasKnowledgeSources={false}
                  searchResults={[]}
                  session={chatSession}
                  serverConfig={account.serverConfig}
                  themeConfig={{}}
                  snackbar={snackbar}
                  conversationStarters={projectManagerApp?.config?.helix?.assistants?.[0]?.conversation_starters || []}
                  app={projectManagerApp}
                  onSessionUpdate={handleChatSessionUpdate}
                  hideSearchMode={true}
                  noBackground={true}
                  fullWidth={true}
                  hideHeader={true}
                />
              </Box>
            </Box>
          </Box>
        )}

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
        />
      ))}

    </Page>
  );
};

export default SpecTasksPage;
