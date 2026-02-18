import React, { FC, useState, useEffect, useMemo, useCallback } from 'react';
import { useQuery } from '@tanstack/react-query';
import {
  Box,
  Button,
  Typography,
  Alert,
  Stack,
  MenuItem,
  Menu,
  IconButton,
  Tooltip,
  useMediaQuery,
  useTheme,
} from '@mui/material';
import {
  Add as AddIcon,
  Refresh as RefreshIcon,
  Explore as ExploreIcon,
  Stop as StopIcon,
  ViewKanban as KanbanIcon,
  History as AuditIcon,
  Tab as TabIcon,
  Archive as ArchiveIcon,
  BarChart as MetricsIcon,
  Visibility as ViewIcon,
} from '@mui/icons-material';
import { Plus, X, Play, Settings, MoreHorizontal, FolderOpen, GitMerge, UserPlus } from 'lucide-react';

import Page from '../components/system/Page';
import SpecTaskKanbanBoard from '../components/tasks/SpecTaskKanbanBoard';
import ProjectAuditTrail from '../components/tasks/ProjectAuditTrail';
import TabsView from '../components/tasks/TabsView';
import PreviewPanel from '../components/app/PreviewPanel';
import SpecTasksMobileBottomNav from '../components/tasks/SpecTasksMobileBottomNav';
import NewSpecTaskForm from '../components/tasks/NewSpecTaskForm';
import { SESSION_TYPE_TEXT } from '../types';
import { useStreaming } from '../contexts/streaming';
import { TypesSession, TypesSpecTask } from '../api/api';

import useAccount from '../hooks/useAccount';
import useApi from '../hooks/useApi';
import useSnackbar from '../hooks/useSnackbar';
import useRouter from '../hooks/useRouter';
import useApps from '../hooks/useApps';
import EditIcon from '@mui/icons-material/Edit';
import {
  useGetProject,
  useGetProjectRepositories,
  useGetProjectExploratorySession,
  useStartProjectExploratorySession,
  useStopProjectExploratorySession,
  useResumeProjectExploratorySession,
  useGetStartupScriptHistory,
} from '../services';
import { useListSessions, useGetSession } from '../services/sessionService';
import {
  useListProjectAccessGrants,
  useCreateProjectAccessGrant,
  useDeleteProjectAccessGrant,
} from '../services/projectAccessGrantService';
import ProjectMembersBar from '../components/project/ProjectMembersBar';

const SpecTasksPage: FC = () => {
  const account = useAccount();
  const api = useApi();
  const snackbar = useSnackbar();
  const router = useRouter();
  const apps = useApps();
  const theme = useTheme();
  const isMobile = useMediaQuery(theme.breakpoints.down('md'));

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

  // Access grants for invite dialog
  const { data: accessGrants = [] } = useListProjectAccessGrants(
    projectId || '',
    !!project?.organization_id,
  );
  const createAccessGrantMutation = useCreateProjectAccessGrant(projectId || '');
  const deleteAccessGrantMutation = useDeleteProjectAccessGrant(projectId || '');

  // Startup script history - used to detect if user has modified the default script
  // Only fetch when we have a project with a default repo
  const hasDefaultRepo = !!project?.default_repo_id;
  const { data: startupScriptHistory, isSuccess: isStartupScriptHistoryLoaded } = useGetStartupScriptHistory(projectId || '', hasDefaultRepo);
  // Script is considered "not configured" if there's only 1 commit (the initial auto-generated script)
  const startupScriptNotConfigured = hasDefaultRepo && (startupScriptHistory?.length ?? 0) <= 1;

  // Redirect to projects list if no project selected (new architecture: must select project first)
  // Exception: if user is trying to create a new task (new=true param), allow it for backward compat
  useEffect(() => {
    const isCreatingNew = router.params.new === 'true';
    if (!projectId && !isCreatingNew) {
      console.log('No project ID in route - redirecting to projects list');
      account.orgNavigate('projects');
    }
  }, [projectId, router.params.new, account]);

  // Read query params for view mode override and task/desktop/review opening
  const queryTab = router.params.tab as string | undefined;
  const openTaskId = router.params.openTask as string | undefined;
  const openDesktopId = router.params.openDesktop as string | undefined;
  const openReviewId = router.params.openReview as string | undefined;
  const inviteOpen = router.params.invite === 'true';

  // State for view management - always default to kanban, but respect query param
  const [viewMode, setViewMode] = useState<'kanban' | 'workspace' | 'audit'>(() => {
    // Check query param - allows "Split Screen" links to work
    if (queryTab === 'workspace' || queryTab === 'kanban' || queryTab === 'audit') {
      return queryTab;
    }
    // Always default to kanban (no localStorage persistence - user prefers fresh start)
    return 'kanban';
  });

  // Update view mode if query param changes
  useEffect(() => {
    if (queryTab === 'workspace' || queryTab === 'kanban' || queryTab === 'audit') {
      setViewMode(queryTab);
    }
  }, [queryTab]);

  // On mobile, fallback to kanban if workspace is selected (workspace doesn't work on small screens)
  useEffect(() => {
    if (isMobile && viewMode === 'workspace') {
      setViewMode('kanban');
    }
  }, [isMobile, viewMode]);

  const [createDialogOpen, setCreateDialogOpen] = useState(false);
  const [refreshing, setRefreshing] = useState(false);
  const [refreshTrigger, setRefreshTrigger] = useState(0);

  // Kanban view options state (controlled from topbar)
  const METRICS_STORAGE_KEY = 'helix-kanban-show-metrics';
  const MERGED_STORAGE_KEY = 'helix-kanban-show-merged';
  const [showArchived, setShowArchived] = useState(false);
  const [showMetrics, setShowMetrics] = useState(() => {
    const stored = localStorage.getItem(METRICS_STORAGE_KEY);
    return stored !== null ? stored === 'true' : true;
  });
  const [showMerged, setShowMerged] = useState(() => {
    const stored = localStorage.getItem(MERGED_STORAGE_KEY);
    return stored !== null ? stored === 'true' : true;
  });
  const [viewMenuAnchorEl, setViewMenuAnchorEl] = useState<null | HTMLElement>(null);

  const handleToggleMetrics = useCallback(() => {
    setShowMetrics(prev => {
      const newValue = !prev;
      localStorage.setItem(METRICS_STORAGE_KEY, String(newValue));
      return newValue;
    });
  }, []);

  const handleToggleMerged = useCallback(() => {
    setShowMerged(prev => {
      const newValue = !prev;
      localStorage.setItem(MERGED_STORAGE_KEY, String(newValue));
      return newValue;
    });
  }, []);

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
  const [selectedSessionId, setSelectedSessionId] = useState<string | undefined>();
  const [isNewChatMode, setIsNewChatMode] = useState(true);

  // Reset to fresh chat when project changes
  useEffect(() => {
    if (projectId) {
      setSelectedSessionId(undefined);
      setChatSession(undefined);
      setIsNewChatMode(true);
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

  // Fetch tasks for Workspace view
  const { data: tasksData } = useQuery({
    queryKey: ['spec-tasks', projectId, refreshTrigger],
    queryFn: async () => {
      const response = await api.getApiClient().v1SpecTasksList({
        project_id: projectId || 'default',
      });
      return response.data || [];
    },
    enabled: !!projectId && viewMode === 'workspace',
    refetchInterval: 3700, // 3.7s - prime to avoid sync with other polling
  });

  // Get the default repository ID from the project
  const defaultRepoId = project?.default_repo_id;

  // Check if the default repo is an external repo (e.g., GitHub, Azure DevOps)
  const hasExternalRepo = useMemo(() => {
    const defaultRepo = projectRepositories.find(r => r.id === defaultRepoId);
    return !!(defaultRepo?.is_external || defaultRepo?.azure_devops || defaultRepo?.external_type);
  }, [projectRepositories, defaultRepoId]);

  const boardWipLimits = useMemo(() => {
    const limits = project?.metadata?.board_settings?.wip_limits;
    if (!limits) return undefined;
    return {
      planning: limits.planning ?? 3,
      review: limits.review ?? 2,
      implementation: limits.implementation ?? 5,
    };
  }, [project?.metadata?.board_settings?.wip_limits]);

  // Track newly created task ID for focusing "Start Planning" button
  const [focusTaskId, setFocusTaskId] = useState<string | undefined>(undefined);

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

  // Load tasks and apps on mount
  useEffect(() => {
    if (account.user?.id) {
      apps.loadApps(); // Load available agents
    }
  }, []);

  // Handle URL parameters for opening dialog
  useEffect(() => {
    if (router.params.new === 'true') {
      setCreateDialogOpen(true);
      // Clear URL parameter after handling
      router.removeParams(['new']);
    }
  }, [router.params.new]);

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

  const handleStartExploratorySession = async () => {
    try {
      const session = await startExploratorySessionMutation.mutateAsync();
      snackbar.success('Human Desktop started');
      // Navigate to the Human Desktop page
      account.orgNavigate('project-team-desktop', { id: projectId, sessionId: session.id });
    } catch (err: any) {
      // Extract error message from API response
      const errorMessage = err?.response?.data?.error || err?.message || 'Failed to start Human Desktop';
      snackbar.error(errorMessage);
    }
  };

  const handleResumeExploratorySession = async (e: React.MouseEvent) => {
    if (!exploratorySessionData) return;

    try {
      // Use the mutation hook which properly invalidates the cache
      const session = await resumeExploratorySessionMutation.mutateAsync();
      snackbar.success('Human Desktop resumed');
      // Navigate to the Human Desktop page
      account.orgNavigate('project-team-desktop', { id: projectId, sessionId: session.id });
    } catch (err) {
      snackbar.error('Failed to resume Human Desktop');
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

  const handleTaskCreated = useCallback((task: TypesSpecTask) => {
    setCreateDialogOpen(false);
    if (task.id) {
      setFocusTaskId(task.id);
      setTimeout(() => setFocusTaskId(undefined), 5000);
    }
    setRefreshTrigger(prev => prev + 1);
  }, []);

  // Invite dialog helpers
  const handleOpenInvite = useCallback(() => {
    router.mergeParams({ invite: 'true' });
  }, []);

  const handleCloseInvite = useCallback(() => {
    router.removeParams(['invite']);
  }, []);

  const handleCreateAccessGrant = async (request: any) => {
    try {
      const result = await createAccessGrantMutation.mutateAsync(request);
      if (result) {
        snackbar.success('Access grant created');
        return result;
      }
      return null;
    } catch (err) {
      snackbar.error('Failed to create access grant');
      return null;
    }
  };

  const handleDeleteAccessGrant = async (grantId: string) => {
    try {
      await deleteAccessGrantMutation.mutateAsync(grantId);
      snackbar.success('Access grant removed');
      return true;
    } catch (err) {
      snackbar.error('Failed to remove access grant');
      return false;
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
      showDrawerButton={true}
      disableContentScroll={true}
      topbarContent={
        <Stack direction="row" spacing={2} sx={{ justifyContent: 'flex-end', width: '100%', minWidth: 0, alignItems: 'center' }}>
          {/* Invite / Share button */}
          <Box sx={{ display: { xs: 'none', md: 'flex' }, alignItems: 'center' }}>
            <ProjectMembersBar
              currentUser={account.user}
              projectOwnerId={project?.user_id}
              projectId={projectId || ''}
              organizationId={project?.organization_id}
              accessGrants={accessGrants}
              inviteOpen={inviteOpen}
              onOpenInvite={handleOpenInvite}
              onCloseInvite={handleCloseInvite}
              onCreateGrant={handleCreateAccessGrant}
              onDeleteGrant={handleDeleteAccessGrant}
            />
          </Box>

          {/* View mode toggle: Board vs Workspace vs Audit Trail */}
          <Stack direction="row" spacing={0.5} sx={{ borderRadius: 1, p: 0.5 }}>
            <Tooltip
              title={
                <Box sx={{ p: 0.5 }}>
                  <Typography variant="subtitle2" sx={{ fontWeight: 'bold', mb: 0.5 }}>Board View</Typography>
                  <Typography variant="caption" component="div">
                    • Manage a fleet of AI agents working in parallel<br />
                    • Each task runs in its own isolated environment<br />
                    • Spec-driven workflow: planning → review → implement → PR<br />
                    • Watch agents work live on their desktops
                  </Typography>
                </Box>
              }
              arrow
              placement="bottom"
            >
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
            {/* Workspace toggle hidden on phones - multi-panel layout doesn't work on small screens */}
            <Box sx={{ display: { xs: 'none', md: 'flex' }, alignItems: 'center' }}>
              <Tooltip
                title={
                  <Box sx={{ p: 0.5 }}>
                    <Typography variant="subtitle2" sx={{ fontWeight: 'bold', mb: 0.5 }}>Split Screen</Typography>
                    <Typography variant="caption" component="div">
                      • Work on multiple tasks side-by-side<br />
                      • Drag dividers to resize panels<br />
                      • Open tasks and desktops in tabs<br />
                      • Drag tabs between panels
                    </Typography>
                  </Box>
                }
                arrow
                placement="bottom"
              >
                <IconButton
                  size="small"
                  onClick={() => setViewMode('workspace')}
                  sx={{
                    bgcolor: viewMode === 'workspace' ? 'background.paper' : 'transparent',
                    boxShadow: viewMode === 'workspace' ? 1 : 0,
                    '&:hover': { bgcolor: viewMode === 'workspace' ? 'background.paper' : 'action.selected' },
                  }}
                >
                  <TabIcon fontSize="small" color={viewMode === 'workspace' ? 'primary' : 'inherit'} />
                </IconButton>
              </Tooltip>
            </Box>
            <Tooltip
              title={
                <Box sx={{ p: 0.5 }}>
                  <Typography variant="subtitle2" sx={{ fontWeight: 'bold', mb: 0.5 }}>Audit Trail</Typography>
                  <Typography variant="caption" component="div">
                    • View all project activity<br />
                    • Track task status changes<br />
                    • See agent actions and decisions<br />
                    • Review approval history
                  </Typography>
                </Box>
              }
              arrow
              placement="bottom"
            >
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
         
          {/* Hide these buttons on mobile - they'll be in the floating menu */}
          <Box sx={{ display: { xs: 'none', md: 'flex' }, gap: 2, alignItems: 'center' }}>
            {!exploratorySessionData ? (
              <Tooltip title="Test your app and find tasks for your agents. Shared with your team.">
                <Button
                  variant="text"
                  color="secondary"
                  size="small"
                  startIcon={<ExploreIcon />}
                  onClick={handleStartExploratorySession}
                  disabled={startExploratorySessionMutation.isPending}
                  sx={{ flexShrink: 0, textTransform: 'none', fontWeight: 500 }}
                >
                  {startExploratorySessionMutation.isPending ? 'Starting...' : 'Open Human Desktop'}
                </Button>
              </Tooltip>
            ) : exploratorySessionData.config?.external_agent_status === 'stopped' ? (
              <Tooltip title="Test your app and find tasks for your agents. Shared with your team.">
                <Button
                  variant="text"
                  color="secondary"
                  size="small"
                  startIcon={<Play size={18} />}
                  onClick={handleResumeExploratorySession}
                  disabled={resumeExploratorySessionMutation.isPending}
                  sx={{ flexShrink: 0, textTransform: 'none', fontWeight: 500 }}
                >
                  {resumeExploratorySessionMutation.isPending ? 'Resuming...' : 'Resume Human Desktop'}
                </Button>
              </Tooltip>
            ) : (
              <>
                <Button
                  variant="text"
                  color="primary"
                  size="small"
                  startIcon={<Play size={18} />}
                  onClick={() => {
                    // Navigate to the Human Desktop page
                    account.orgNavigate('project-team-desktop', { id: projectId, sessionId: exploratorySessionData.id });
                  }}
                  sx={{ flexShrink: 0, textTransform: 'none', fontWeight: 500 }}
                >
                  View Human Desktop
                </Button>
                <Button
                  variant="text"
                  color="error"
                  size="small"
                  startIcon={<StopIcon />}
                  onClick={handleStopExploratorySession}
                  disabled={stopExploratorySessionMutation.isPending}
                  sx={{ flexShrink: 0, textTransform: 'none', fontWeight: 500 }}
                >
                  {stopExploratorySessionMutation.isPending ? 'Stopping...' : 'Stop Session'}
                </Button>
              </>
            )}
            <Tooltip title={projectManagerAppId ? "Chat with Project Manager agent" : "Configure Project Manager agent in project settings to enable chat"}>
              <span>
                <Button
                  variant="text"
                  size="small"
                  startIcon={<Plus size={18} />}
                  onClick={handleOpenChatPanel}
                  disabled={!projectManagerAppId}
                  sx={{ flexShrink: 0, textTransform: 'none', fontWeight: 500 }}
                >
                  New Chat
                </Button>
              </span>
            </Tooltip>
          </Box>
          {/* Hide menu button on mobile - it will be in the bottom nav */}
          <Box sx={{ display: { xs: 'none', md: 'block' } }}>
            <IconButton
              size="small"
              onClick={(e) => setViewMenuAnchorEl(e.currentTarget)}
            >
              <MoreHorizontal size={18} />
            </IconButton>
          </Box>
          <Menu
            anchorEl={viewMenuAnchorEl}
            open={Boolean(viewMenuAnchorEl)}
            onClose={() => setViewMenuAnchorEl(null)}
            anchorOrigin={{ vertical: 'bottom', horizontal: 'right' }}
            transformOrigin={{ vertical: 'top', horizontal: 'right' }}
            slotProps={{
              paper: {
                sx: {
                  minWidth: 200,
                  boxShadow: '0 4px 20px rgba(0,0,0,0.15)',
                },
              },
            }}
          >
            {defaultRepoId && (
              <MenuItem onClick={() => { account.orgNavigate('git-repo-detail', { repoId: defaultRepoId }); setViewMenuAnchorEl(null); }}>
                <FolderOpen style={{ marginRight: 12, width: 20, height: 20 }} />
                Files
              </MenuItem>
            )}
            {projectId && (
              <MenuItem onClick={() => { account.orgNavigate('project-settings', { id: projectId }); setViewMenuAnchorEl(null); }}>
                <Settings style={{ marginRight: 12, width: 20, height: 20 }} />
                Settings
              </MenuItem>
            )}
            <MenuItem onClick={() => { handleOpenInvite(); setViewMenuAnchorEl(null); }}>
              <UserPlus style={{ marginRight: 12, width: 20, height: 20 }} />
              Sharing
            </MenuItem>
            <MenuItem onClick={() => { setShowArchived(!showArchived); setViewMenuAnchorEl(null); }}>
              {showArchived ? <ViewIcon sx={{ mr: 1.5, fontSize: 20 }} /> : <ArchiveIcon sx={{ mr: 1.5, fontSize: 20 }} />}
              {showArchived ? 'Show Active Tasks' : 'Show Archived Tasks'}
            </MenuItem>
            <MenuItem onClick={() => { handleToggleMetrics(); setViewMenuAnchorEl(null); }}>
              <MetricsIcon sx={{ mr: 1.5, fontSize: 20 }} />
              {showMetrics ? 'Hide Metrics' : 'Show Metrics'}
            </MenuItem>
            <MenuItem onClick={() => { handleToggleMerged(); setViewMenuAnchorEl(null); }}>
              <GitMerge style={{ marginRight: 12, width: 20, height: 20 }} />
              {showMerged ? 'Hide Merged' : 'Show Merged'}
            </MenuItem>
          </Menu>
        </Stack>
      }
    >
      <Box sx={{
        display: 'flex',
        flexDirection: 'row',
        width: '100%',
        flex: 1,
        minHeight: 0,
        overflow: 'hidden',
        position: 'relative',
      }}>

        {/* MAIN CONTENT */}
        <Box sx={{
          flex: 1,
          display: 'flex',
          flexDirection: 'column',
          minWidth: 0,
          minHeight: 0,
          overflow: 'hidden',
          transition: 'all 0.3s ease-in-out',
          px: { xs: 0, md: 3 },
          pl: { xs: 2, md: 3 },
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
              No repositories attached. Go to Settings to connect a repository.
            </Alert>
          )}

          {/* No startup script warning - show when repo is connected but startup script hasn't been modified */}
          {projectRepositories.length > 0 && isStartupScriptHistoryLoaded && startupScriptNotConfigured && (
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
                  Configure Startup Script
                </Button>
              }
            >
              Set up a startup script to install dependencies and start your dev server.
            </Alert>
          )}

          {/* Main Content: Kanban Board, Tabs View, or Audit Trail */}
          <Box sx={{ flex: 1, minHeight: 0, minWidth: 0, display: 'flex', flexDirection: 'column', overflowX: 'hidden' }}>
            {viewMode === 'kanban' && (
              <SpecTaskKanbanBoard
                userId={account.user?.id}
                projectId={projectId}
                wipLimits={boardWipLimits}
                onCreateTask={handleOpenCreateDialog}
                onTaskClick={(task) => {
                  // Navigate to task detail page
                  account.orgNavigate('project-task-detail', { id: projectId, taskId: task.id });
                }}
                onRefresh={() => {
                  setRefreshing(true);
                  setTimeout(() => setRefreshing(false), 2000);
                }}
                refreshing={refreshing}
                refreshTrigger={refreshTrigger}
                focusTaskId={focusTaskId}
                hasExternalRepo={hasExternalRepo}
                showArchived={showArchived}
                showMetrics={showMetrics}
                showMerged={showMerged}
              />
            )}
            {viewMode === 'workspace' && (
              <TabsView
                projectId={projectId}
                tasks={tasksData || []}
                onCreateTask={handleOpenCreateDialog}
                onRefresh={() => setRefreshTrigger(prev => prev + 1)}
                initialTaskId={openTaskId}
                initialDesktopId={openDesktopId}
                initialReviewId={openReviewId}
                exploratorySessionId={exploratorySessionData?.id}
              />
            )}
            {viewMode === 'audit' && (
              <ProjectAuditTrail
                projectId={projectId || ''}
                onTaskClick={(taskId) => {
                  // Navigate to task detail page
                  account.orgNavigate('project-task-detail', { id: projectId, taskId });
                }}
              />
            )}
          </Box>
        </Box>

        {/* RIGHT PANEL: New Spec Task - slides in from right, full screen on mobile */}
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
            // Full screen overlay on mobile
            position: { xs: 'fixed', md: 'relative' },
            top: { xs: 0, md: 'auto' },
            left: { xs: 0, md: 'auto' },
            right: { xs: 0, md: 'auto' },
            bottom: { xs: 0, md: 'auto' },
            zIndex: { xs: 1200, md: 'auto' },
          }}
        >
          {createDialogOpen && projectId && (
            <NewSpecTaskForm
              projectId={projectId}
              onTaskCreated={handleTaskCreated}
              onClose={() => setCreateDialogOpen(false)}
              showHeader={true}
              embedded={false}
            />
          )}
        </Box>

        {/* RIGHT PANEL: Chat with Project Manager Agent - full screen on mobile */}
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
              // Full screen overlay on mobile
              position: { xs: 'fixed', md: 'relative' },
              top: { xs: 0, md: 'auto' },
              left: { xs: 0, md: 'auto' },
              right: { xs: 0, md: 'auto' },
              bottom: { xs: 0, md: 'auto' },
              zIndex: { xs: 1200, md: 'auto' },
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

      {/* Mobile Bottom Navigation Bar */}
      {isMobile && (
        <SpecTasksMobileBottomNav
          onNewTask={handleOpenCreateDialog}
          onNewChat={handleOpenChatPanel}
          chatDisabled={!projectManagerAppId}
          onMenuClick={(e) => setViewMenuAnchorEl(e.currentTarget)}
        />
      )}

    </Page>
  );
};

export default SpecTasksPage;
