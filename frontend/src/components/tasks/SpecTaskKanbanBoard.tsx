import React, { useState, useEffect, useMemo, useCallback } from 'react';
import {
  Box,
  Typography,
  Card,
  CardContent,
  CardHeader,
  Grid,
  Button,
  Chip,
  LinearProgress,
  CircularProgress,
  Alert,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  TextField,
  IconButton,
  Collapse,
  Stack,
  Divider,
  Badge,
  Tooltip,
  Menu,
  MenuItem,
  Select,
  FormControl,
  InputLabel,
  Avatar,
} from '@mui/material';
import { useSortable } from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';
import {
  Add as AddIcon,
  ExpandMore as ExpandMoreIcon,
  ExpandLess as ExpandLessIcon,
  Source as GitIcon,
  Description as SpecIcon,
  PlayArrow as PlayIcon,
  CheckCircle as ApproveIcon,
  Cancel as CancelIcon,
  MoreVert as MoreIcon,
  Code as CodeIcon,
  Timeline as TimelineIcon,
  AccountTree as TreeIcon,
  Visibility as ViewIcon,
  Edit as EditIcon,
  Delete as DeleteIcon,
  Close as CloseIcon,
  Refresh as RefreshIcon,
  Restore as RestoreIcon,
  VisibilityOff as VisibilityOffIcon,
} from '@mui/icons-material';
// Removed drag-and-drop imports to prevent infinite loops
import { useTheme } from '@mui/material/styles';
// Removed date-fns dependency - using native JavaScript instead

import useApi from '../../hooks/useApi';
import useAccount from '../../hooks/useAccount';
import useRouter from '../../hooks/useRouter';
import ExternalAgentDesktopViewer from '../external-agent/ExternalAgentDesktopViewer';
import DesignDocViewer from './DesignDocViewer';
import DesignReviewViewer from '../spec-tasks/DesignReviewViewer';
import specTaskService, {
  SpecTask,
  MultiSessionOverview,
  useSpecTask,
  useMultiSessionOverview,
} from '../../services/specTaskService';
import gitRepositoryService, {
  SampleType,
  useCreateSampleRepository,
  getSampleTypeCategory,
  isBusinessTask,
  getBusinessTaskDescription,
} from '../../services/gitRepositoryService';
import { useSampleTypes } from '../../hooks/useSampleTypes';

// Minimal wrapper for desktop viewer with custom overlay for Kanban cards
// Uses screenshot mode (not live stream) to avoid performance issues with many cards
const LiveAgentScreenshot: React.FC<{
  sessionId: string;
  projectId?: string;
}> = React.memo(({ sessionId, projectId }) => {
  return (
    <Box
      sx={{
        mt: 1,
        mb: 1,
        position: 'relative',
        borderRadius: 1,
        overflow: 'hidden',
        border: '1px solid',
        borderColor: 'divider',
        minHeight: 80,
      }}
    >
      <Box sx={{ position: 'relative', height: 150 }}>
        <ExternalAgentDesktopViewer
          sessionId={sessionId}
          height={150}
          mode="screenshot"
        />
      </Box>
      <Box
        sx={{
          position: 'absolute',
          bottom: 0,
          left: 0,
          right: 0,
          background: 'linear-gradient(to top, rgba(0,0,0,0.8), transparent)',
          color: 'white',
          p: 0.5,
          px: 1,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          pointerEvents: 'none',
        }}
      >
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
          <ViewIcon fontSize="small" />
          <Typography variant="caption" sx={{ fontWeight: 500 }}>
            Planning Agent
          </Typography>
        </Box>
      </Box>
    </Box>
  );
});

// SpecTask types and statuses
type SpecTaskPhase = 'backlog' | 'planning' | 'review' | 'implementation' | 'completed';
type SpecTaskPriority = 'low' | 'medium' | 'high' | 'critical';

interface SpecTaskWithExtras extends SpecTask {
  hasSpecs: boolean;
  phase: SpecTaskPhase;
  planningStatus?: 'none' | 'active' | 'pending_review' | 'completed' | 'failed';
  gitRepositoryId?: string;
  gitRepositoryUrl?: string;
  multiSessionOverview?: MultiSessionOverview;
  lastActivity?: string;
  activeSessionsCount?: number;
  completedSessionsCount?: number;
  specApprovalNeeded?: boolean;
  onReviewDocs?: (task: SpecTaskWithExtras) => void;
}

interface KanbanColumn {
  id: SpecTaskPhase;
  title: string;
  color: string;
  backgroundColor: string;
  description: string;
  limit?: number;
  tasks: SpecTaskWithExtras[];
}

interface SpecTaskKanbanBoardProps {
  userId?: string;
  projectId?: string; // Filter tasks by project ID
  onCreateTask?: () => void;
  onTaskClick?: (task: any) => void;
  onRefresh?: () => void;
  refreshing?: boolean;
  refreshTrigger?: number;
  wipLimits?: {
    planning: number;
    review: number;
    implementation: number;
  };
  // repositories prop removed - repos are now managed at project level
}

// TaskCard component - moved outside to prevent recreation on every render
const TaskCard: React.FC<{
  task: SpecTaskWithExtras;
  index: number;
  columns: KanbanColumn[];
  onStartPlanning?: (task: SpecTaskWithExtras) => Promise<void>;
  onArchiveTask?: (task: SpecTaskWithExtras, archived: boolean) => Promise<void>;
  onTaskClick?: (task: SpecTaskWithExtras) => void;
  projectId?: string;
}> = ({ task, index, columns, onStartPlanning, onArchiveTask, onTaskClick, projectId }) => {
  const account = useAccount();

  // Check if planning column is full
  const planningColumn = columns.find(col => col.id === 'planning');
  const isPlanningFull = planningColumn && planningColumn.limit
    ? planningColumn.tasks.length >= planningColumn.limit
    : false;

  const handleStartPlanning = async (e: React.MouseEvent) => {
    e.stopPropagation();
    if (onStartPlanning) {
      await onStartPlanning(task);
    }
  };

  return (
    <Card
      onClick={() => onTaskClick && onTaskClick(task)}
      sx={{
        mb: 1,
        backgroundColor: 'background.paper',
        cursor: 'pointer',
        '&:hover': {
          boxShadow: 2,
        },
      }}
    >
      <CardContent sx={{ p: 2, '&:last-child': { pb: 2 } }}>
        <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', mb: 1 }}>
          <Typography variant="subtitle2" sx={{ fontWeight: 'bold', flex: 1 }}>
            {task.name}
          </Typography>
          <Tooltip title={task.archived ? "Restore" : "Archive"}>
            <IconButton
              size="small"
              onClick={(e) => {
                e.stopPropagation();
                if (onArchiveTask) {
                  onArchiveTask(task, !task.archived);
                }
              }}
              sx={{ ml: 1 }}
            >
              {task.archived ? <RestoreIcon fontSize="small" /> : <CloseIcon fontSize="small" />}
            </IconButton>
          </Tooltip>
        </Box>

        {/* Session chips - repositories managed at project level */}
        {((task.activeSessionsCount ?? 0) > 0 || (task.completedSessionsCount ?? 0) > 0) && (
          <Box sx={{ display: 'flex', gap: 1, flexWrap: 'wrap', alignItems: 'center', mb: 1 }}>
            {(task.activeSessionsCount ?? 0) > 0 && (
              <Chip
                size="small"
                label={`${task.activeSessionsCount ?? 0} Active`}
                color="warning"
              />
            )}

            {(task.completedSessionsCount ?? 0) > 0 && (
              <Chip
                size="small"
                label={`${task.completedSessionsCount ?? 0} Done`}
                color="success"
              />
            )}
          </Box>
        )}

        {/* Live screenshot widget for active planning sessions */}
        {task.spec_session_id && (
          <LiveAgentScreenshot
            sessionId={task.spec_session_id}
            projectId={projectId}
          />
        )}

        {/* Show "Start Planning" button only for backlog tasks */}
        {task.phase === 'backlog' && (
          <Box sx={{ mt: 1 }}>
            {/* Show error if present */}
            {task.metadata?.error && (
              <Box sx={{
                mb: 1,
                p: 1,
                backgroundColor: 'error.light',
                borderRadius: 1,
                border: '1px solid',
                borderColor: 'error.main'
              }}>
                <Typography variant="caption" color="error.dark" sx={{ fontWeight: 600 }}>
                  ⚠️ {task.metadata.error as string}
                </Typography>
              </Box>
            )}
            <Button
              size="small"
              variant="contained"
              color="warning"
              startIcon={<PlayIcon />}
              onClick={handleStartPlanning}
              disabled={isPlanningFull}
              fullWidth
            >
              {task.metadata?.error ? 'Retry Planning' : isPlanningFull ? 'Planning Full' : 'Start Planning'}
            </Button>
            {isPlanningFull && (
              <Typography variant="caption" color="error" sx={{ mt: 0.5, display: 'block', textAlign: 'center' }}>
                Planning column full ({planningColumn?.limit} max)
              </Typography>
            )}
          </Box>
        )}

        {/* Show "Review Design" button for review phase tasks */}
        {task.phase === 'review' && task.onReviewDocs && (
          <Box sx={{ mt: 1 }}>
            <Button
              size="small"
              variant="contained"
              color="info"
              startIcon={<SpecIcon />}
              onClick={(e) => {
                e.stopPropagation();
                task.onReviewDocs(task);
              }}
              fullWidth
              sx={{
                background: 'linear-gradient(45deg, #2196f3 30%, #21cbf3 90%)',
                boxShadow: '0 3px 5px 2px rgba(33, 203, 243, .3)',
              }}
            >
              Review Design
            </Button>
          </Box>
        )}
      </CardContent>
    </Card>
  );
};

const DroppableColumn: React.FC<{
  column: KanbanColumn;
  columns: KanbanColumn[];
  onStartPlanning?: (task: SpecTaskWithExtras) => Promise<void>;
  onArchiveTask?: (task: SpecTaskWithExtras, archived: boolean) => Promise<void>;
  onTaskClick?: (task: SpecTaskWithExtras) => void;
  projectId?: string;
  theme: any;
}> = ({ column, columns, onStartPlanning, onArchiveTask, onTaskClick, projectId, theme }): JSX.Element => {
  const router = useRouter();
  const account = useAccount();

  // Simplified - no drag and drop, no complex interactions
  const setNodeRef = (node: HTMLElement | null) => {};

  // Render task card wrapper - simplified
  const renderTaskCard = (task: SpecTaskWithExtras, index: number) => {
    return (
      <TaskCard
        key={task.id || `task-${index}`}
        task={task}
        index={index}
        columns={columns}
        onStartPlanning={onStartPlanning}
        onArchiveTask={onArchiveTask}
        onTaskClick={onTaskClick}
        projectId={projectId}
      />
    );
  };

    return (
      <Box key={column.id} sx={{ width: 280, flexShrink: 0, height: '100%' }}>
        <Card sx={{ height: '100%', display: 'flex', flexDirection: 'column' }}>
          <CardHeader
            title={
              <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                <Typography variant="h6" sx={{ color: column.color }}>
                  {column.title}
                </Typography>
                <Chip
                  size="small"
                  label={column.tasks.length}
                  sx={{
                    backgroundColor: column.backgroundColor,
                    color: column.color,
                    minWidth: '24px'
                  }}
                />
                {column.limit && column.tasks.length >= column.limit && (
                  <Chip size="small" label="FULL" color="error" />
                )}
              </Box>
            }
            sx={{
              pb: 1,
              flexShrink: 0,
              '& .MuiCardHeader-title': { fontSize: '1rem' }
            }}
          />
          <CardContent
            ref={setNodeRef}
            sx={{
              flex: 1,
              minHeight: 0,
              overflowY: 'auto',
              overflowX: 'hidden',
              backgroundColor: 'transparent',
              p: 1,
              '&:last-child': { pb: 1 }
            }}
          >
            {column.tasks.map((task, index) => renderTaskCard(task, index))}
            {column.tasks.length === 0 && (
              <Typography
                variant="body2"
                color="text.secondary"
                sx={{ textAlign: 'center', py: 2 }}
              >
                No tasks
              </Typography>
            )}
          </CardContent>
        </Card>
      </Box>
    );
};

const SpecTaskKanbanBoard: React.FC<SpecTaskKanbanBoardProps> = ({
  userId,
  projectId,
  onCreateTask,
  onTaskClick,
  onRefresh,
  refreshing = false,
  refreshTrigger,
  wipLimits = { planning: 3, review: 2, implementation: 5 },
}) => {
  const theme = useTheme();
  const api = useApi();
  const account = useAccount();
  
  console.log('🔥 KANBAN COMPONENT MOUNTED with props:', { userId });

  // State
  const [tasks, setTasks] = useState<SpecTaskWithExtras[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [planningDialogOpen, setPlanningDialogOpen] = useState(false);
  const [selectedTask, setSelectedTask] = useState<SpecTaskWithExtras | null>(null);
  const [showArchived, setShowArchived] = useState(false);
  const [archiveConfirmOpen, setArchiveConfirmOpen] = useState(false);
  const [taskToArchive, setTaskToArchive] = useState<SpecTaskWithExtras | null>(null);

  // Design doc viewer state
  const [docViewerOpen, setDocViewerOpen] = useState(false);
  const [reviewingTask, setReviewingTask] = useState<SpecTaskWithExtras | null>(null);
  const [designReviewViewerOpen, setDesignReviewViewerOpen] = useState(false);
  const [activeReviewId, setActiveReviewId] = useState<string | null>(null);

  // Planning form state
  const [newTaskRequirements, setNewTaskRequirements] = useState('');
  const [selectedSampleType, setSelectedSampleType] = useState('');

  // Available sample types for planning
  const [sampleTypes, setSampleTypes] = useState<any[]>([]);

  // Debug sample types data
  useEffect(() => {
    console.log('Sample types data updated:', sampleTypes);
  }, [sampleTypes]);

  // WIP limits for kanban columns (use prop values or defaults)
  const WIP_LIMITS = {
    backlog: undefined,
    planning: wipLimits.planning,
    review: wipLimits.review,
    implementation: wipLimits.implementation,
    completed: undefined,
  };

  // Kanban columns configuration
  const columns: KanbanColumn[] = useMemo(() => [
    {
      id: 'backlog',
      title: 'Backlog',
      color: theme.palette.mode === 'dark' ? theme.palette.grey[400] : theme.palette.text.secondary,
      backgroundColor: theme.palette.mode === 'dark' ? theme.palette.grey[800] : theme.palette.grey[200],
      description: 'Tasks without specifications',
      tasks: tasks.filter(t => (t as any).phase === 'backlog' && !t.hasSpecs),
    },
    {
      id: 'planning',
      title: 'Planning',
      color: theme.palette.warning.main,
      backgroundColor: theme.palette.warning.light + '20',
      description: 'Specs being generated by Zed agents',
      limit: WIP_LIMITS.planning,
      tasks: tasks.filter(t => (t as any).phase === 'planning' || t.planningStatus === 'active'),
    },
    {
      id: 'review',
      title: 'Spec Review',
      color: theme.palette.info.main,
      backgroundColor: theme.palette.info.light + '20',
      description: 'Specs ready for human review',
      limit: WIP_LIMITS.review,
      tasks: tasks.filter(t => (t as any).phase === 'review' || t.specApprovalNeeded),
    },
    {
      id: 'implementation',
      title: 'Implementation',
      color: theme.palette.success.main,
      backgroundColor: theme.palette.success.light + '20',
      description: 'Multi-session implementation in progress',
      limit: WIP_LIMITS.implementation,
      tasks: tasks.filter(t => (t as any).phase === 'implementation' && (t.activeSessionsCount || 0) > 0),
    },
    {
      id: 'completed',
      title: 'Completed',
      color: theme.palette.success.dark,
      backgroundColor: theme.palette.success.dark + '20',
      description: 'Completed tasks',
      tasks: tasks.filter(t => (t as any).phase === 'completed' || t.status === 'completed'),
    },
  ], [tasks, theme, wipLimits]);

  // Load sample types using generated client
  const { data: sampleTypesData, loading: sampleTypesLoading } = useSampleTypes();

  // Initial load
  useEffect(() => {
    if (!account.user?.id) return;

    const loadTasks = async () => {
      try {
        setLoading(true);
        const response = await api.get('/api/v1/spec-tasks', {
          params: {
            project_id: projectId || 'default',
            archived_only: showArchived
          }
        });

        const tasksData = response.data || response;
        const specTasks: SpecTask[] = Array.isArray(tasksData) ? tasksData : [];

        // Better phase mapping based on actual status
        const enhancedTasks: SpecTaskWithExtras[] = specTasks.map((task) => {
          let phase: SpecTaskPhase = 'backlog';
          let planningStatus: 'none' | 'active' | 'pending_review' | 'completed' | 'failed' = 'none';

          if (task.status === 'spec_generation') {
            phase = 'planning';
            planningStatus = 'active';
          } else if (task.status === 'spec_review') {
            phase = 'review';
            planningStatus = 'pending_review';
          } else if (task.status === 'spec_approved') {
            phase = 'implementation';
            planningStatus = 'completed';
          } else if (task.status === 'implementing') {
            phase = 'implementation';
            planningStatus = 'completed';
          } else if (task.status === 'completed') {
            phase = 'completed';
            planningStatus = 'completed';
          }

          // Check for errors in metadata (tasks stay in backlog with error)
          if (task.status === 'backlog' && task.metadata?.error) {
            planningStatus = 'failed';
          }

          return {
            ...task,
            hasSpecs: task.status !== 'backlog',
            phase,
            planningStatus,
            activeSessionsCount: 0,
            completedSessionsCount: 0,
            onReviewDocs: handleReviewDocs,
          };
        });

        setTasks(enhancedTasks);
      } catch (err) {
        console.error('Failed to load tasks:', err);
        setError('Failed to load tasks');
      } finally {
        setLoading(false);
      }
    };

    loadTasks();
  }, [account.user?.id, projectId, showArchived, refreshTrigger]);

  // Set up polling for real-time updates
  useEffect(() => {
    if (!account.user?.id) return;

    const interval = setInterval(async () => {
      try {
        const response = await api.get('/api/v1/spec-tasks', {
          params: {
            project_id: projectId || 'default',
            archived_only: showArchived
          }
        });

        const tasksData = response.data || response;
        const specTasks: SpecTask[] = Array.isArray(tasksData) ? tasksData : [];

        const enhancedTasks: SpecTaskWithExtras[] = specTasks.map((task) => {
          let phase: SpecTaskPhase = 'backlog';
          let planningStatus: 'none' | 'active' | 'pending_review' | 'completed' | 'failed' = 'none';

          if (task.status === 'spec_generation') {
            phase = 'planning';
            planningStatus = 'active';
          } else if (task.status === 'spec_review') {
            phase = 'review';
            planningStatus = 'pending_review';
          } else if (task.status === 'spec_approved') {
            phase = 'implementation';
            planningStatus = 'completed';
          } else if (task.status === 'implementing') {
            phase = 'implementation';
            planningStatus = 'completed';
          } else if (task.status === 'completed') {
            phase = 'completed';
            planningStatus = 'completed';
          }

          // Check for errors in metadata (tasks stay in backlog with error)
          if (task.status === 'backlog' && task.metadata?.error) {
            planningStatus = 'failed';
          }

          return {
            ...task,
            hasSpecs: task.status !== 'backlog',
            phase,
            planningStatus,
            activeSessionsCount: 0,
            completedSessionsCount: 0,
            onReviewDocs: handleReviewDocs,
          };
        });

        setTasks(enhancedTasks);
      } catch (err) {
        console.error('Failed to poll tasks:', err);
      }
    }, 3000); // Faster polling: 3 seconds instead of 10 for responsive updates

    return () => clearInterval(interval);
  }, [account.user?.id, projectId, showArchived]);

  // Update sample types when data loads
  useEffect(() => {
    console.log('Raw sampleTypesData:', sampleTypesData);
    if (sampleTypesData && sampleTypesData.length > 0) {
      console.log('Setting sample types:', sampleTypesData);
      setSampleTypes(sampleTypesData);
    }
  }, [sampleTypesData]);

  // Removed auto-refresh to prevent infinite loops

  // Update sample types when data loads
  useEffect(() => {
    if (sampleTypesData && sampleTypesData.length > 0) {
      setSampleTypes(sampleTypesData);
    }
  }, [sampleTypesData]);



  // Handle assigning agent to task
  const handleAssignAgent = useCallback(async (taskId: string, agentType: string) => {
    try {
      console.log('Starting agent session for task:', taskId, 'with agent:', agentType);
      
      const response = await api.post(`/api/v1/agents/work`, {
        name: tasks.find(t => t.id === taskId)?.name || 'Spec Generation Task',
        description: `Generate specifications for: ${tasks.find(t => t.id === taskId)?.description || ''}`,
        source: 'spec_task_kanban',
        source_id: taskId,
        priority: 5,
        agent_type: agentType,
        work_data: {
          task_id: taskId,
          project_id: projectId || 'default',
          action: 'generate_specs'
        }
      });

      if (response && response.data) {
        console.log('Agent session started:', response.data);

        // Don't update task status from frontend - let backend handle this
        // The backend should update task status based on agent progress
        // Reload tasks immediately to show the session was created
        try {
          const refreshResponse = await api.get('/api/v1/spec-tasks', {
            params: { project_id: projectId || 'default' }
          });
          
          const tasksData = refreshResponse.data || refreshResponse;
          const specTasks: SpecTask[] = Array.isArray(tasksData) ? tasksData : [];
          
          const enhancedTasks: SpecTaskWithExtras[] = specTasks.map((task) => {
            let phase: SpecTaskPhase = 'backlog';
            let planningStatus: 'none' | 'active' | 'pending_review' | 'completed' | 'failed' = 'none';
            
            if (task.status === 'spec_generation') {
              phase = 'planning';
              planningStatus = 'active';
            } else if (task.status === 'spec_review') {
              phase = 'review';
              planningStatus = 'pending_review';
            } else if (task.status === 'spec_approved') {
              phase = 'implementation';
              planningStatus = 'completed';
            } else if (task.status === 'implementing') {
              phase = 'implementation';
              planningStatus = 'completed';
            } else if (task.status === 'completed') {
              phase = 'completed';
              planningStatus = 'completed';
            }
            
            return {
              ...task,
              hasSpecs: task.status !== 'backlog',
              phase,
              planningStatus,
              activeSessionsCount: 0,
              completedSessionsCount: 0,
            };
          });

          setTasks(enhancedTasks);
        } catch (err) {
          console.error('Failed to reload tasks after agent assignment:', err);
        }
      }
    } catch (error) {
      console.error('Failed to assign agent:', error);
      setError('Failed to start agent session. Please try again.');
    }
  }, []);

  // Handle phase transitions with appropriate actions
  const handlePhaseTransition = async (task: SpecTaskWithExtras, newPhase: SpecTaskPhase) => {
    try {
      if (newPhase === 'planning' && !task.hasSpecs) {
        // Start planning session
        setSelectedTask(task);
        setPlanningDialogOpen(true);
      } else if (newPhase === 'review' && task.planningStatus === 'pending_review') {
        // Specs are ready for review - just update the local state
        // The actual review will be done through the review interface
        setTasks(prev => prev.map(t => 
          t.id === task.id ? { ...t, phase: newPhase } : t
        ));
      } else if (newPhase === 'implementation' && task.hasSpecs) {
        // Start implementation sessions
        await startImplementation(task);
      } else {
        // Generic status update
        await updateTaskStatus(task.id || '', newPhase);
      }
    } catch (err) {
      console.error('Failed to handle phase transition:', err);
      setError('Failed to update task. Please try again.');
    }
  };

  // Start planning session for a task
  const createSampleRepoMutation = useCreateSampleRepository();
  
  const startPlanning = async (task: SpecTaskWithExtras, sampleType?: string) => {
    try {
      if (!sampleType || !account.user?.id) {
        setError('Sample type and user ID are required');
        return;
      }

      // First create the sample repository
      const sampleRepo = await createSampleRepoMutation.mutateAsync({
        name: `${task.name} - ${sampleType}`,
        description: task.description,
        owner_id: account.user.id,
        sample_type: sampleType,
      });

      if (sampleRepo) {
        // Update task with repository info
        setTasks(prev => prev.map(t => 
          t.id === task.id 
            ? { 
                ...t, 
                phase: 'planning', 
                planningStatus: 'active',
                gitRepositoryId: sampleRepo.id,
                gitRepositoryUrl: sampleRepo.clone_url
              }
            : t
        ));
        
        setPlanningDialogOpen(false);
        setSelectedTask(null);
        setNewTaskRequirements('');
        setSelectedSampleType('');
      }
    } catch (err) {
      console.error('Failed to start planning:', err);
      setError('Failed to start planning session. Please try again.');
    }
  };

  // Start implementation sessions
  const startImplementation = async (task: SpecTaskWithExtras) => {
    try {
      const response = await api.post(`/api/v1/spec-tasks/${task.id}/implementation-sessions`, {
        session_count: 3, // Default to 3 parallel sessions
        agent_types: ['zed', 'zed', 'zed'],
      });

      if (response.data) {
        // Update task status
        setTasks(prev => prev.map(t => 
          t.id === task.id 
            ? { 
                ...t, 
                phase: 'implementation',
                activeSessionsCount: response.data.work_session_count || 3
              }
            : t
        ));
      }
    } catch (err) {
      console.error('Failed to start implementation:', err);
      setError('Failed to start implementation. Please try again.');
    }
  };

  // Update task status
  const updateTaskStatus = async (taskId: string, phase: SpecTaskPhase) => {
    try {
      // Map phase to status
      const statusMap: Record<SpecTaskPhase, string> = {
        backlog: 'draft',
        planning: 'planning',
        review: 'pending_approval',
        implementation: 'implementing',
        completed: 'completed',
      };

      await api.put(`/api/v1/spec-tasks/${taskId}`, {
        status: statusMap[phase],
      });

      // Update local state
      setTasks(prev => prev.map(t => 
        t.id === taskId ? { ...t, phase, status: statusMap[phase] } : t
      ));
    } catch (err) {
      console.error('Failed to update task status:', err);
      throw err;
    }
  };

  // Create new SpecTask
  const createTask = async () => {
    try {
      const response = await api.post('/api/v1/spec-tasks/from-prompt', {
        name: newTaskName,
        description: newTaskDescription,
        project_id: projectId || 'default',
      });

      if (response.data) {
        // Add to local state
        const newTask: SpecTaskWithExtras = {
          ...response.data,
          hasSpecs: false,
          planningStatus: 'none',
          phase: 'backlog',
          activeSessionsCount: 0,
          completedSessionsCount: 0,
        };
        
        setTasks(prev => [...prev, newTask]);
        setCreateDialogOpen(false);
        setNewTaskName('');
        setNewTaskDescription('');
        setSelectedSampleType('');
      }
    } catch (err) {
      console.error('Failed to create task:', err);
      setError('Failed to create task. Please try again.');
    }
  };

  // Get priority color
  const getPriorityColor = (priority: string) => {
    switch (priority) {
      case 'critical': return theme.palette.error.main;
      case 'high': return theme.palette.warning.main;
      case 'medium': return theme.palette.info.main;
      case 'low': return theme.palette.success.main;
      default: return theme.palette.grey[500];
    }
  };

  // Get spec status icon and color
  const getSpecStatusInfo = (task: SpecTaskWithExtras) => {
    if (!task.hasSpecs && task.planningStatus === 'none') {
      return { icon: <SpecIcon />, color: theme.palette.error.main, text: 'No specs' };
    } else if (task.planningStatus === 'active') {
      return { icon: <CircularProgress size={16} />, color: theme.palette.warning.main, text: 'Generating specs' };
    } else if (task.planningStatus === 'pending_review') {
      return { icon: <ViewIcon />, color: theme.palette.info.main, text: 'Review needed' };
    } else if (task.planningStatus === 'failed') {
      return { icon: <CancelIcon />, color: theme.palette.error.main, text: 'Planning failed' };
    } else if (task.planningStatus === 'completed') {
      return { icon: <ApproveIcon />, color: theme.palette.success.main, text: 'Specs approved' };
    }
    return { icon: <SpecIcon />, color: theme.palette.grey[500], text: 'Unknown' };
  };

  // Handle archiving/unarchiving a task
  const handleArchiveTask = async (task: SpecTaskWithExtras, archived: boolean) => {
    // If archiving (not unarchiving), show confirmation dialog
    if (archived) {
      setTaskToArchive(task);
      setArchiveConfirmOpen(true);
      return;
    }

    // Unarchiving doesn't need confirmation
    await performArchive(task, archived);
  };

  // Actually perform the archive operation (called after confirmation or for unarchive)
  const performArchive = async (task: SpecTaskWithExtras, archived: boolean) => {
    try {
      await api.getApiClient().v1SpecTasksArchivePartialUpdate(task.id!, { archived });

      // Refresh tasks
      const response = await api.get('/api/v1/spec-tasks', {
        params: {
          project_id: projectId || 'default',
          archived_only: showArchived
        }
      });

      const tasksData = response.data || response;
      const specTasks: SpecTask[] = Array.isArray(tasksData) ? tasksData : [];

      const enhancedTasks: SpecTaskWithExtras[] = specTasks.map((t) => {
        let phase: SpecTaskPhase = 'backlog';
        let planningStatus: 'none' | 'active' | 'pending_review' | 'completed' | 'failed' = 'none';

        if (t.status === 'spec_generation') {
          phase = 'planning';
          planningStatus = 'active';
        } else if (t.status === 'spec_review') {
          phase = 'review';
          planningStatus = 'pending_review';
        } else if (t.status === 'spec_approved') {
          phase = 'implementation';
          planningStatus = 'completed';
        } else if (t.status === 'implementing') {
          phase = 'implementation';
          planningStatus = 'completed';
        } else if (t.status === 'completed') {
          phase = 'completed';
          planningStatus = 'completed';
        }

        return {
          ...t,
          hasSpecs: t.status === 'spec_approved' || t.status === 'implementing' || t.status === 'completed',
          planningStatus,
          phase,
          activeSessionsCount: 0,
          completedSessionsCount: 0,
        };
      });

      setTasks(enhancedTasks);
    } catch (error) {
      console.error('Failed to archive task:', error);
      setError('Failed to archive task');
    }
  };

  // Handle starting planning for a task
  const handleStartPlanning = async (task: SpecTaskWithExtras) => {
    // Check WIP limit
    const planningColumn = columns.find(col => col.id === 'planning');
    const isPlanningFull = planningColumn && planningColumn.limit
      ? planningColumn.tasks.length >= planningColumn.limit
      : false;

    if (isPlanningFull) {
      setError(`Planning column is full (${planningColumn?.limit} tasks max). Please complete existing planning tasks first.`);
      return;
    }

    try {
      // Call the start-planning endpoint which actually starts spec generation
      await api.getApiClient().v1SpecTasksStartPlanningCreate(task.id!);

      // Aggressive polling after starting planning to catch spec_session_id update
      // Poll at 1s, 2s, 4s, 6s intervals to catch the async session creation
      const pollForSessionId = async (retryCount = 0, maxRetries = 6) => {
        const response = await api.getApiClient().v1SpecTasksList({
          project_id: projectId || 'default'
        });

        const tasksData = response.data || response;
        const specTasks: SpecTask[] = Array.isArray(tasksData) ? tasksData : [];

        const enhancedTasks: SpecTaskWithExtras[] = specTasks.map((t) => {
          let phase: SpecTaskPhase = 'backlog';
          let planningStatus: 'none' | 'active' | 'pending_review' | 'completed' | 'failed' = 'none';

          if (t.status === 'spec_generation') {
            phase = 'planning';
            planningStatus = 'active';
          } else if (t.status === 'spec_review') {
            phase = 'review';
            planningStatus = 'pending_review';
          } else if (t.status === 'spec_approved') {
            phase = 'implementation';
            planningStatus = 'completed';
          } else if (t.status === 'implementing') {
            phase = 'implementation';
            planningStatus = 'completed';
          } else if (t.status === 'completed') {
            phase = 'completed';
            planningStatus = 'completed';
          }

          return {
            ...t,
            hasSpecs: t.status !== 'backlog',
            phase,
            planningStatus,
            activeSessionsCount: 0,
            completedSessionsCount: 0,
          };
        });

        setTasks(enhancedTasks);

        // Check if the task has spec_session_id now
        const updatedTask = specTasks.find(t => t.id === task.id);

        // Check if task failed during async agent launch (error stored in metadata)
        if (updatedTask?.status === 'backlog' && updatedTask?.metadata?.error) {
          console.error('❌ Planning failed:', updatedTask.metadata.error);
          setError(updatedTask.metadata.error as string);
          return;
        }

        if (updatedTask?.spec_session_id) {
          console.log('✅ Session ID populated:', updatedTask.spec_session_id);
          return; // Session ID found, stop polling
        }

        // If session ID not found and we haven't exhausted retries, poll again
        if (retryCount < maxRetries) {
          const delay = retryCount < 3 ? 1000 : 2000; // 1s for first 3 retries, 2s after
          console.log(`⏳ Polling for session ID (attempt ${retryCount + 1}/${maxRetries}), waiting ${delay}ms...`);
          setTimeout(() => pollForSessionId(retryCount + 1, maxRetries), delay);
        } else {
          console.warn('⚠️ Session ID not populated after max retries');
        }
      };

      // Start aggressive polling
      await pollForSessionId();
    } catch (err: any) {
      console.error('Failed to start planning:', err);
      // Extract error message from API response
      const errorMessage = err?.response?.data?.error
        || err?.response?.data?.message
        || err?.message
        || 'Failed to start planning. Please try again.';
      setError(errorMessage);
    }
  };

  // Handle reviewing documents
  const handleReviewDocs = async (task: SpecTaskWithExtras) => {
    setReviewingTask(task);

    // Fetch the latest design review for this task
    try {
      const response = await api.get(`/api/v1/spec-tasks/${task.id}/design-reviews`);
      const reviews = response.data?.reviews || [];

      if (reviews.length > 0) {
        // Get the latest non-superseded review
        const latestReview = reviews.find((r: any) => r.status !== 'superseded') || reviews[0];
        setActiveReviewId(latestReview.id);
        setDesignReviewViewerOpen(true);
      } else {
        // No design review exists yet, show old doc viewer
        setDocViewerOpen(true);
      }
    } catch (error) {
      console.error('Failed to fetch design reviews:', error);
      // Fallback to old doc viewer
      setDocViewerOpen(true);
    }
  };

  // Handle approving specs
  const handleApproveSpecs = async (comment?: string) => {
    if (!reviewingTask) return;

    try {
      // Call approve specs API
      await api.getApiClient().v1SpecTasksApproveSpecsCreate(reviewingTask.id!, {
        approved: true,
        comments: comment || 'Specs approved',
      });

      // Refresh tasks
      const response = await api.getApiClient().v1SpecTasksList({
        project_id: projectId || 'default',
      });
      const tasksData = response.data || response;
      const specTasks: SpecTask[] = Array.isArray(tasksData) ? tasksData : [];

      const enhancedTasks: SpecTaskWithExtras[] = specTasks.map((t) => {
        let phase: SpecTaskPhase = 'backlog';
        let planningStatus: 'none' | 'active' | 'pending_review' | 'completed' | 'failed' = 'none';

        if (t.status === 'spec_generation') {
          phase = 'planning';
          planningStatus = 'active';
        } else if (t.status === 'spec_review') {
          phase = 'review';
          planningStatus = 'pending_review';
        } else if (t.status === 'spec_approved') {
          phase = 'implementation';
          planningStatus = 'completed';
        } else if (t.status === 'implementing') {
          phase = 'implementation';
          planningStatus = 'completed';
        } else if (t.status === 'completed') {
          phase = 'completed';
          planningStatus = 'completed';
        }

        return {
          ...t,
          hasSpecs: t.status !== 'backlog',
          phase,
          planningStatus,
          activeSessionsCount: 0,
          completedSessionsCount: 0,
          onReviewDocs: handleReviewDocs,
        };
      });

      setTasks(enhancedTasks);
      setDocViewerOpen(false);
      setReviewingTask(null);
    } catch (err) {
      console.error('Failed to approve specs:', err);
      setError('Failed to approve specs');
    }
  };

  // Handle rejecting specs (request changes)
  const handleRejectSpecs = async (comment: string) => {
    if (!reviewingTask) return;

    try {
      // Call approve specs API with approved = false
      await api.getApiClient().v1SpecTasksApproveSpecsCreate(reviewingTask.id!, {
        approved: false,
        comments: comment,
      });

      // Refresh tasks
      const response = await api.getApiClient().v1SpecTasksList({
        project_id: projectId || 'default',
      });
      const tasksData = response.data || response;
      const specTasks: SpecTask[] = Array.isArray(tasksData) ? tasksData : [];

      const enhancedTasks: SpecTaskWithExtras[] = specTasks.map((t) => {
        let phase: SpecTaskPhase = 'backlog';
        let planningStatus: 'none' | 'active' | 'pending_review' | 'completed' | 'failed' = 'none';

        if (t.status === 'spec_generation') {
          phase = 'planning';
          planningStatus = 'active';
        } else if (t.status === 'spec_review') {
          phase = 'review';
          planningStatus = 'pending_review';
        } else if (t.status === 'spec_approved') {
          phase = 'implementation';
          planningStatus = 'completed';
        } else if (t.status === 'implementing') {
          phase = 'implementation';
          planningStatus = 'completed';
        } else if (t.status === 'completed') {
          phase = 'completed';
          planningStatus = 'completed';
        }

        return {
          ...t,
          hasSpecs: t.status !== 'backlog',
          phase,
          planningStatus,
          activeSessionsCount: 0,
          completedSessionsCount: 0,
          onReviewDocs: handleReviewDocs,
        };
      });

      setTasks(enhancedTasks);
      setDocViewerOpen(false);
      setReviewingTask(null);
    } catch (err) {
      console.error('Failed to request changes:', err);
      setError('Failed to request changes');
    }
  };

  // Handle rejecting completely (archive)
  const handleRejectCompletely = async (comment: string) => {
    if (!reviewingTask) return;

    try {
      // Archive the task
      await api.getApiClient().v1SpecTasksArchivePartialUpdate(reviewingTask.id!, {
        archived: true,
      });

      // Refresh tasks
      const response = await api.getApiClient().v1SpecTasksList({
        project_id: projectId || 'default',
        archived_only: showArchived,
      });
      const tasksData = response.data || response;
      const specTasks: SpecTask[] = Array.isArray(tasksData) ? tasksData : [];

      const enhancedTasks: SpecTaskWithExtras[] = specTasks.map((t) => {
        let phase: SpecTaskPhase = 'backlog';
        let planningStatus: 'none' | 'active' | 'pending_review' | 'completed' | 'failed' = 'none';

        if (t.status === 'spec_generation') {
          phase = 'planning';
          planningStatus = 'active';
        } else if (t.status === 'spec_review') {
          phase = 'review';
          planningStatus = 'pending_review';
        } else if (t.status === 'spec_approved') {
          phase = 'implementation';
          planningStatus = 'completed';
        } else if (t.status === 'implementing') {
          phase = 'implementation';
          planningStatus = 'completed';
        } else if (t.status === 'completed') {
          phase = 'completed';
          planningStatus = 'completed';
        }

        return {
          ...t,
          hasSpecs: t.status !== 'backlog',
          phase,
          planningStatus,
          activeSessionsCount: 0,
          completedSessionsCount: 0,
          onReviewDocs: handleReviewDocs,
        };
      });

      setTasks(enhancedTasks);
      setDocViewerOpen(false);
      setReviewingTask(null);
    } catch (err) {
      console.error('Failed to reject completely:', err);
      setError('Failed to reject completely');
    }
  };

  // Render draggable task card
  const DraggableTaskCard = ({ task, index }: { task: SpecTaskWithExtras; index: number }) => {
    const specStatus = getSpecStatusInfo(task);
    const taskId = task.id || `task-${index}`;
    
    const {
      attributes,
      listeners,
      setNodeRef,
      transform,
      transition,
      isDragging,
    } = useSortable({ id: taskId });

    const style = {
      transform: CSS.Transform.toString(transform),
      transition,
    };
    
    return (
      <Card
        ref={setNodeRef}
        style={style}
        {...attributes}
        {...listeners}
        sx={{
          mb: 1,
          cursor: isDragging ? 'grabbing' : 'grab',
          borderLeft: `4px solid ${getPriorityColor(task.priority || 'medium')}`,
          opacity: isDragging ? 0.8 : 1,
          '&:hover': {
            boxShadow: theme.shadows[4],
          },
        }}
        onClick={() => onTaskClick?.(task)}
      >
            <CardContent sx={{ p: 2, '&:last-child': { pb: 2 } }}>
              {/* Task header */}
              <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', mb: 1 }}>
                <Typography variant="subtitle2" sx={{ fontWeight: 600, flex: 1 }}>
                  {task.name}
                </Typography>
                <IconButton size="small" onClick={(e) => e.stopPropagation()}>
                  <MoreIcon fontSize="small" />
                </IconButton>
              </Box>

              {/* Task description */}
              <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
                {(task.description || '').length > 100 
                  ? `${(task.description || '').substring(0, 100)}...` 
                  : task.description || 'No description'
                }
              </Typography>

              {/* Status chips */}
              <Box sx={{ display: 'flex', gap: 0.5, flexWrap: 'wrap', mb: 1 }}>
                {/* Spec status */}
                <Chip
                  icon={specStatus.icon}
                  label={specStatus.text}
                  size="small"
                  sx={{ 
                    backgroundColor: specStatus.color + '20',
                    color: specStatus.color,
                  }}
                />

                {/* Priority */}
                <Chip
                  label={task.priority || 'medium'}
                  size="small"
                  sx={{ 
                    backgroundColor: getPriorityColor(task.priority || 'medium') + '20',
                    color: getPriorityColor(task.priority || 'medium'),
                  }}
                />

                {/* Active sessions indicator */}
                {(task.activeSessionsCount || 0) > 0 && (
                  <Chip
                    icon={<CodeIcon />}
                    label={`${task.activeSessionsCount} active`}
                    size="small"
                    color="success"
                  />
                )}

                {/* Git repository indicator */}
                {task.gitRepositoryId && (
                  <Chip
                    icon={<GitIcon />}
                    label="Git repo"
                    size="small"
                    color="primary"
                  />
                )}
              </Box>

              {/* Progress indicator for implementation */}
              {(task as any).phase === 'implementation' && task.multiSessionOverview && (
                <Box sx={{ mt: 1 }}>
                  <Box sx={{ display: 'flex', justifyContent: 'space-between', mb: 0.5 }}>
                    <Typography variant="caption">Progress</Typography>
                    <Typography variant="caption">
                      {task.completedSessionsCount || 0}/{(task.activeSessionsCount || 0) + (task.completedSessionsCount || 0)}
                    </Typography>
                  </Box>
                  <LinearProgress
                    variant="determinate"
                    value={
                      (task.completedSessionsCount || 0) / 
                      ((task.activeSessionsCount || 0) + (task.completedSessionsCount || 0)) * 100
                    }
                    sx={{ height: 4, borderRadius: 2 }}
                  />
                </Box>
              )}

              {/* Last activity */}
              <Typography variant="caption" color="text.secondary" sx={{ mt: 1, display: 'block' }}>
                {task.lastActivity ? formatTimeAgo(new Date(task.lastActivity)) : 'No activity'}
              </Typography>
            </CardContent>
      </Card>
    );
  };



  if (loading) {
    return (
      <Box sx={{ display: 'flex', flexDirection: 'column', justifyContent: 'center', alignItems: 'center', height: 400, gap: 2 }}>
        <CircularProgress />
        <Typography variant="body2" color="text.secondary">
          Loading spec tasks...
        </Typography>
      </Box>
    );
  }

  if (error) {
    return (
      <Box sx={{ display: 'flex', flexDirection: 'column', alignItems: 'center', height: 400, gap: 2, p: 4 }}>
        <Alert severity="error" sx={{ width: '100%', maxWidth: 600 }}>
          {error}
        </Alert>
        <Button variant="outlined" onClick={() => window.location.reload()}>
          Retry
        </Button>
      </Box>
    );
  }

  if (!account.user?.id) {
    return (
      <Box sx={{ display: 'flex', flexDirection: 'column', alignItems: 'center', height: 400, gap: 2, p: 4 }}>
        <Typography variant="h6" color="text.secondary">
          Please log in to view spec tasks
        </Typography>
      </Box>
    );
  }



  return (
    <Box sx={{ flex: 1, display: 'flex', flexDirection: 'column', minHeight: 0 }}>
      {/* Header */}
      <Box sx={{ flexShrink: 0, display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 2 }}>
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 2 }}>
          <Typography variant="h5" sx={{ fontWeight: 600 }}>
            SpecTask Board
          </Typography>
          {onCreateTask && (
            <Button
              variant="contained"
              color="secondary"
              startIcon={<AddIcon />}
              onClick={onCreateTask}
              sx={{
                '& .MuiButton-endIcon': {
                  ml: 1,
                  opacity: 0.7,
                  fontSize: '0.75rem',
                },
              }}
              endIcon={
                <Box component="span" sx={{
                  fontSize: '0.75rem',
                  opacity: 0.7,
                  fontFamily: 'monospace',
                  ml: 1,
                  backgroundColor: 'rgba(0, 0, 0, 0.2)',
                  px: 0.75,
                  py: 0.25,
                  borderRadius: '4px',
                }}>
                  ↵
                </Box>
              }
            >
              New SpecTask
            </Button>
          )}
        </Box>
        <Box sx={{ display: 'flex', gap: 1 }}>
          <Button
            size="small"
            variant="outlined"
            startIcon={showArchived ? <ViewIcon /> : <VisibilityOffIcon />}
            onClick={() => setShowArchived(!showArchived)}
          >
            {showArchived ? 'Show Active' : 'Show Archived'}
          </Button>
        </Box>
      </Box>

      {error && <Alert severity="error" sx={{ flexShrink: 0, mb: 2 }} onClose={() => setError(null)}>{error}</Alert>}

      {/* Kanban Board - drag and drop disabled to prevent infinite loops */}
      <Box sx={{ flex: 1, display: 'flex', gap: 1, overflowX: 'auto', overflowY: 'hidden', minHeight: 0 }}>
        {columns.map((column) => (
          <DroppableColumn
            key={column.id}
            column={column}
            columns={columns}
            onStartPlanning={handleStartPlanning}
            onArchiveTask={handleArchiveTask}
            onTaskClick={onTaskClick}
            projectId={projectId}
            theme={theme}
          />
        ))}
      </Box>

      {/* Design Document Viewer */}
      <DesignDocViewer
        open={docViewerOpen}
        onClose={() => {
          setDocViewerOpen(false);
          setReviewingTask(null);
        }}
        taskId={reviewingTask?.id || ''}
        taskName={reviewingTask?.name || ''}
        sessionId={reviewingTask?.spec_session_id}
        onApprove={handleApproveSpecs}
        onReject={handleRejectSpecs}
        onRejectCompletely={handleRejectCompletely}
      />

      {/* Design Review Viewer - New beautiful review UI */}
      {designReviewViewerOpen && reviewingTask && activeReviewId && (
        <DesignReviewViewer
          open={designReviewViewerOpen}
          onClose={() => {
            setDesignReviewViewerOpen(false);
            setReviewingTask(null);
            setActiveReviewId(null);
            // Refresh tasks to update status
            setRefreshTrigger(prev => prev + 1);
          }}
          specTaskId={reviewingTask.id!}
          reviewId={activeReviewId}
        />
      )}

      {/* Planning Dialog */}
      <Dialog open={planningDialogOpen} onClose={() => setPlanningDialogOpen(false)} maxWidth="md" fullWidth>
        <DialogTitle>Start Planning Session</DialogTitle>
        <DialogContent>
          <Stack spacing={2} sx={{ mt: 1 }}>
            <Typography variant="body2" color="text.secondary">
              Start a Zed planning session for: <strong>{selectedTask?.name}</strong>
            </Typography>
            
            <TextField
              label="Requirements"
              fullWidth
              multiline
              rows={4}
              value={newTaskRequirements}
              onChange={(e) => setNewTaskRequirements(e.target.value)}
              helperText="Describe what you want the planning agent to implement"
            />
            
            <Button 
              variant="outlined" 
              onClick={() => console.log('🔥 TEST BUTTON CLICKED - EVENTS WORK!')}
              sx={{ mt: 2 }}
            >
              TEST BUTTON - Click Me
            </Button>
            
            <FormControl fullWidth>
              <InputLabel>Project Template</InputLabel>
              <Select
                value={selectedSampleType}
                onChange={(e) => {
                  console.log('🔥 CREATE DIALOG SELECT ONCHANGE FIRED!', e.target.value);
                  setSelectedSampleType(e.target.value as string);
                }}
                native
              >
                <option value="">Select a project template</option>
                {sampleTypes.map((type: SampleType, index) => {
                  const typeId = type.id || `sample-${index}`;
                  
                  return (
                    <option 
                      key={typeId} 
                      value={typeId}
                    >
                      {type.name}
                    </option>
                  );
                })}
              </Select>
            </FormControl>
          </Stack>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setPlanningDialogOpen(false)}>Cancel</Button>
          <Button 
            onClick={() => selectedTask && startPlanning(selectedTask, selectedSampleType)}
            variant="contained"
            disabled={!newTaskRequirements.trim() || !selectedSampleType}
          >
            Start Planning
          </Button>
        </DialogActions>
      </Dialog>

      {/* Archive Confirmation Dialog */}
      <Dialog open={archiveConfirmOpen} onClose={() => setArchiveConfirmOpen(false)} maxWidth="sm" fullWidth>
        <DialogTitle>Archive Task?</DialogTitle>
        <DialogContent>
          <Alert severity="warning" sx={{ mb: 2 }}>
            <Typography variant="body2">
              Archiving this task will:
            </Typography>
            <ul style={{ marginTop: 8, marginBottom: 0, paddingLeft: 20 }}>
              <li><Typography variant="body2">Stop any running external agents</Typography></li>
              <li><Typography variant="body2">Lose any unsaved data in the desktop</Typography></li>
              <li><Typography variant="body2">Hide the task from the board</Typography></li>
            </ul>
          </Alert>
          <Typography variant="body2" color="text.secondary">
            The conversation history will be preserved and you can restore the task later.
          </Typography>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => {
            setArchiveConfirmOpen(false);
            setTaskToArchive(null);
          }}>
            Cancel
          </Button>
          <Button
            onClick={async () => {
              if (taskToArchive) {
                setArchiveConfirmOpen(false);
                await performArchive(taskToArchive, true);
                setTaskToArchive(null);
              }
            }}
            variant="contained"
            color="warning"
          >
            Archive Task
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
};

// Helper function to replace date-fns formatDistanceToNow
function formatTimeAgo(date: Date): string {
  const now = new Date();
  const diffMs = now.getTime() - date.getTime();
  const diffMins = Math.floor(diffMs / 60000);
  const diffHours = Math.floor(diffMs / 3600000);
  const diffDays = Math.floor(diffMs / 86400000);

  if (diffMins < 1) return 'just now';
  if (diffMins < 60) return `${diffMins} minute${diffMins === 1 ? '' : 's'} ago`;
  if (diffHours < 24) return `${diffHours} hour${diffHours === 1 ? '' : 's'} ago`;
  if (diffDays < 30) return `${diffDays} day${diffDays === 1 ? '' : 's'} ago`;
  
  return date.toLocaleDateString();
}

export default SpecTaskKanbanBoard;