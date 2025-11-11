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
import { useApproveImplementation, useStopAgent } from '../../services/specTaskWorkflowService';

// Minimal wrapper for desktop viewer with custom overlay for Kanban cards
// Uses screenshot mode (not live stream) to avoid performance issues with many cards
const LiveAgentScreenshot: React.FC<{
  sessionId: string;
  projectId?: string;
}> = React.memo(({ sessionId, projectId }) => {
  return (
    <Box
      sx={{
        mt: 1.5,
        mb: 0.5,
        position: 'relative',
        borderRadius: 1.5,
        overflow: 'hidden',
        border: '1px solid',
        borderColor: 'rgba(0, 0, 0, 0.08)',
        minHeight: 80,
      }}
    >
      <Box sx={{ position: 'relative', height: 180 }}>
        <ExternalAgentDesktopViewer
          sessionId={sessionId}
          height={180}
          mode="screenshot"
        />
      </Box>
      <Box
        sx={{
          position: 'absolute',
          bottom: 0,
          left: 0,
          right: 0,
          background: 'linear-gradient(to top, rgba(0,0,0,0.7), transparent)',
          color: 'white',
          p: 0.5,
          px: 1.5,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          pointerEvents: 'none',
        }}
      >
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
          <CircleIcon sx={{ fontSize: 8, color: '#4ade80' }} />
          <Typography variant="caption" sx={{ fontWeight: 500, fontSize: '0.7rem' }}>
            Agent Running
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
  onReviewDocs?: (task: SpecTaskWithExtras) => void;
  projectId?: string;
}> = ({ task, index, columns, onStartPlanning, onArchiveTask, onTaskClick, onReviewDocs, projectId }) => {
  const account = useAccount();
  const approveImplementationMutation = useApproveImplementation(task.id!);
  const stopAgentMutation = useStopAgent(task.id!);

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

  // Get phase-based accent color for cards
  const getPhaseAccent = (phase: string) => {
    switch (phase) {
      case 'planning': return '#f59e0b';
      case 'review': return '#3b82f6';
      case 'implementation': return '#10b981';
      case 'completed': return '#6b7280';
      default: return '#e5e7eb';
    }
  };

  const accentColor = getPhaseAccent(task.phase);

  return (
    <Card
      onClick={() => onTaskClick && onTaskClick(task)}
      sx={{
        mb: 1.5,
        backgroundColor: 'background.paper',
        cursor: 'pointer',
        border: '1px solid',
        borderColor: 'rgba(0, 0, 0, 0.08)',
        borderLeft: `3px solid ${accentColor}`,
        boxShadow: 'none',
        transition: 'all 0.15s ease-in-out',
        '&:hover': {
          borderColor: 'rgba(0, 0, 0, 0.12)',
          backgroundColor: 'rgba(0, 0, 0, 0.01)',
        },
      }}
    >
      <CardContent sx={{ p: 2, '&:last-child': { pb: 2 } }}>
        {/* Task name */}
        <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', mb: 1 }}>
          <Typography variant="body2" sx={{ fontWeight: 500, flex: 1, lineHeight: 1.4, color: 'text.primary' }}>
            {task.name}
          </Typography>
          <Box sx={{ display: 'flex', gap: 0.5 }}>
            {/* Design doc icon - visible when task has specs (not in backlog) */}
            {task.phase !== 'backlog' && (
              <Tooltip title="View Design Document">
                <IconButton
                  size="small"
                  onClick={(e) => {
                    e.stopPropagation();
                    if (onReviewDocs) {
                      onReviewDocs(task);
                    }
                  }}
                  sx={{
                    width: 24,
                    height: 24,
                    color: 'primary.main',
                    '&:hover': {
                      color: 'primary.dark',
                      backgroundColor: 'rgba(33, 150, 243, 0.08)',
                    },
                  }}
                >
                  <DesignDocsIcon sx={{ fontSize: 16 }} />
                </IconButton>
              </Tooltip>
            )}
            <Tooltip title={task.archived ? "Restore" : "Archive"}>
              <IconButton
                size="small"
                onClick={(e) => {
                  e.stopPropagation();
                  if (onArchiveTask) {
                    onArchiveTask(task, !task.archived);
                  }
                }}
                sx={{
                  width: 24,
                  height: 24,
                  color: 'text.secondary',
                  '&:hover': {
                    color: 'text.primary',
                    backgroundColor: 'rgba(0, 0, 0, 0.04)',
                  },
                }}
              >
                {task.archived ? <RestoreIcon sx={{ fontSize: 16 }} /> : <CloseIcon sx={{ fontSize: 16 }} />}
              </IconButton>
            </Tooltip>
          </Box>
        </Box>

        {/* Status row - minimal dots and text */}
        <Box sx={{ display: 'flex', gap: 1.5, alignItems: 'center', mb: 1.5 }}>
          {/* Phase status dot */}
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
            <CircleIcon
              sx={{
                fontSize: 8,
                color: task.phase === 'planning' ? '#f59e0b' :
                       task.phase === 'review' ? '#3b82f6' :
                       task.phase === 'implementation' ? '#10b981' :
                       task.phase === 'completed' ? '#6b7280' : '#9ca3af'
              }}
            />
            <Typography variant="caption" sx={{ fontSize: '0.7rem', color: 'text.secondary', fontWeight: 500 }}>
              {task.phase === 'backlog' ? 'Backlog' :
               task.phase === 'planning' ? 'Planning' :
               task.phase === 'review' ? 'Review' :
               task.phase === 'implementation' ? 'In Progress' : 'Done'}
            </Typography>
          </Box>

          {/* Active sessions count */}
          {((task.activeSessionsCount ?? 0) > 0 || (task.completedSessionsCount ?? 0) > 0) && (
            <>
              <Typography variant="caption" sx={{ color: 'text.disabled' }}>•</Typography>
              <Typography variant="caption" sx={{ fontSize: '0.7rem', color: 'text.secondary', fontWeight: 500 }}>
                {(task.activeSessionsCount ?? 0) > 0 && `${task.activeSessionsCount} active`}
                {(task.activeSessionsCount ?? 0) > 0 && (task.completedSessionsCount ?? 0) > 0 && ', '}
                {(task.completedSessionsCount ?? 0) > 0 && `${task.completedSessionsCount} done`}
              </Typography>
            </>
          )}
        </Box>

        {/* Live screenshot widget for active planning sessions */}
        {task.spec_session_id && (
          <LiveAgentScreenshot
            sessionId={task.spec_session_id}
            projectId={projectId}
          />
        )}

        {/* Show "Start Planning" button only for backlog tasks */}
        {task.phase === 'backlog' && (
          <Box sx={{ mt: 1.5 }}>
            {/* Show error if present */}
            {task.metadata?.error && (
              <Box sx={{
                mb: 1,
                px: 1.5,
                py: 1,
                backgroundColor: 'rgba(239, 68, 68, 0.08)',
                borderRadius: 1,
                border: '1px solid rgba(239, 68, 68, 0.2)'
              }}>
                <Typography variant="caption" sx={{ fontWeight: 500, color: '#ef4444', fontSize: '0.7rem' }}>
                  ⚠ {task.metadata.error as string}
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
              <Typography variant="caption" sx={{ mt: 0.75, display: 'block', textAlign: 'center', color: '#ef4444', fontSize: '0.7rem' }}>
                Planning column at capacity ({planningColumn?.limit})
              </Typography>
            )}
          </Box>
        )}

        {/* Show "Review Design" button for review phase tasks */}
        {task.phase === 'review' && onReviewDocs && (
          <Box sx={{ mt: 1.5 }}>
            <Button
              size="small"
              variant="contained"
              color="info"
              startIcon={<SpecIcon />}
              onClick={(e) => {
                e.stopPropagation();
                onReviewDocs(task);
              }}
              fullWidth
            >
              Review Design
            </Button>
          </Box>
        )}

        {/* Implementation phase buttons */}
        {task.status === 'implementation' && (
          <Box sx={{ mt: 1.5, display: 'flex', flexDirection: 'column', gap: 1 }}>
            <Button
              size="small"
              variant="outlined"
              startIcon={<ViewIcon />}
              onClick={(e) => {
                e.stopPropagation();
                if (onTaskClick) onTaskClick(task);
              }}
              fullWidth
            >
              View Agent Session
            </Button>
            <Button
              size="small"
              variant="outlined"
              color="error"
              startIcon={<StopIcon />}
              onClick={(e) => {
                e.stopPropagation();
                stopAgentMutation.mutate();
              }}
              disabled={stopAgentMutation.isPending}
              fullWidth
            >
              Stop Agent
            </Button>
          </Box>
        )}

        {/* Implementation review phase buttons */}
        {task.status === 'implementation_review' && (
          <Box sx={{ mt: 1.5, display: 'flex', flexDirection: 'column', gap: 1 }}>
            <Button
              size="small"
              variant="contained"
              color="primary"
              startIcon={<ViewIcon />}
              onClick={(e) => {
                e.stopPropagation();
                if (onTaskClick) onTaskClick(task);
              }}
              fullWidth
            >
              Review Implementation
            </Button>
            <Button
              size="small"
              variant="contained"
              color="success"
              startIcon={<ApproveIcon />}
              onClick={(e) => {
                e.stopPropagation();
                approveImplementationMutation.mutate();
              }}
              disabled={approveImplementationMutation.isPending}
              fullWidth
            >
              Approve Implementation
            </Button>
            <Button
              size="small"
              variant="outlined"
              color="error"
              startIcon={<StopIcon />}
              onClick={(e) => {
                e.stopPropagation();
                stopAgentMutation.mutate();
              }}
              disabled={stopAgentMutation.isPending}
              fullWidth
            >
              Stop Agent
            </Button>
          </Box>
        )}

        {/* Completed tasks - offer exploratory session */}
        {task.status === 'done' && task.merged_to_main && (
          <Box sx={{ mt: 1.5 }}>
            <Alert severity="success" sx={{ py: 0.5 }}>
              <Typography variant="caption" sx={{ fontSize: '0.7rem', display: 'block', mb: 0.5 }}>
                Merged to main! Test the feature:
              </Typography>
              <Button
                size="small"
                variant="outlined"
                color="success"
                startIcon={<LaunchIcon />}
                onClick={(e) => {
                  e.stopPropagation();
                  // TODO: Start exploratory session on main branch
                  console.log('Start exploratory session for', task.id);
                }}
                fullWidth
              >
                Start Exploratory Session
              </Button>
            </Alert>
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
  onReviewDocs?: (task: SpecTaskWithExtras) => void;
  projectId?: string;
  theme: any;
}> = ({ column, columns, onStartPlanning, onArchiveTask, onTaskClick, onReviewDocs, projectId, theme }): JSX.Element => {
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
        onReviewDocs={onReviewDocs}
        projectId={projectId}
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

  // Kanban columns configuration - Linear color scheme
  const columns: KanbanColumn[] = useMemo(() => [
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
      title: 'Review',
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
      tasks: tasks.filter(t => (t as any).phase === 'implementation' && (t.activeSessionsCount || 0) > 0),
    },
    {
      id: 'completed',
      title: 'Done',
      color: '#6b7280',
      backgroundColor: 'transparent',
      description: 'Completed',
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

    // Fetch the latest design review for this task using generated client
    try {
      const response = await api.getApiClient().v1SpecTasksDesignReviewsDetail(task.id);
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
            Tasks
          </Typography>
          {onCreateTask && (
            <Button
              variant="contained"
              color="secondary"
              startIcon={<AddIcon />}
              onClick={onCreateTask}
            >
              New Task
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
        <DialogTitle sx={{ fontWeight: 600, fontSize: '1.125rem' }}>Archive Task?</DialogTitle>
        <DialogContent>
          <Alert severity="warning" sx={{ mb: 2, border: '1px solid rgba(245, 158, 11, 0.2)' }}>
            <Typography variant="body2" sx={{ fontWeight: 500, mb: 1 }}>
              Archiving this task will:
            </Typography>
            <ul style={{ marginTop: 0, marginBottom: 0, paddingLeft: 20 }}>
              <li><Typography variant="body2">Stop any running external agents</Typography></li>
              <li><Typography variant="body2">Lose any unsaved data in the desktop</Typography></li>
              <li><Typography variant="body2">Hide the task from the board</Typography></li>
            </ul>
          </Alert>
          <Typography variant="body2" color="text.secondary">
            The conversation history will be preserved and you can restore the task later.
          </Typography>
        </DialogContent>
        <DialogActions sx={{ px: 3, pb: 2.5 }}>
          <Button
            onClick={() => {
              setArchiveConfirmOpen(false);
              setTaskToArchive(null);
            }}
            sx={{
              textTransform: 'none',
              fontWeight: 500,
              color: 'text.secondary',
            }}
          >
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