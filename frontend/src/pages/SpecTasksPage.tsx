import React, { FC, useState, useEffect } from 'react';
import {
  Box,
  Button,
  Typography,
  Alert,
  Chip,
  Stack,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  TextField,
  FormControl,
  InputLabel,
  Select,
  MenuItem,
  ListSubheader,
  CircularProgress,
  Tabs,
  Tab,
} from '@mui/material';
import {
  Add as AddIcon,
  Refresh as RefreshIcon,
  Settings as SettingsIcon,
} from '@mui/icons-material';

import Page from '../components/system/Page';
import SpecTaskKanbanBoard from '../components/tasks/SpecTaskKanbanBoard';
import LoadingSpinner from '../components/widgets/LoadingSpinner';

import useAccount from '../hooks/useAccount';
import useApi from '../hooks/useApi';
import useSnackbar from '../hooks/useSnackbar';
import useRouter from '../hooks/useRouter';
import useApps from '../hooks/useApps';
import { useSampleTypes } from '../hooks/useSampleTypes';
import { useSpecTasks } from '../hooks/useSpecTasks';
import { useGitRepositories, useCreateGitRepository, useCreateSampleRepository } from '../services/gitRepositoryService';
import { TypesSpecTask, ServicesCreateTaskRequest } from '../api/api';
import {
  SampleType,
  getSampleTypeIcon,
  getSampleTypeCategory,
  isBusinessTask,
  getBusinessTaskDescription,
  groupSampleTypesByCategory,
  getCategoryDisplayName
} from '../utils/sampleTypeUtils';

const SpecTasksPage: FC = () => {
  const account = useAccount();
  const api = useApi();
  const snackbar = useSnackbar();
  const router = useRouter();
  const apps = useApps();

  // Get project ID from URL if in project context
  const projectId = router.params.id as string | undefined;

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

  // Settings dialog state
  const [settingsDialogOpen, setSettingsDialogOpen] = useState(false);
  const [settingsLoading, setSettingsLoading] = useState(false);
  const [wipLimits, setWipLimits] = useState({
    planning: 3,
    review: 2,
    implementation: 5,
  });

  // Create task form state (SIMPLIFIED)
  const [taskPrompt, setTaskPrompt] = useState(''); // Single text box for everything
  const [taskPriority, setTaskPriority] = useState('medium');
  const [selectedHelixAgent, setSelectedHelixAgent] = useState('');

  // Repository attachment state
  const [repoTabValue, setRepoTabValue] = useState(0); // 0 = existing, 1 = new
  const [selectedExistingRepo, setSelectedExistingRepo] = useState('');
  const [newRepoType, setNewRepoType] = useState('empty'); // Default to 'empty', or sample type ID
  const [newRepoName, setNewRepoName] = useState(''); // Name for new repository

  // Data hooks
  const { data: sampleTypes, loading: sampleTypesLoading, loadSampleTypes } = useSampleTypes();
  const createSampleRepoMutation = useCreateSampleRepository();
  const { data: tasks, loading: tasksLoading, listTasks } = useSpecTasks();

  // Get current org/user ID for fetching repositories
  const currentOrg = account.organizationTools.organization;
  const ownerId = currentOrg?.id || account.user?.id || '';

  // Fetch existing repositories
  const { data: existingRepositories = [], isLoading: repositoriesLoading } = useGitRepositories(ownerId);
  const createGitRepoMutation = useCreateGitRepository();

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

  // Auto-select default agent when dialog opens
  useEffect(() => {
    if (createDialogOpen) {
      // Check if "Default Spec Agent" already exists
      const defaultAgent = apps.apps.find(app =>
        (app.config?.helix?.name || app.name) === 'Default Spec Agent'
      );

      if (defaultAgent) {
        // Select the existing default agent
        setSelectedHelixAgent(defaultAgent.id || '');
      } else if (apps.apps.length === 0) {
        // No agents exist, default to create option
        setSelectedHelixAgent('__create_default__');
      } else {
        // Agents exist but no default agent, select first one
        setSelectedHelixAgent(apps.apps[0]?.id || '__create_default__');
      }
    }
  }, [createDialogOpen, apps.apps]);

  // Load board settings on mount
  useEffect(() => {
    const loadSettings = async () => {
      try {
        const response = await api.get('/api/v1/spec-tasks/board-settings');
        if (response.data && response.data.wip_limits) {
          setWipLimits({
            planning: response.data.wip_limits.planning || 3,
            review: response.data.wip_limits.review || 2,
            implementation: response.data.wip_limits.implementation || 5,
          });
        }
      } catch (error) {
        console.error('Failed to load board settings:', error);
        // Use default values if loading fails
      }
    };

    if (account.user?.id) {
      loadSettings();
    }
  }, [account.user?.id]);

  // Handle URL parameters for pre-selecting repo and opening dialog
  useEffect(() => {
    if (router.params.new === 'true') {
      setCreateDialogOpen(true);

      // Pre-select repository if repo_id is provided
      if (router.params.repo_id) {
        setSelectedExistingRepo(router.params.repo_id);
        setRepoTabValue(0); // Switch to existing repo tab
      }

      // Clear URL parameters after handling them
      router.removeParams(['new', 'repo_id']);
    }
  }, [router.params.new, router.params.repo_id]);

  // Check authentication
  const checkLoginStatus = (): boolean => {
    if (!account.user) {
      account.setShowLoginWindow(true);
      return false;
    }
    return true;
  };

  // Handle settings dialog
  const handleOpenSettings = () => {
    setSettingsDialogOpen(true);
  };

  const handleCloseSettings = () => {
    setSettingsDialogOpen(false);
  };

  const handleSaveSettings = async () => {
    try {
      setSettingsLoading(true);
      await api.put('/api/v1/spec-tasks/board-settings', {
        wip_limits: wipLimits,
      });
      snackbar.success('Board settings saved successfully');
      setSettingsDialogOpen(false);

      // Refresh the kanban board to apply new limits
      setRefreshing(true);
      setTimeout(() => setRefreshing(false), 1000);
    } catch (error) {
      console.error('Failed to save board settings:', error);
      snackbar.error('Failed to save settings. Please try again.');
    } finally {
      setSettingsLoading(false);
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

      // Handle repository attachment
      let repositoryId: string | undefined;

      if (repoTabValue === 0) {
        // Use existing repository
        if (selectedExistingRepo) {
          repositoryId = selectedExistingRepo;
        }
      } else if (repoTabValue === 1) {
        // Create new repository
        if (!newRepoName.trim()) {
          snackbar.error('Please enter a repository name');
          return;
        }

        // Check if repository name already exists
        const nameExists = existingRepositories.some((repo: any) => repo.name === newRepoName.trim());
        if (nameExists) {
          snackbar.error(`A repository named "${newRepoName.trim()}" already exists. Please choose a different name.`);
          return;
        }

        if (newRepoType === 'empty') {
          // Create empty repository
          try {
            snackbar.info('Creating empty repository...');

            const newRepo = await createGitRepoMutation.mutateAsync({
              name: newRepoName.trim(),
              description: taskPrompt,
              owner_id: ownerId,
              repo_type: 'project',
              default_branch: 'main',
              metadata: {
                kodit_indexing: true,
              },
            });

            // Check if repository was created successfully
            if (!newRepo || !newRepo.id) {
              snackbar.error('Failed to create empty repository. Please try again.');
              return;
            }

            repositoryId = newRepo.id;
          } catch (err: any) {
            console.error('Failed to create empty repository:', err);
            const errorMessage = err?.response?.data?.message
              || err?.message
              || 'Failed to create empty repository. Please try again.';
            snackbar.error(errorMessage);
            return;
          }
        } else if (newRepoType) {
          // Create sample repository
          if (!newRepoName.trim()) {
            snackbar.error('Please enter a repository name');
            return;
          }

          try {
            snackbar.info('Creating demo repository...');

            const newRepo = await createSampleRepoMutation.mutateAsync({
              owner_id: ownerId,
              sample_type: newRepoType,
              name: newRepoName.trim(),
              kodit_indexing: true,
            });

            // Check if repository was created successfully
            if (!newRepo || !newRepo.id) {
              snackbar.error('Failed to create sample repository. Please try again.');
              return;
            }

            repositoryId = newRepo.id;
          } catch (err: any) {
            console.error('Failed to create sample repository:', err);
            const errorMessage = err?.response?.data?.message
              || err?.message
              || 'Failed to create sample repository. Please try again.';
            snackbar.error(errorMessage);
            return;
          }
        }
      }

      // Create SpecTask with simplified single-field approach
      const createTaskRequest: ServicesCreateTaskRequest = {
        prompt: taskPrompt, // Just the raw user input!
        priority: taskPriority,
        project_id: projectId || 'default', // Use project ID from route, or 'default'
        app_id: agentId || undefined, // Include selected or created agent if provided
        git_repository_id: repositoryId, // Attach repository if selected/created
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
        setRepoTabValue(0);
        setSelectedExistingRepo('');
        setNewRepoType('empty');
        setNewRepoName('');

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

  const getSampleTypesByCategory = () => {
    if (!sampleTypes || sampleTypes.length === 0) return { empty: [], development: [], business: [], content: [] };

    const categorized = {
      empty: [] as SampleType[],
      development: [] as SampleType[],
      business: [] as SampleType[],
      content: [] as SampleType[],
    };

    sampleTypes.forEach((type: SampleType) => {
      const category = getSampleTypeCategory(type.id || '');
      categorized[category].push(type);
    });

    return categorized;
  };

  const categorizedSampleTypes = getSampleTypesByCategory();

  return (
    <Page
      breadcrumbTitle="SpecTasks"
      orgBreadcrumbs={true}
      showDrawerButton={false}
      topbarContent={
        <Stack direction="row" spacing={2} sx={{ justifyContent: 'flex-end', width: '100%', minWidth: 0 }}>
          <Button
            variant="outlined"
            startIcon={<SettingsIcon />}
            onClick={handleOpenSettings}
            sx={{ flexShrink: 0 }}
          >
            Settings
          </Button>
          <Button
            variant="outlined"
            startIcon={refreshing ? <CircularProgress size={16} /> : <RefreshIcon />}
            onClick={() => {
              setRefreshing(true);
              setTimeout(() => setRefreshing(false), 2000);
            }}
            disabled={refreshing}
            sx={{ flexShrink: 0 }}
          >
            Refresh
          </Button>
        </Stack>
      }
    >
      <Box sx={{ width: '100%', maxWidth: '100%', height: 'calc(100vh - 120px)', display: 'flex', flexDirection: 'column', overflowX: 'hidden', overflowY: 'hidden', px: 3, boxSizing: 'border-box' }}>
        {/* Introduction */}
        <Box sx={{ flexShrink: 0, mb: 2, minWidth: 0 }}>
          <Typography variant="h4" sx={{ fontWeight: 600, mb: 1 }}>
            Spec Work for Agents
          </Typography>
          <Typography variant="body1" color="text.secondary">
            Add tasks for your agents to do, verify their informed plans, then supervise them executing them. Jump in when they need help or guidance
          </Typography>
        </Box>

        {/* Kanban Board */}
        <Box sx={{ flex: 1, minHeight: 0, minWidth: 0, display: 'flex', flexDirection: 'column', overflowX: 'hidden' }}>
          <SpecTaskKanbanBoard
            userId={account.user?.id}
            projectId={projectId}
            onCreateTask={() => setCreateDialogOpen(true)}
            refreshTrigger={refreshTrigger}
            wipLimits={wipLimits}
            repositories={existingRepositories}
          />
        </Box>
      </Box>

      {/* Create SpecTask Dialog - SIMPLIFIED */}
      <Dialog open={createDialogOpen} onClose={() => setCreateDialogOpen(false)} maxWidth="md" fullWidth>
        <DialogTitle>
          <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
              <AddIcon />
              New SpecTask
            </Box>
            <FormControl size="small" sx={{ minWidth: 100 }}>
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
          </Box>
        </DialogTitle>
        <DialogContent>
          <Stack spacing={3} sx={{ mt: 1 }}>
            {/* Single large text box for everything */}
            <TextField
              label="Describe what you want to get done"
              fullWidth
              required
              multiline
              rows={12}
              value={taskPrompt}
              onChange={(e) => setTaskPrompt(e.target.value)}
              placeholder="Dump everything you know here - the AI will parse this into requirements, design, and implementation plan.

Examples:
- Add dark mode toggle to settings page
- Fix the user registration bug where emails aren't validated properly
- Refactor the payment processing to use Stripe instead of PayPal"
              helperText="The planning agent will extract task name, description, type, and generate full specifications from this"
              autoFocus
            />

            {/* Repository Selection - Tabs layout */}
            <Box sx={{ border: 1, borderColor: 'divider', borderRadius: 1, p: 2 }}>
              <Typography variant="subtitle2" sx={{ mb: 2, fontWeight: 600 }}>
                Repository
              </Typography>

              <Tabs
                value={repoTabValue}
                onChange={(e, newValue) => {
                  setRepoTabValue(newValue);
                  // Reset selections when switching
                  setSelectedExistingRepo('');
                  setNewRepoType('empty');
                  setNewRepoName('');
                }}
                sx={{ borderBottom: 1, borderColor: 'divider', mb: 2 }}
              >
                <Tab label="Use Existing Repository" />
                <Tab label="Create New (Empty or Demo)" />
              </Tabs>

              {/* Tab Panel 0: Existing Repository */}
              {repoTabValue === 0 && (
                <Box sx={{ pt: 1 }}>
                  <FormControl fullWidth>
                    <InputLabel>Select Repository</InputLabel>
                    <Select
                      value={selectedExistingRepo}
                      onChange={(e) => setSelectedExistingRepo(e.target.value)}
                      label="Select Repository"
                      disabled={repositoriesLoading}
                    >
                      {existingRepositories.map((repo: any) => (
                        <MenuItem key={repo.id} value={repo.id}>
                          {repo.name || repo.id}
                        </MenuItem>
                      ))}
                    </Select>
                  </FormControl>

                  {!repositoriesLoading && existingRepositories.length === 0 && (
                    <Typography variant="caption" color="text.secondary" sx={{ mt: 1, display: 'block' }}>
                      No existing repositories found. Create a new one in the other tab.
                    </Typography>
                  )}
                </Box>
              )}

              {/* Tab Panel 1: Create New Repository */}
              {repoTabValue === 1 && (
                <Box sx={{ pt: 1 }}>
                  <Stack spacing={2}>
                    <FormControl fullWidth>
                      <InputLabel>Repository Type</InputLabel>
                      <Select
                        value={newRepoType}
                        onChange={(e) => {
                          const selectedType = e.target.value;
                          setNewRepoType(selectedType);

                          // Auto-fill name for demo repositories
                          if (selectedType !== 'empty') {
                            const sampleType = sampleTypes.find((t: SampleType) => t.id === selectedType);
                            if (sampleType?.name) {
                              const autoName = sampleType.name
                                .toLowerCase()
                                .replace(/[^a-z0-9\s-]/g, '')
                                .replace(/\s+/g, '-')
                                .replace(/-+/g, '-')
                                .replace(/^-|-$/g, ''); // Remove leading/trailing hyphens
                              setNewRepoName(autoName);
                            }
                          } else {
                            // Clear name for empty repos so user must enter it
                            setNewRepoName('');
                          }
                        }}
                        label="Repository Type"
                        disabled={sampleTypesLoading}
                      >
                        <MenuItem value="empty">
                          📄 New Empty Repository
                        </MenuItem>
                        <ListSubheader>Demo Repositories</ListSubheader>
                        {sampleTypes && sampleTypes.map((type: SampleType) => (
                          <MenuItem key={type.id} value={type.id}>
                            {getSampleTypeIcon(type.id || '')} {type.name}
                          </MenuItem>
                        ))}
                      </Select>
                    </FormControl>

                    <TextField
                      label="Repository Name"
                      fullWidth
                      required
                      value={newRepoName}
                      onChange={(e) => setNewRepoName(e.target.value)}
                      placeholder="my-project-repo"
                      helperText={newRepoType === 'empty'
                        ? "Enter a name for the new repository"
                        : "Auto-filled from demo type (you can edit if needed)"}
                    />

                    <Typography variant="caption" color="text.secondary">
                      The repository will be created when you create the SpecTask.
                    </Typography>
                  </Stack>
                </Box>
              )}
            </Box>

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
                    {app.config?.helix?.name || app.name || 'Unnamed Agent'}
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
          </Stack>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => {
            setCreateDialogOpen(false);
            setTaskPrompt('');
            setTaskPriority('medium');
            setSelectedHelixAgent('');
            setRepoTabValue(0);
            setSelectedExistingRepo('');
            setNewRepoType('empty');
            setNewRepoName('');
          }}>
            Cancel
          </Button>
          <Button
            onClick={handleCreateTask}
            variant="contained"
            disabled={!taskPrompt.trim() || createSampleRepoMutation.isPending || createGitRepoMutation.isPending}
            startIcon={createSampleRepoMutation.isPending || createGitRepoMutation.isPending ? <CircularProgress size={16} /> : <AddIcon />}
          >
            {createSampleRepoMutation.isPending || createGitRepoMutation.isPending ? 'Creating...' : 'Create Task'}
          </Button>
        </DialogActions>
      </Dialog>

      {/* Board Settings Dialog */}
      <Dialog open={settingsDialogOpen} onClose={handleCloseSettings} maxWidth="sm" fullWidth>
        <DialogTitle>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
            <SettingsIcon />
            Board Settings
          </Box>
        </DialogTitle>
        <DialogContent>
          <Stack spacing={3} sx={{ mt: 2 }}>
            <Typography variant="body1" color="text.secondary">
              Configure work-in-progress (WIP) limits for each column in the Kanban board.
              These limits help maintain flow and prevent overloading.
            </Typography>

            <TextField
              label="Planning Limit"
              fullWidth
              value={wipLimits.planning}
              onChange={(e) => setWipLimits({ ...wipLimits, planning: parseInt(e.target.value) || 0 })}
              helperText="Maximum number of tasks allowed in the Planning column"
            />

            <TextField
              label="Review Limit"
              fullWidth
              value={wipLimits.review}
              onChange={(e) => setWipLimits({ ...wipLimits, review: parseInt(e.target.value) || 0 })}
              helperText="Maximum number of tasks allowed in the Review column"
            />

            <TextField
              label="Implementation Limit"
              fullWidth
              value={wipLimits.implementation}
              onChange={(e) => setWipLimits({ ...wipLimits, implementation: parseInt(e.target.value) || 0 })}
              helperText="Maximum number of tasks allowed in the Implementation column"
            />

            <Alert severity="info">
              <Typography variant="body2">
                <strong>Note:</strong> Backlog and Completed columns do not have WIP limits.
              </Typography>
            </Alert>
          </Stack>
        </DialogContent>
        <DialogActions>
          <Button onClick={handleCloseSettings} disabled={settingsLoading}>
            Cancel
          </Button>
          <Button
            onClick={handleSaveSettings}
            variant="contained"
            disabled={settingsLoading}
            startIcon={settingsLoading ? <CircularProgress size={16} /> : undefined}
          >
            {settingsLoading ? 'Saving...' : 'Save Settings'}
          </Button>
        </DialogActions>
      </Dialog>
    </Page>
  );
};

export default SpecTasksPage;
