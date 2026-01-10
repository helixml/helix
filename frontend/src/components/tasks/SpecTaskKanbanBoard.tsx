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
  Circle as CircleIcon,
  MenuBook as DesignDocsIcon,
  Stop as StopIcon,
  RocketLaunch as LaunchIcon,
} from '@mui/icons-material';
// Removed drag-and-drop imports to prevent infinite loops
import { useTheme } from '@mui/material/styles';
// Removed date-fns dependency - using native JavaScript instead

import useApi from '../../hooks/useApi';
import useAccount from '../../hooks/useAccount';
import useRouter from '../../hooks/useRouter';
import { getBrowserLocale } from '../../hooks/useBrowserLocale';
import DesignReviewViewer from '../spec-tasks/DesignReviewViewer';
import ArchiveConfirmDialog from './ArchiveConfirmDialog';
import TaskCard from './TaskCard';
import {
  SpecTask,
  MultiSessionOverview,
} from '../../services/specTaskService';
import {
  useCreateSampleRepository,
} from '../../services/gitRepositoryService';
import { useSampleTypes } from '../../hooks/useSampleTypes';

// SpecTask types and statuses
type SpecTaskPhase = 'backlog' | 'planning' | 'review' | 'implementation' | 'pull_request' | 'completed';
type SpecTaskPriority = 'low' | 'medium' | 'high' | 'critical';

// Helper function to map backend status to frontend phase
// IMPORTANT: This must be used consistently everywhere to prevent tasks from disappearing
function mapStatusToPhase(status: string): { phase: SpecTaskPhase; planningStatus: 'none' | 'active' | 'pending_review' | 'completed' | 'failed' | 'queued'; hasSpecs: boolean } {
  let phase: SpecTaskPhase = 'backlog';
  let planningStatus: 'none' | 'active' | 'pending_review' | 'completed' | 'failed' | 'queued' = 'none';
  let hasSpecs = status !== 'backlog';

  // Queued states - show in backlog but with queued status
  if (status === 'queued_spec_generation' || status === 'queued_implementation') {
    phase = 'backlog';
    planningStatus = 'queued';
    hasSpecs = false;
  }
  // Spec generation phase
  else if (status === 'spec_generation') {
    phase = 'planning';
    planningStatus = 'active';
  }
  // Spec review/revision phase
  else if (status === 'spec_review' || status === 'spec_revision') {
    phase = 'review';
    planningStatus = 'pending_review';
  }
  // Implementation phase (all implementation-related statuses)
  else if (
    status === 'spec_approved' ||
    status === 'implementation_queued' ||
    status === 'implementation' ||
    status === 'implementing' ||
    status === 'implementation_review' ||
    status === 'implementation_failed'
  ) {
    phase = 'implementation';
    planningStatus = 'completed';
  }
  // Pull Request phase (external repos - awaiting merge)
  else if (status === 'pull_request') {
    phase = 'pull_request';
    planningStatus = 'completed';
  }
  // Completed/Merged phase
  else if (status === 'done' || status === 'completed') {
    phase = 'completed';
    planningStatus = 'completed';
  }
  // Failed spec generation - show in backlog with error state
  else if (status === 'spec_failed') {
    phase = 'backlog';
    planningStatus = 'failed';
    hasSpecs = false;
  }
  // Default: backlog (for 'backlog' status and any unknown status)
  // hasSpecs is already set based on status !== 'backlog'

  return { phase, planningStatus, hasSpecs };
}

interface SpecTaskWithExtras extends SpecTask {
  hasSpecs: boolean;
  phase: SpecTaskPhase;
  planningStatus?: 'none' | 'active' | 'pending_review' | 'completed' | 'failed' | 'queued';
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
  focusTaskId?: string; // Task ID to focus "Start Planning" button on (for newly created tasks)
  hasExternalRepo?: boolean; // When true, project uses external repo (ADO) - Accept button becomes "Open PR"
  // repositories prop removed - repos are now managed at project level
}

const DroppableColumn: React.FC<{
  column: KanbanColumn;
  columns: KanbanColumn[];
  onStartPlanning?: (task: SpecTaskWithExtras) => Promise<void>;
  onArchiveTask?: (task: SpecTaskWithExtras, archived: boolean) => Promise<void>;
  onTaskClick?: (task: SpecTaskWithExtras) => void;
  onReviewDocs?: (task: SpecTaskWithExtras) => void;
  projectId?: string;
  focusTaskId?: string;
  archivingTaskId?: string | null;
  hasExternalRepo?: boolean;
  theme: any;
}> = ({ column, columns, onStartPlanning, onArchiveTask, onTaskClick, onReviewDocs, projectId, focusTaskId, archivingTaskId, hasExternalRepo, theme }): JSX.Element => {
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
        onReviewDocs={onReviewDocs}
        projectId={projectId}
        focusStartPlanning={task.id === focusTaskId}
        isArchiving={task.id === archivingTaskId}
        hasExternalRepo={hasExternalRepo}
      />
    );
  };

    // Column color mapping
    const getColumnAccent = (id: string) => {
      switch (id) {
        case 'backlog': return { color: '#6b7280', bg: 'transparent' };
        case 'planning': return { color: '#f59e0b', bg: 'rgba(245, 158, 11, 0.08)' };
        case 'review': return { color: '#3b82f6', bg: 'rgba(59, 130, 246, 0.08)' };
        case 'implementation': return { color: '#10b981', bg: 'rgba(16, 185, 129, 0.08)' };
        case 'pull_request': return { color: '#8b5cf6', bg: 'rgba(139, 92, 246, 0.08)' }; // Purple for PR
        case 'completed': return { color: '#6b7280', bg: 'transparent' };
        default: return { color: '#6b7280', bg: 'transparent' };
      }
    };

    const accent = getColumnAccent(column.id);

    return (
      <Box key={column.id} sx={{ width: 300, flexShrink: 0, height: '100%' }}>
        <Box sx={{
          height: '100%',
          display: 'flex',
          flexDirection: 'column',
          border: '1px solid',
          borderColor: 'rgba(0, 0, 0, 0.1)',
          borderRadius: 2,
          backgroundColor: 'background.paper',
          boxShadow: '0 1px 3px 0 rgba(0, 0, 0, 0.04), inset 0 1px 0 0 rgba(255, 255, 255, 0.5)',
        }}>
          {/* Column header - Linear style */}
          <Box sx={{
            px: 2.5,
            py: 2,
            borderBottom: '1px solid',
            borderColor: 'rgba(0, 0, 0, 0.06)',
            flexShrink: 0,
          }}>
            <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
              <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                <Typography variant="body2" sx={{ fontWeight: 600, color: 'text.primary', fontSize: '0.8125rem' }}>
                  {column.title}
                </Typography>
                <Box sx={{
                  display: 'inline-flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  minWidth: '20px',
                  height: '20px',
                  px: 0.75,
                  borderRadius: '10px',
                  backgroundColor: accent.bg || 'rgba(0, 0, 0, 0.04)',
                  border: '1px solid',
                  borderColor: accent.bg ? `${accent.color}20` : 'rgba(0, 0, 0, 0.06)',
                }}>
                  <Typography variant="caption" sx={{
                    fontSize: '0.7rem',
                    fontWeight: 600,
                    color: accent.color || 'text.secondary',
                    lineHeight: 1,
                  }}>
                    {column.tasks.length}
                  </Typography>
                </Box>
                {column.limit && column.tasks.length >= column.limit && (
                  <Box sx={{
                    px: 0.75,
                    py: 0.25,
                    borderRadius: '4px',
                    backgroundColor: 'rgba(239, 68, 68, 0.1)',
                    border: '1px solid rgba(239, 68, 68, 0.2)',
                  }}>
                    <Typography variant="caption" sx={{ fontSize: '0.65rem', fontWeight: 600, color: '#ef4444' }}>
                      FULL
                    </Typography>
                  </Box>
                )}
              </Box>
            </Box>
          </Box>

          {/* Column content */}
          <Box
            ref={setNodeRef}
            sx={{
              flex: 1,
              minHeight: 0,
              overflowY: 'auto',
              overflowX: 'hidden',
              px: 2,
              pt: 2,
              pb: 1,
              '&::-webkit-scrollbar': {
                width: '6px',
              },
              '&::-webkit-scrollbar-track': {
                background: 'transparent',
              },
              '&::-webkit-scrollbar-thumb': {
                background: 'rgba(0, 0, 0, 0.1)',
                borderRadius: '3px',
                '&:hover': {
                  background: 'rgba(0, 0, 0, 0.15)',
                },
              },
            }}
          >
            {column.tasks.map((task, index) => renderTaskCard(task, index))}
            {column.tasks.length === 0 && (
              <Box sx={{
                textAlign: 'center',
                py: 6,
                color: 'text.disabled',
              }}>
                <Typography variant="caption" sx={{ fontSize: '0.75rem', fontWeight: 500 }}>
                  No tasks
                </Typography>
              </Box>
            )}
          </Box>
        </Box>
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
  focusTaskId,
  hasExternalRepo = false,
}) => {
  const theme = useTheme();
  const api = useApi();
  const account = useAccount(); 

  // Track initial load to avoid showing loading spinner on refreshes
  const hasLoadedOnceRef = React.useRef(false);

  // State
  const [tasks, setTasks] = useState<SpecTaskWithExtras[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [planningDialogOpen, setPlanningDialogOpen] = useState(false);
  const [selectedTask, setSelectedTask] = useState<SpecTaskWithExtras | null>(null);
  const [showArchived, setShowArchived] = useState(false);
  const [archiveConfirmOpen, setArchiveConfirmOpen] = useState(false);
  const [taskToArchive, setTaskToArchive] = useState<SpecTaskWithExtras | null>(null);
  const [archivingTaskId, setArchivingTaskId] = useState<string | null>(null);

  // Design review viewer state
  const [reviewingTask, setReviewingTask] = useState<SpecTaskWithExtras | null>(null);
  const [designReviewViewerOpen, setDesignReviewViewerOpen] = useState(false);
  const [activeReviewId, setActiveReviewId] = useState<string | null>(null);
  const [designReviewInitialTab, setDesignReviewInitialTab] = useState<'requirements' | 'technical_design' | 'implementation_plan'>('requirements');

  // Planning form state
  const [newTaskRequirements, setNewTaskRequirements] = useState('');
  const [selectedSampleType, setSelectedSampleType] = useState('');

  // Available sample types for planning
  const [sampleTypes, setSampleTypes] = useState<any[]>([]);  

  // Keyboard shortcut for creating new task (Enter key)
  useEffect(() => {
    const handleKeyPress = (e: KeyboardEvent) => {
      // Only trigger if not in an interactive element
      const target = e.target as HTMLElement

      // Skip if in form elements
      if (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.tagName === 'SELECT') {
        return
      }

      // Skip if in iframe (video stream)
      if (target.tagName === 'IFRAME') {
        return
      }

      // Skip if element is contentEditable
      if (target.isContentEditable) {
        return
      }

      // Skip if inside an element with role that expects keyboard input
      const role = target.getAttribute('role')
      if (role === 'textbox' || role === 'searchbox' || role === 'combobox') {
        return
      }

      // Skip if inside MoonlightStreamViewer or any video container
      if (target.closest('[data-video-container]') || target.closest('.moonlight-stream-viewer')) {
        return
      }

      // Skip if inside prompt input area
      if (target.closest('[data-prompt-input]') || target.closest('.prompt-input-container')) {
        return
      }

      if (e.key === 'Enter') {
        if (onCreateTask) {
          onCreateTask()
        }
      }
    }

    window.addEventListener('keydown', handleKeyPress)
    return () => window.removeEventListener('keydown', handleKeyPress)
  }, [onCreateTask])

  // WIP limits for kanban columns (use prop values or defaults)
  const WIP_LIMITS = {
    backlog: undefined,
    planning: wipLimits.planning,
    review: wipLimits.review,
    implementation: wipLimits.implementation,
    completed: undefined,
  };

  // Kanban columns configuration - Linear color scheme
  // Pull Request column only shown for external repos (ADO)
  const columns: KanbanColumn[] = useMemo(() => {
    const baseColumns: KanbanColumn[] = [
      {
        id: 'backlog',
        title: 'Backlog',
        color: '#6b7280',
        backgroundColor: 'transparent',
        description: 'Tasks without specifications',
        tasks: tasks.filter(t => (t as any).phase === 'backlog' && !t.hasSpecs),
      },
      {
        id: 'planning',
        title: 'Planning',
        color: '#f59e0b',
        backgroundColor: 'rgba(245, 158, 11, 0.08)',
        description: 'Specs being generated',
        limit: WIP_LIMITS.planning,
        tasks: tasks.filter(t => (t as any).phase === 'planning' || t.planningStatus === 'active'),
      },
      {
        id: 'review',
        title: 'Spec Review',
        color: '#3b82f6',
        backgroundColor: 'rgba(59, 130, 246, 0.08)',
        description: 'Ready for review',
        limit: WIP_LIMITS.review,
        tasks: tasks.filter(t => (t as any).phase === 'review' || t.specApprovalNeeded),
      },
      {
        id: 'implementation',
        title: 'In Progress',
        color: '#10b981',
        backgroundColor: 'rgba(16, 185, 129, 0.08)',
        description: 'Implementation active',
        limit: WIP_LIMITS.implementation,
        tasks: tasks.filter(t => (t as any).phase === 'implementation'),
      },
    ];

    // Only show Pull Request column for external repos (ADO)
    if (hasExternalRepo) {
      baseColumns.push({
        id: 'pull_request',
        title: 'Pull Request',
        color: '#8b5cf6',
        backgroundColor: 'rgba(139, 92, 246, 0.08)',
        description: 'Awaiting merge in external repo',
        tasks: tasks.filter(t => (t as any).phase === 'pull_request' || t.status === 'pull_request'),
      });
    }

    baseColumns.push({
      id: 'completed',
      title: 'Merged',
      color: '#6b7280',
      backgroundColor: 'transparent',
      description: 'Merged to main',
      tasks: tasks.filter(t => (t as any).phase === 'completed' || t.status === 'done'),
    });

    return baseColumns;
  }, [tasks, theme, wipLimits, hasExternalRepo]);

  // Load sample types using generated client
  const { data: sampleTypesData, loading: sampleTypesLoading } = useSampleTypes();

  // Initial load
  useEffect(() => {
    if (!account.user?.id) return;

    const loadTasks = async () => {
      try {
        // Only show loading spinner on initial load
        // This prevents the board from disappearing during refreshes
        if (!hasLoadedOnceRef.current) {
          setLoading(true);
        }
        const response = await api.get('/api/v1/spec-tasks', {
          params: {
            project_id: projectId || 'default',
            archived_only: showArchived
          }
        });

        const tasksData = response.data || response;
        const specTasks: SpecTask[] = Array.isArray(tasksData) ? tasksData : [];

        // Use consistent phase mapping helper
        const enhancedTasks: SpecTaskWithExtras[] = specTasks.map((task) => {
          const { phase, planningStatus: mappedStatus, hasSpecs } = mapStatusToPhase(task.status || 'backlog');

          // Check for errors in metadata (tasks stay in backlog with error)
          const planningStatus = (task.status === 'backlog' && task.metadata?.error) ? 'failed' : mappedStatus;

          return {
            ...task,
            hasSpecs,
            phase,
            planningStatus,
            activeSessionsCount: 0,
            completedSessionsCount: 0,
            onReviewDocs: handleReviewDocs,
          };
        });

        setTasks(enhancedTasks);
        hasLoadedOnceRef.current = true;
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

        // Use consistent phase mapping helper
        const enhancedTasks: SpecTaskWithExtras[] = specTasks.map((task) => {
          const { phase, planningStatus: mappedStatus, hasSpecs } = mapStatusToPhase(task.status || 'backlog');
          const planningStatus = (task.status === 'backlog' && task.metadata?.error) ? 'failed' : mappedStatus;

          return {
            ...task,
            hasSpecs,
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
    if (sampleTypesData && sampleTypesData.length > 0) {      
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
          
          // Use consistent phase mapping helper
          const enhancedTasks: SpecTaskWithExtras[] = specTasks.map((task) => {
            const { phase, planningStatus, hasSpecs } = mapStatusToPhase(task.status || 'backlog');

            return {
              ...task,
              hasSpecs,
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
    setArchivingTaskId(task.id!);
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

      // Use consistent phase mapping helper
      const enhancedTasks: SpecTaskWithExtras[] = specTasks.map((t) => {
        const { phase, planningStatus, hasSpecs } = mapStatusToPhase(t.status || 'backlog');

        return {
          ...t,
          hasSpecs,
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
    } finally {
      setArchivingTaskId(null);
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
      // Include keyboard layout from browser locale detection (or ?keyboard= override)
      const { keyboardLayout, timezone, isOverridden } = getBrowserLocale();
      const queryParams = new URLSearchParams();
      if (keyboardLayout) queryParams.set('keyboard', keyboardLayout);
      if (timezone) queryParams.set('timezone', timezone);
      const queryString = queryParams.toString();
      const url = `/api/v1/spec-tasks/${task.id}/start-planning${queryString ? `?${queryString}` : ''}`;

      // Log keyboard layout being sent to API
      console.log(`%c[Start Planning] Keyboard: ${keyboardLayout}${isOverridden ? ' (from URL override)' : ' (from browser)'}`, 'color: #2196F3; font-weight: bold;');
      console.log(`[Start Planning] API URL: ${url}`);

      const response = await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'include',
      });
      if (!response.ok) {
        const errorData = await response.json().catch(() => ({}));
        throw new Error(errorData.error || errorData.message || `Failed to start planning: ${response.statusText}`);
      }

      // Aggressive polling after starting planning to catch planning_session_id update
      // Poll at 1s, 2s, 4s, 6s intervals to catch the async session creation
      const pollForSessionId = async (retryCount = 0, maxRetries = 6) => {
        const response = await api.getApiClient().v1SpecTasksList({
          project_id: projectId || 'default'
        });

        const tasksData = response.data || response;
        const specTasks: SpecTask[] = Array.isArray(tasksData) ? tasksData : [];

        // Use consistent phase mapping helper
        const enhancedTasks: SpecTaskWithExtras[] = specTasks.map((t) => {
          const { phase, planningStatus, hasSpecs } = mapStatusToPhase(t.status || 'backlog');

          return {
            ...t,
            hasSpecs,
            phase,
            planningStatus,
            activeSessionsCount: 0,
            completedSessionsCount: 0,
          };
        });

        setTasks(enhancedTasks);

        // Check if the task has planning_session_id now
        const updatedTask = specTasks.find(t => t.id === task.id);

        // Check if task failed during async agent launch (error stored in metadata)
        if (updatedTask?.status === 'backlog' && updatedTask?.metadata?.error) {
          console.error('❌ Planning failed:', updatedTask.metadata.error);
          setError(updatedTask.metadata.error as string);
          return;
        }

        if (updatedTask?.planning_session_id) {
          console.log('✅ Planning session ID populated:', updatedTask.planning_session_id);
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

  // Handle reviewing documents - optionally open to a specific tab
  // Default to 'requirements' since that's the natural starting point for review
  const handleReviewDocs = async (task: SpecTaskWithExtras, initialTab: 'requirements' | 'technical_design' | 'implementation_plan' = 'requirements') => {
    setReviewingTask(task);
    setDesignReviewInitialTab(initialTab);

    // Fetch the latest design review for this task using generated client
    try {
      const response = await api.getApiClient().v1SpecTasksDesignReviewsDetail(task.id);
      console.log('Design reviews response:', response);
      const reviews = response.data?.reviews || [];
      console.log('Reviews array:', reviews);

      if (reviews.length > 0) {
        // Get the latest non-superseded review
        const latestReview = reviews.find((r: any) => r.status !== 'superseded') || reviews[0];
        console.log('Opening review:', latestReview.id, 'for task:', task.id);
        setActiveReviewId(latestReview.id);
        setDesignReviewViewerOpen(true);
      } else {
        // This shouldn't happen since we auto-create reviews on push
        console.error('No design review found for task:', task.id);
        setError('No design review found. Please try starting planning again.');
      }
    } catch (error) {
      console.error('Failed to fetch design reviews:', error);
      setError('Failed to load design review');
    }
  };

  if (loading) {
    return (
      <Box sx={{ display: 'flex', flexDirection: 'column', justifyContent: 'center', alignItems: 'center', height: 400, gap: 2 }}>
        <CircularProgress size={32} sx={{ color: 'text.secondary' }} />
        <Typography variant="body2" color="text.secondary" sx={{ fontSize: '0.8125rem' }}>
          Loading tasks...
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
      {/* Header - Linear style */}
      <Box sx={{ flexShrink: 0, display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 3, pb: 2, borderBottom: '1px solid', borderColor: 'rgba(0, 0, 0, 0.06)' }}>
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 2 }}>
          <Typography variant="h5" sx={{ fontWeight: 600, fontSize: '1.25rem', color: 'text.primary' }}>
            Project Workspace
          </Typography>
          {onCreateTask && (
            <Tooltip title="Press Enter">
              <Button
                variant="contained"
                color="secondary"
                startIcon={<AddIcon />}
                onClick={onCreateTask}
              >
                New Task
              </Button>
            </Tooltip>
          )}
        </Box>
        <Box sx={{ display: 'flex', gap: 1 }}>
          <Button
            size="small"
            variant="outlined"
            startIcon={showArchived ? <ViewIcon /> : <VisibilityOffIcon />}
            onClick={() => setShowArchived(!showArchived)}
          >
            {showArchived ? 'Active' : 'Archived'}
          </Button>
        </Box>
      </Box>

      {error && (
        <Alert
          severity="error"
          sx={{
            flexShrink: 0,
            mb: 2,
            border: '1px solid rgba(239, 68, 68, 0.2)',
            backgroundColor: 'rgba(239, 68, 68, 0.08)',
          }}
          onClose={() => setError(null)}
        >
          {error}
        </Alert>
      )}

      {/* Kanban Board */}
      <Box sx={{
        flex: 1,
        display: 'flex',
        gap: 2,
        overflowX: 'auto',
        overflowY: 'hidden',
        minHeight: 0,
        pb: 2,
        '&::-webkit-scrollbar': {
          height: '8px',
        },
        '&::-webkit-scrollbar-track': {
          background: 'rgba(0, 0, 0, 0.02)',
          borderRadius: '4px',
        },
        '&::-webkit-scrollbar-thumb': {
          background: 'rgba(0, 0, 0, 0.1)',
          borderRadius: '4px',
          '&:hover': {
            background: 'rgba(0, 0, 0, 0.15)',
          },
        },
      }}>
        {columns.map((column) => (
          <DroppableColumn
            key={column.id}
            column={column}
            columns={columns}
            onStartPlanning={handleStartPlanning}
            onArchiveTask={handleArchiveTask}
            onTaskClick={onTaskClick}
            onReviewDocs={handleReviewDocs}
            projectId={projectId}
            focusTaskId={focusTaskId}
            archivingTaskId={archivingTaskId}
            hasExternalRepo={hasExternalRepo}
            theme={theme}
          />
        ))}
      </Box>

      {/* Design Review Viewer - Floating window for spec review */}
      {designReviewViewerOpen && reviewingTask && activeReviewId && (
        <DesignReviewViewer
          open={designReviewViewerOpen}
          onClose={() => {
            setDesignReviewViewerOpen(false);
            setReviewingTask(null);
            setActiveReviewId(null);
            // Refresh tasks to update status
            if (onRefresh) onRefresh();
          }}
          specTaskId={reviewingTask.id!}
          reviewId={activeReviewId}
          initialTab={designReviewInitialTab}
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
      <ArchiveConfirmDialog
        open={archiveConfirmOpen}
        onClose={() => {
          setArchiveConfirmOpen(false);
          setTaskToArchive(null);
        }}
        onConfirm={() => {
          if (taskToArchive) {
            setArchiveConfirmOpen(false);
            const task = taskToArchive;
            setTaskToArchive(null);
            performArchive(task, true);
          }
        }}
        taskName={taskToArchive?.name}
        isArchiving={!!archivingTaskId}
      />
    </Box>
  );
};

export default SpecTaskKanbanBoard;