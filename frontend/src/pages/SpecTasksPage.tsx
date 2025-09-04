import React, { FC, useState, useEffect } from 'react';
import {
  Container,
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
import gitRepositoryService, { 
  useSampleTypes,
  useCreateSampleRepository,
  SampleType,
  getSampleTypeIcon,
  getSampleTypeCategory,
  isBusinessTask,
  getBusinessTaskDescription,
} from '../services/gitRepositoryService';
import specTaskService, {
  SpecTask,
  useSpecTask,
} from '../services/specTaskService';

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
      {...other}
    >
      {value === index && <Box sx={{ py: 3 }}>{children}</Box>}
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
  const { data: sampleTypes, isLoading: sampleTypesLoading } = useSampleTypes();
  const createSampleRepo = useCreateSampleRepository();
  const { data: selectedTask } = useSpecTask(selectedTaskId || '');

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
      const repository = await createSampleRepo.mutateAsync({
        name: `${newTaskName} - ${selectedSampleType}`,
        description: newTaskDescription,
        owner_id: account.user?.id || '',
        sample_type: selectedSampleType,
      });

      // Then create the SpecTask
      const response = await api.post('/api/v1/spec-tasks/from-prompt', {
        name: newTaskName,
        description: newTaskDescription,
        type: taskType,
        priority: taskPriority,
        project_id: account.organizationTools.organization?.id || 'default',
        requirements: newTaskRequirements,
        git_repository_id: repository.id,
        git_repository_url: repository.clone_url,
      });

      if (response.data) {
        snackbar.success('SpecTask created successfully!');
        setCreateDialogOpen(false);
        resetCreateForm();
        
        // Refresh the kanban board
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
    if (!sampleTypes?.sample_types) return { development: [], business: [], content: [], other: [] };

    const categorized = {
      development: [] as SampleType[],
      business: [] as SampleType[],
      content: [] as SampleType[],
      other: [] as SampleType[],
    };

    sampleTypes.sample_types.forEach((type: SampleType) => {
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
        <Stack direction="row" spacing={2}>
          <Button
            variant="outlined"
            startIcon={refreshing ? <CircularProgress size={16} /> : <RefreshIcon />}
            onClick={() => {
              setRefreshing(true);
              setTimeout(() => setRefreshing(false), 2000);
            }}
            disabled={refreshing}
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
          >
            New SpecTask
          </Button>
        </Stack>
      }
    >
      <Container maxWidth="xl" sx={{ height: 'calc(100vh - 120px)', display: 'flex', flexDirection: 'column' }}>
        {/* Introduction */}
        <Box sx={{ mb: 2 }}>
          <Typography variant="h4" sx={{ fontWeight: 600, mb: 1 }}>
            SpecTask Management
          </Typography>
          <Typography variant="body1" color="text.secondary">
            Manage multi-session development and business tasks with git-based coordination
          </Typography>
        </Box>

        {/* Tabs for different views */}
        <Box sx={{ borderBottom: 1, borderColor: 'divider', mb: 2 }}>
          <Tabs value={currentTab} onChange={handleTabChange} aria-label="spec-tasks-tabs">
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
        <Box sx={{ flex: 1, overflow: 'hidden' }}>
          <TabPanel value={currentTab} index={0}>
            <SpecTaskKanbanBoard
              projectId={account.organizationTools.organization?.id}
              userId={account.user?.id}
              onTaskClick={handleTaskClick}
            />
          </TabPanel>

          <TabPanel value={currentTab} index={1}>
            <SpecTaskTable
              tasks={[]}
              loading={false}
              onTaskSelect={handleTaskClick}
              onRefresh={() => {
                setRefreshing(true);
                setTimeout(() => setRefreshing(false), 2000);
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
      </Container>

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
                  <InputLabel>Choose a template or start empty</InputLabel>
                  <Select
                    value={selectedSampleType}
                    onChange={(e) => setSelectedSampleType(e.target.value)}
                    label="Choose a template or start empty"
                  >
                    {/* Development Templates */}
                    {categorizedSampleTypes.development.length > 0 && (
                      <>
                        <Typography variant="overline" sx={{ px: 2, py: 1, color: 'primary.main', fontWeight: 600 }}>
                          Development Templates
                        </Typography>
                        {categorizedSampleTypes.development.map((type: SampleType) => (
                          <MenuItem key={type.id} value={type.id}>
                            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, width: '100%' }}>
                              <Typography sx={{ fontSize: '1.2em' }}>
                                {getSampleTypeIcon(type.id || '')}
                              </Typography>
                              <Box sx={{ flex: 1 }}>
                                <Typography variant="body2" sx={{ fontWeight: 600 }}>
                                  {type.name}
                                </Typography>
                                <Typography variant="caption" color="text.secondary">
                                  {type.description}
                                </Typography>
                              </Box>
                            </Box>
                          </MenuItem>
                        ))}
                      </>
                    )}

                    {/* Business Templates */}
                    {categorizedSampleTypes.business.length > 0 && (
                      <>
                        <Typography variant="overline" sx={{ px: 2, py: 1, color: 'secondary.main', fontWeight: 600 }}>
                          Business Templates
                        </Typography>
                        {categorizedSampleTypes.business.map((type: SampleType) => (
                          <MenuItem key={type.id} value={type.id}>
                            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, width: '100%' }}>
                              <Typography sx={{ fontSize: '1.2em' }}>
                                {getSampleTypeIcon(type.id || '')}
                              </Typography>
                              <Box sx={{ flex: 1 }}>
                                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                                  <Typography variant="body2" sx={{ fontWeight: 600 }}>
                                    {type.name}
                                  </Typography>
                                  <Chip 
                                    label="Business" 
                                    size="small" 
                                    color="secondary" 
                                    sx={{ height: 16 }}
                                  />
                                </Box>
                                <Typography variant="caption" color="text.secondary">
                                  {getBusinessTaskDescription(type.id || '')}
                                </Typography>
                              </Box>
                            </Box>
                          </MenuItem>
                        ))}
                      </>
                    )}

                    {/* Content Templates */}
                    {categorizedSampleTypes.content.length > 0 && (
                      <>
                        <Typography variant="overline" sx={{ px: 2, py: 1, color: 'success.main', fontWeight: 600 }}>
                          Content Templates
                        </Typography>
                        {categorizedSampleTypes.content.map((type: SampleType) => (
                          <MenuItem key={type.id} value={type.id}>
                            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, width: '100%' }}>
                              <Typography sx={{ fontSize: '1.2em' }}>
                                {getSampleTypeIcon(type.id || '')}
                              </Typography>
                              <Box sx={{ flex: 1 }}>
                                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                                  <Typography variant="body2" sx={{ fontWeight: 600 }}>
                                    {type.name}
                                  </Typography>
                                  <Chip 
                                    label="Content" 
                                    size="small" 
                                    color="success" 
                                    sx={{ height: 16 }}
                                  />
                                </Box>
                                <Typography variant="caption" color="text.secondary">
                                  {type.description}
                                </Typography>
                              </Box>
                            </Box>
                          </MenuItem>
                        ))}
                      </>
                    )}
                  </Select>
                </FormControl>
              )}
              
              {selectedSampleType && (
                <Alert severity="info" sx={{ mt: 2 }}>
                  <Typography variant="body2">
                    <strong>Selected Template:</strong> {sampleTypes?.sample_types?.find(t => t.id === selectedSampleType)?.name}
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
            disabled={!newTaskName.trim() || !newTaskDescription.trim() || !selectedSampleType || createSampleRepo.isPending}
            startIcon={createSampleRepo.isPending ? <CircularProgress size={16} /> : <AddIcon />}
          >
            {createSampleRepo.isPending ? 'Creating...' : 'Create SpecTask'}
          </Button>
        </DialogActions>
      </Dialog>
    </Page>
  );
};

export default SpecTasksPage;