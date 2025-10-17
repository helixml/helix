import React, { FC, useState, useEffect } from 'react';
import {
  Box,
  Button,
  Typography,
  Tabs,
  Tab,
  Card,
  CardContent,
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
} from '@mui/material';
import {
  Add as AddIcon,
  ViewKanban as KanbanIcon,
  TableView as TableIcon,
  Dashboard as DashboardIcon,
  Refresh as RefreshIcon,
} from '@mui/icons-material';

import Page from '../components/system/Page';
import SpecTaskKanbanBoard from '../components/tasks/SpecTaskKanbanBoard';
import SpecTaskTable from '../components/tasks/SpecTaskTable';
import MultiSessionDashboard from '../components/tasks/MultiSessionDashboard';
import LoadingSpinner from '../components/widgets/LoadingSpinner';

import useAccount from '../hooks/useAccount';
import useApi from '../hooks/useApi';
import useSnackbar from '../hooks/useSnackbar';
import useRouter from '../hooks/useRouter';
import { useSampleTypes, useCreateSampleRepository } from '../hooks/useSampleTypes';
import { useSpecTasks, useSpecTask } from '../hooks/useSpecTasks';
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

interface TabPanelProps {
  children?: React.ReactNode;
  index: number;
  value: number;
}

function TabPanel(props: TabPanelProps) {
  const { children, value, index, ...other } = props;

  return (
    <div
      role="tabpanel"
      hidden={value !== index}
      id={`spec-tasks-tabpanel-${index}`}
      aria-labelledby={`spec-tasks-tab-${index}`}
      style={{ display: value === index ? 'flex' : 'none', flexDirection: 'column', flex: 1, minHeight: 0, overflowX: 'hidden' }}
      {...other}
    >
      {value === index && children}
    </div>
  );
}

const SpecTasksPage: FC = () => {
  const account = useAccount();
  const api = useApi();
  const snackbar = useSnackbar();
  const router = useRouter();

  // State for view management
  const [currentTab, setCurrentTab] = useState(0);
  const [selectedTaskId, setSelectedTaskId] = useState<string | null>(null);
  const [createDialogOpen, setCreateDialogOpen] = useState(false);
  const [refreshing, setRefreshing] = useState(false);

  // Create task form state
  const [newTaskName, setNewTaskName] = useState('');
  const [newTaskDescription, setNewTaskDescription] = useState('');
  const [newTaskRequirements, setNewTaskRequirements] = useState('');
  const [selectedSampleType, setSelectedSampleType] = useState('');
  const [taskType, setTaskType] = useState('feature');
  const [taskPriority, setTaskPriority] = useState('medium');

  // Data hooks
  const { data: sampleTypes, loading: sampleTypesLoading, loadSampleTypes } = useSampleTypes();
  const { create: createSampleRepo, loading: createSampleRepoLoading } = useCreateSampleRepository();
  const { data: tasks, loading: tasksLoading, listTasks } = useSpecTasks();
  const { data: selectedTask } = useSpecTask(selectedTaskId || '');

  // Load tasks on mount (sample types auto-load via hook)
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
    }
  }, []);

  // Check authentication
  const checkLoginStatus = (): boolean => {
    if (!account.user) {
      account.setShowLoginWindow(true);
      return false;
    }
    return true;
  };

  // Handle task creation
  const handleCreateTask = async () => {
    if (!checkLoginStatus()) return;

    try {
      if (!newTaskName.trim() || !newTaskDescription.trim() || !selectedSampleType) {
        snackbar.error('Please fill in all required fields');
        return;
      }

      // First create the sample repository
      const repository = await createSampleRepo({
        name: `${newTaskName} - ${selectedSampleType}`,
        description: newTaskDescription,
        owner_id: account.user?.id || '',
        sample_type: selectedSampleType,
      });

      if (!repository) {
        snackbar.error('Failed to create repository. Please try again.');
        return;
      }

      // Then create the SpecTask
      const createTaskRequest: ServicesCreateTaskRequest = {
        prompt: `${newTaskDescription}\n\nRequirements:\n${newTaskRequirements}`,
        type: taskType,
        priority: taskPriority,
        project_id: account.organizationTools.organization?.id || 'default',
      };

      console.log('🔥 CREATING TASK WITH REQUEST:', createTaskRequest);
      console.log('🔥 USER ID:', account.user?.id);

      const response = await api.getApiClient().v1SpecTasksFromPromptCreate(createTaskRequest);

      if (response.data) {
        console.log('🔥 TASK CREATED SUCCESSFULLY:', response.data);
        snackbar.success('SpecTask created successfully!');
        setCreateDialogOpen(false);
        resetCreateForm();

        // Refresh the task list and kanban board
        listTasks();
        setRefreshing(true);
        setTimeout(() => setRefreshing(false), 1000);
      }
    } catch (error) {
      console.error('Failed to create SpecTask:', error);
      snackbar.error('Failed to create SpecTask. Please try again.');
    }
  };

  const resetCreateForm = () => {
    setNewTaskName('');
    setNewTaskDescription('');
    setNewTaskRequirements('');
    setSelectedSampleType('');
    setTaskType('feature');
    setTaskPriority('medium');
  };

  const handleTaskClick = (task: any) => {
    setSelectedTaskId(task.id);
    if (currentTab !== 2) {
      setCurrentTab(2); // Switch to dashboard view
    }
  };

  const handleTabChange = (event: React.SyntheticEvent, newValue: number) => {
    setCurrentTab(newValue);
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
      topbarContent={
        <Stack direction="row" spacing={2} sx={{ justifyContent: 'flex-end', width: '100%', minWidth: 0 }}>
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
          <Button
            id="new-spec-task-button"
            variant="contained"
            color="primary"
            startIcon={<AddIcon />}
            onClick={() => {
              if (checkLoginStatus()) {
                setCreateDialogOpen(true);
              }
            }}
            sx={{ flexShrink: 0 }}
          >
            New SpecTask
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

        {/* Tabs for different views */}
        <Box sx={{ flexShrink: 0, borderBottom: 1, borderColor: 'divider', mb: 2, minWidth: 0 }}>
          <Tabs value={currentTab} onChange={handleTabChange} aria-label="spec-tasks-tabs" variant="scrollable" scrollButtons="auto">
            <Tab
              icon={<KanbanIcon />}
              label="Kanban Board"
              id="spec-tasks-tab-0"
              aria-controls="spec-tasks-tabpanel-0"
            />
            <Tab
              icon={<TableIcon />}
              label="Table View"
              id="spec-tasks-tab-1"
              aria-controls="spec-tasks-tabpanel-1"
            />
            <Tab
              icon={<DashboardIcon />}
              label="Session Dashboard"
              id="spec-tasks-tab-2"
              aria-controls="spec-tasks-tabpanel-2"
              disabled={!selectedTaskId}
            />
          </Tabs>
        </Box>

        {/* Tab content */}
        <Box sx={{ flex: 1, minHeight: 0, minWidth: 0, display: 'flex', flexDirection: 'column', overflowX: 'hidden' }}>
          <TabPanel value={currentTab} index={0}>
            <SpecTaskKanbanBoard
              userId={account.user?.id}
              onTaskClick={handleTaskClick}
            />
          </TabPanel>

          <TabPanel value={currentTab} index={1}>
            <SpecTaskTable
              tasks={tasks || []}
              loading={tasksLoading}
              onTaskSelect={handleTaskClick}
              onRefresh={() => {
                setRefreshing(true);
                listTasks().finally(() => setRefreshing(false));
              }}
            />
          </TabPanel>

          <TabPanel value={currentTab} index={2}>
            {selectedTaskId ? (
              <MultiSessionDashboard taskId={selectedTaskId} />
            ) : (
              <Card>
                <CardContent sx={{ textAlign: 'center', py: 4 }}>
                  <Typography variant="h6" color="text.secondary" gutterBottom>
                    No SpecTask Selected
                  </Typography>
                  <Typography variant="body2" color="text.secondary">
                    Select a SpecTask from the Kanban board or table view to see its multi-session dashboard
                  </Typography>
                </CardContent>
              </Card>
            )}
          </TabPanel>
        </Box>
      </Box>

      {/* Create SpecTask Dialog */}
      <Dialog open={createDialogOpen} onClose={() => setCreateDialogOpen(false)} maxWidth="md" fullWidth>
        <DialogTitle>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
            <AddIcon />
            Create New SpecTask
          </Box>
        </DialogTitle>
        <DialogContent>
          <Stack spacing={3} sx={{ mt: 1 }}>
            {/* Basic Information */}
            <Box>
              <Typography variant="subtitle1" sx={{ mb: 2, fontWeight: 600 }}>
                Basic Information
              </Typography>
              <Stack spacing={2}>
                <TextField
                  label="Task Name"
                  fullWidth
                  required
                  value={newTaskName}
                  onChange={(e) => setNewTaskName(e.target.value)}
                  placeholder="e.g., Add user authentication, LinkedIn outreach campaign"
                />
                <TextField
                  label="Description"
                  fullWidth
                  required
                  multiline
                  rows={3}
                  value={newTaskDescription}
                  onChange={(e) => setNewTaskDescription(e.target.value)}
                  placeholder="Describe what this task should accomplish..."
                />
                <TextField
                  label="Detailed Requirements"
                  fullWidth
                  multiline
                  rows={4}
                  value={newTaskRequirements}
                  onChange={(e) => setNewTaskRequirements(e.target.value)}
                  placeholder="Provide detailed requirements for the planning agent..."
                  helperText="This will be used by the Zed planning agent to generate specifications"
                />
              </Stack>
            </Box>

            {/* Task Configuration */}
            <Box>
              <Typography variant="subtitle1" sx={{ mb: 2, fontWeight: 600 }}>
                Task Configuration
              </Typography>
              <Stack direction="row" spacing={2}>
                <FormControl sx={{ minWidth: 120 }}>
                  <InputLabel>Type</InputLabel>
                  <Select
                    value={taskType}
                    onChange={(e) => setTaskType(e.target.value)}
                    label="Type"
                  >
                    <MenuItem value="feature">Feature</MenuItem>
                    <MenuItem value="bug">Bug Fix</MenuItem>
                    <MenuItem value="refactor">Refactor</MenuItem>
                    <MenuItem value="business">Business</MenuItem>
                    <MenuItem value="content">Content</MenuItem>
                  </Select>
                </FormControl>

                <FormControl sx={{ minWidth: 120 }}>
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
              </Stack>
            </Box>

            {/* Project Template Selection */}
            <Box>
              <Typography variant="subtitle1" sx={{ mb: 2, fontWeight: 600 }}>
                Project Template
              </Typography>
              {sampleTypesLoading ? (
                <Box sx={{ display: 'flex', justifyContent: 'center', py: 2 }}>
                  <CircularProgress size={24} />
                </Box>
              ) : (
                <FormControl fullWidth required>
                  <InputLabel>Project Template</InputLabel>
                  <Select
                    value={selectedSampleType}
                    onChange={(e) => {
                      console.log('🔥 Template selected:', e.target.value);
                      setSelectedSampleType(e.target.value);
                    }}
                    label="Project Template"
                  >
                    <MenuItem value="">
                      <em>- Select - </em>
                    </MenuItem>
                    {sampleTypes.map((type: SampleType) => (
                      <MenuItem key={type.id} value={type.id}>
                        {getSampleTypeIcon(type.id || '')} {type.name}
                      </MenuItem>
                    ))}
                  </Select>
                </FormControl>
              )}

              {selectedSampleType && (
                <Alert severity="info" sx={{ mt: 2 }}>
                  <Typography variant="body2">
                    <strong>Selected Template:</strong> {sampleTypes?.find((t: any) => t.id === selectedSampleType)?.name}
                  </Typography>
                  <Typography variant="caption">
                    A git repository will be created with this template for context-aware planning by Zed agents.
                  </Typography>
                </Alert>
              )}
            </Box>
          </Stack>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setCreateDialogOpen(false)}>
            Cancel
          </Button>
          <Button
            onClick={handleCreateTask}
            variant="contained"
            disabled={!newTaskName.trim() || !newTaskDescription.trim() || !selectedSampleType || createSampleRepoLoading}
            startIcon={createSampleRepoLoading ? <CircularProgress size={16} /> : <AddIcon />}
          >
            {createSampleRepoLoading ? 'Creating...' : 'Create SpecTask'}
          </Button>
        </DialogActions>
      </Dialog>
    </Page>
  );
};

export default SpecTasksPage;
