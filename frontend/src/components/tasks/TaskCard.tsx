import React, { useState, useMemo, useRef, useEffect } from 'react'
import {
  Card,
  CardContent,
  Box,
  Typography,
  Button,
  IconButton,
  Tooltip,
  Alert,
  CircularProgress,
  LinearProgress,
  keyframes,
  Dialog,
  DialogTitle,
  DialogContent,
} from '@mui/material'
import {
  PlayArrow as PlayIcon,
  Description as SpecIcon,
  Visibility as ViewIcon,
  CheckCircle as ApproveIcon,
  Stop as StopIcon,
  RocketLaunch as LaunchIcon,
  Close as CloseIcon,
  Restore as RestoreIcon,
  MenuBook as DesignDocsIcon,
  Circle as CircleIcon,
  CheckCircle as CheckCircleIcon,
  RadioButtonUnchecked as UncheckedIcon,
  ContentCopy as CopyIcon,
  AccountTree as BatchIcon,
} from '@mui/icons-material'
import { useApproveImplementation, useStopAgent } from '../../services/specTaskWorkflowService'
import { useTaskProgress } from '../../services/specTaskService'
import ExternalAgentDesktopViewer from '../external-agent/ExternalAgentDesktopViewer'
import CloneTaskDialog from '../specTask/CloneTaskDialog'
import CloneGroupProgressFull from '../specTask/CloneGroupProgress'

// Pulse animation for the active task spinner
const pulseRing = keyframes`
  0% {
    transform: scale(0.8);
    opacity: 1;
  }
  50% {
    transform: scale(1.2);
    opacity: 0.5;
  }
  100% {
    transform: scale(0.8);
    opacity: 1;
  }
`

const spin = keyframes`
  0% {
    transform: rotate(0deg);
  }
  100% {
    transform: rotate(360deg);
  }
`

type SpecTaskPhase = 'backlog' | 'planning' | 'review' | 'implementation' | 'pull_request' | 'completed'

interface SpecTaskWithExtras {
  id: string
  name: string
  status: string
  phase: SpecTaskPhase
  planning_session_id?: string
  archived?: boolean
  metadata?: { error?: string }
  merged_to_main?: boolean
  just_do_it_mode?: boolean
  started_at?: string
  design_docs_pushed_at?: string
  clone_group_id?: string
  cloned_from_id?: string
  pull_request_url?: string
}

interface KanbanColumn {
  id: SpecTaskPhase
  limit?: number
  tasks: SpecTaskWithExtras[]
}

interface TaskCardProps {
  task: SpecTaskWithExtras
  index: number
  columns: KanbanColumn[]
  onStartPlanning?: (task: SpecTaskWithExtras) => Promise<void>
  onArchiveTask?: (task: SpecTaskWithExtras, archived: boolean) => Promise<void>
  onTaskClick?: (task: SpecTaskWithExtras) => void
  onReviewDocs?: (task: SpecTaskWithExtras) => void
  projectId?: string
  focusStartPlanning?: boolean // When true, focus the Start Planning button
  isArchiving?: boolean // When true, show spinner on archive button (parent is archiving this task)
  hasExternalRepo?: boolean // When true, project uses external repo (ADO) - Accept button becomes "Open PR"
}

// Interface for checklist items from API
interface ChecklistItem {
  index: number
  description: string
  status: string
}

interface ChecklistProgress {
  tasks: ChecklistItem[]
  total_tasks: number
  completed_tasks: number
  in_progress_task?: ChecklistItem
  progress_pct: number
}

const formatDuration = (startedAt: string): string => {
  const start = new Date(startedAt).getTime()
  const now = Date.now()
  const diffMs = now - start
  
  if (diffMs < 0) return '0s'
  
  const totalSeconds = Math.floor(diffMs / 1000)
  const hours = Math.floor(totalSeconds / 3600)
  const minutes = Math.floor((totalSeconds % 3600) / 60)
  const seconds = totalSeconds % 60
  
  if (hours > 0) {
    return `${hours}h${minutes}m`
  }
  return `${minutes}m${seconds}s`
}

const useRunningDuration = (startedAt: string | undefined, enabled: boolean): string | null => {
  const [duration, setDuration] = useState<string | null>(null)
  
  useEffect(() => {
    if (!enabled || !startedAt) {
      setDuration(null)
      return
    }
    
    setDuration(formatDuration(startedAt))
    
    const interval = setInterval(() => {
      setDuration(formatDuration(startedAt))
    }, 1000)
    
    return () => clearInterval(interval)
  }, [startedAt, enabled])
  
  return duration
}

// Gorgeous task progress display with fade effect and spinner
const TaskProgressDisplay: React.FC<{
  checklist: ChecklistProgress
  phaseColor: string
}> = React.memo(({ checklist, phaseColor }) => {
  // Find the in-progress task index
  const inProgressIndex = checklist.in_progress_task?.index ?? -1

  // Get visible tasks: 1 before, in-progress, 2 after (or adjust based on available)
  const visibleTasks = useMemo(() => {
    if (!checklist.tasks || checklist.tasks.length === 0) return []

    const tasks = checklist.tasks
    const activeIdx = inProgressIndex >= 0 ? inProgressIndex : tasks.findIndex(t => t.status === 'pending')

    if (activeIdx < 0) {
      // All completed - show last 3
      return tasks.slice(-3).map((t, i) => ({ ...t, fadeLevel: i === 2 ? 0 : 1 }))
    }

    // Show 1 before, active, 2 after with fade levels
    const start = Math.max(0, activeIdx - 1)
    const end = Math.min(tasks.length, activeIdx + 3)

    return tasks.slice(start, end).map((t, i, arr) => {
      const relativePos = t.index - activeIdx
      let fadeLevel = 0
      if (relativePos < 0) fadeLevel = 2 // Before: more faded
      else if (relativePos > 1) fadeLevel = 2 // Far after: more faded
      else if (relativePos === 1) fadeLevel = 1 // Just after: slightly faded
      return { ...t, fadeLevel }
    })
  }, [checklist.tasks, inProgressIndex])

  if (visibleTasks.length === 0) return null

  return (
    <Box
      sx={{
        mt: 1.5,
        mb: 0.5,
        background: 'linear-gradient(145deg, rgba(255,255,255,0.03) 0%, rgba(255,255,255,0.01) 100%)',
        borderRadius: 2,
        border: '1px solid rgba(255,255,255,0.06)',
        overflow: 'hidden',
      }}
    >
      {/* Progress bar header */}
      <Box
        sx={{
          px: 1.5,
          py: 0.75,
          background: 'linear-gradient(90deg, rgba(0,0,0,0.15) 0%, rgba(0,0,0,0.05) 100%)',
          borderBottom: '1px solid rgba(255,255,255,0.04)',
          display: 'flex',
          alignItems: 'center',
          gap: 1,
        }}
      >
        <LinearProgress
          variant="determinate"
          value={checklist.progress_pct}
          sx={{
            flex: 1,
            height: 4,
            borderRadius: 2,
            backgroundColor: 'rgba(255,255,255,0.08)',
            '& .MuiLinearProgress-bar': {
              background: `linear-gradient(90deg, ${phaseColor}99 0%, ${phaseColor} 100%)`,
              borderRadius: 2,
            },
          }}
        />
        <Typography
          variant="caption"
          sx={{
            fontSize: '0.65rem',
            color: 'rgba(255,255,255,0.5)',
            fontWeight: 600,
            letterSpacing: '0.02em',
            minWidth: 32,
            textAlign: 'right',
          }}
        >
          {checklist.completed_tasks}/{checklist.total_tasks}
        </Typography>
      </Box>

      {/* Task list with fade effect */}
      <Box sx={{ py: 0.5 }}>
        {visibleTasks.map((task, idx) => {
          const isActive = task.status === 'in_progress'
          const isCompleted = task.status === 'done' || task.status === 'completed'
          const opacity = task.fadeLevel === 2 ? 0.35 : task.fadeLevel === 1 ? 0.6 : 1

          return (
            <Box
              key={task.index}
              sx={{
                px: 1.5,
                py: 0.5,
                display: 'flex',
                alignItems: 'flex-start',
                gap: 0.75,
                opacity,
                transition: 'opacity 0.3s ease',
              }}
            >
              {/* Status indicator */}
              <Box
                sx={{
                  width: 16,
                  height: 16,
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  flexShrink: 0,
                  mt: 0.1,
                }}
              >
                {isActive ? (
                  // Animated spinner for active task
                  <Box sx={{ position: 'relative', width: 14, height: 14 }}>
                    <Box
                      sx={{
                        position: 'absolute',
                        inset: 0,
                        borderRadius: '50%',
                        border: `2px solid ${phaseColor}`,
                        borderTopColor: 'transparent',
                        animation: `${spin} 1s linear infinite`,
                      }}
                    />
                    <Box
                      sx={{
                        position: 'absolute',
                        inset: 2,
                        borderRadius: '50%',
                        backgroundColor: phaseColor,
                        animation: `${pulseRing} 2s ease-in-out infinite`,
                      }}
                    />
                  </Box>
                ) : isCompleted ? (
                  <CheckCircleIcon sx={{ fontSize: 14, color: '#10b981' }} />
                ) : (
                  <UncheckedIcon sx={{ fontSize: 14, color: 'rgba(255,255,255,0.25)' }} />
                )}
              </Box>

              {/* Task description */}
              <Typography
                variant="caption"
                sx={{
                  fontSize: '0.68rem',
                  lineHeight: 1.35,
                  color: isActive ? 'rgba(255,255,255,0.9)' : 'rgba(255,255,255,0.6)',
                  fontWeight: isActive ? 500 : 400,
                  overflow: 'hidden',
                  textOverflow: 'ellipsis',
                  display: '-webkit-box',
                  WebkitLineClamp: 2,
                  WebkitBoxOrient: 'vertical',
                  textDecoration: isCompleted ? 'line-through' : 'none',
                  textDecorationColor: 'rgba(255,255,255,0.3)',
                }}
              >
                {task.description}
              </Typography>
            </Box>
          )
        })}
      </Box>
    </Box>
  )
})

const LiveAgentScreenshot: React.FC<{
  sessionId: string
  projectId?: string
  onClick?: () => void
}> = React.memo(
  ({ sessionId, projectId, onClick }) => {
    return (
      <Box
        onClick={(e) => {
          e.stopPropagation() // Prevent card click
          if (onClick) onClick()
        }}
        sx={{
          mt: 1.5,
          mb: 0.5,
          mx: -1, // Extend slightly beyond content using negative margins
          width: 'calc(100% + 16px)', // Compensate for negative margins
          position: 'relative',
          borderRadius: 1.5,
          overflow: 'hidden',
          border: '1px solid',
          borderColor: 'rgba(0, 0, 0, 0.08)',
          minHeight: 80,
          cursor: 'pointer',
          transition: 'all 0.15s ease',
          '&:hover': {
            borderColor: 'primary.main',
            boxShadow: '0 0 0 1px rgba(33, 150, 243, 0.3)',
          },
        }}
      >
        <Box sx={{ position: 'relative', height: 136 }}>
          <ExternalAgentDesktopViewer sessionId={sessionId} height={136} mode="screenshot" />
        </Box>
        <Box
          sx={{
            position: 'absolute',
            bottom: 0,
            left: 0,
            right: 0,
            background: 'linear-gradient(to top, rgba(0,0,0,0.6) 0%, transparent 100%)',
            color: 'white',
            py: 0.75,
            px: 1.5,
            display: 'flex',
            alignItems: 'flex-end',
            justifyContent: 'flex-end',
            pointerEvents: 'none',
          }}
        >
          <Typography variant="caption" sx={{ fontWeight: 500, fontSize: '0.65rem', opacity: 0.8 }}>
            Click to view
          </Typography>
        </Box>
      </Box>
    )
  }
)

export default function TaskCard({
  task,
  index,
  columns,
  onStartPlanning,
  onArchiveTask,
  onTaskClick,
  onReviewDocs,
  projectId,
  focusStartPlanning = false,
  isArchiving = false,
  hasExternalRepo = false,
}: TaskCardProps) {
  const [isStartingPlanning, setIsStartingPlanning] = useState(false)
  const [showCloneDialog, setShowCloneDialog] = useState(false)
  const [showCloneBatchProgress, setShowCloneBatchProgress] = useState(false)
  const approveImplementationMutation = useApproveImplementation(task.id!)
  const stopAgentMutation = useStopAgent(task.id!)

  // Ref for Start Planning button to enable keyboard focus
  const startPlanningButtonRef = useRef<HTMLButtonElement>(null)

  // Focus the Start Planning button when requested
  useEffect(() => {
    if (focusStartPlanning && task.phase === 'backlog' && startPlanningButtonRef.current) {
      // Small delay to ensure DOM is ready after render
      const timer = setTimeout(() => {
        startPlanningButtonRef.current?.focus()
      }, 100)
      return () => clearTimeout(timer)
    }
  }, [focusStartPlanning, task.phase])

  // Fetch checklist progress for active tasks (planning/implementation)
  const showProgress = task.phase === 'planning' || task.phase === 'implementation'
  const { data: progressData } = useTaskProgress(task.id, {
    enabled: showProgress,
    refetchInterval: 5000, // Refresh every 5 seconds for live updates
  })

  const runningDuration = useRunningDuration(
    task.started_at,
    task.status === 'implementation'
  )

  // Check if planning column is full
  const planningColumn = columns.find((col) => col.id === 'planning')
  const isPlanningFull =
    planningColumn && planningColumn.limit ? planningColumn.tasks.length >= planningColumn.limit : false

  const handleStartPlanning = async (e: React.MouseEvent) => {
    e.stopPropagation()
    if (onStartPlanning) {
      setIsStartingPlanning(true)
      try {
        await onStartPlanning(task)
      } finally {
        setIsStartingPlanning(false)
      }
    }
  }

  // Get phase-based accent color for cards
  const getPhaseAccent = (phase: string) => {
    switch (phase) {
      case 'planning':
        return '#f59e0b'
      case 'review':
        return '#3b82f6'
      case 'implementation':
        return '#10b981'
      case 'pull_request':
        return '#8b5cf6' // Purple for PR
      case 'completed':
        return '#6b7280'
      default:
        return '#e5e7eb'
    }
  }

  const accentColor = getPhaseAccent(task.phase)

  // Handle card click - always open task detail view (session viewer)
  const handleCardClick = () => {
    if (onTaskClick) {
      onTaskClick(task)
    }
  }

  return (
    <Card
      onClick={handleCardClick}
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
            {/* Design doc icon - only visible when design docs have actually been pushed */}
            {task.design_docs_pushed_at && (
              <Tooltip title="Review Spec">
                <IconButton
                  size="small"
                  onClick={(e) => {
                    e.stopPropagation()
                    if (onReviewDocs) {
                      onReviewDocs(task)
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
            {/* Clone batch progress - visible for cloned tasks */}
            {task.clone_group_id && (
              <Tooltip title="View Clone Batch Progress">
                <IconButton
                  size="small"
                  onClick={(e) => {
                    e.stopPropagation()
                    setShowCloneBatchProgress(true)
                  }}
                  sx={{
                    width: 24,
                    height: 24,
                    color: 'secondary.main',
                    '&:hover': {
                      color: 'secondary.dark',
                      backgroundColor: 'rgba(156, 39, 176, 0.08)',
                    },
                  }}
                >
                  <BatchIcon sx={{ fontSize: 16 }} />
                </IconButton>
              </Tooltip>
            )}
            {/* Clone button - always visible */}
            <Tooltip title="Clone to Other Projects">
              <IconButton
                size="small"
                onClick={(e) => {
                  e.stopPropagation()
                  setShowCloneDialog(true)
                }}
                sx={{
                  width: 24,
                  height: 24,
                  color: 'text.secondary',
                  '&:hover': {
                    color: 'primary.main',
                    backgroundColor: 'rgba(33, 150, 243, 0.08)',
                  },
                }}
              >
                <CopyIcon sx={{ fontSize: 16 }} />
              </IconButton>
            </Tooltip>
            <Tooltip title={task.archived ? 'Restore' : 'Archive'}>
              <IconButton
                size="small"
                disabled={isArchiving}
                onClick={(e) => {
                  e.stopPropagation()
                  if (onArchiveTask) {
                    // Parent handles the async operation and manages isArchiving state
                    onArchiveTask(task, !task.archived)
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
                {isArchiving ? (
                  <CircularProgress size={14} sx={{ color: 'text.secondary' }} />
                ) : task.archived ? (
                  <RestoreIcon sx={{ fontSize: 16 }} />
                ) : (
                  <CloseIcon sx={{ fontSize: 16 }} />
                )}
              </IconButton>
            </Tooltip>
          </Box>
        </Box>

        {/* Status row */}
        <Box sx={{ display: 'flex', gap: 1.5, alignItems: 'center', mb: 1.5 }}>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
            <CircleIcon
              sx={{
                fontSize: 8,
                color:
                  task.phase === 'planning'
                    ? '#f59e0b'
                    : task.phase === 'review'
                    ? '#3b82f6'
                    : task.phase === 'implementation'
                    ? '#10b981'
                    : task.phase === 'pull_request'
                    ? '#8b5cf6'
                    : task.phase === 'completed'
                    ? '#6b7280'
                    : '#9ca3af',
              }}
            />
            <Typography variant="caption" sx={{ fontSize: '0.7rem', color: 'text.secondary', fontWeight: 500 }}>
              {task.phase === 'backlog'
                ? 'Backlog'
                : task.phase === 'planning'
                ? 'Planning'
                : task.phase === 'review'
                ? 'Review'
                : task.phase === 'implementation'
                ? 'In Progress'
                : task.phase === 'pull_request'
                ? 'Pull Request'
                : 'Merged'}
            </Typography>
            {runningDuration && (
              <Typography variant="caption" sx={{ fontSize: '0.7rem', color: 'text.secondary' }}>
                • {runningDuration}
              </Typography>
            )}
          </Box>
        </Box>

        {/* Gorgeous checklist progress for active tasks */}
        {progressData?.checklist && progressData.checklist.total_tasks > 0 && (
          <TaskProgressDisplay
            checklist={progressData.checklist as ChecklistProgress}
            phaseColor={accentColor}
          />
        )}

        {/* Live screenshot for active sessions - click opens desktop viewer */}
        {task.planning_session_id && (
          <LiveAgentScreenshot
            sessionId={task.planning_session_id}
            projectId={projectId}
            onClick={() => onTaskClick?.(task)}
          />
        )}

        {/* Backlog phase */}
        {task.phase === 'backlog' && (
          <Box sx={{ mt: 1.5 }}>
            {task.metadata?.error && (
              <Box
                sx={{
                  mb: 1,
                  px: 1.5,
                  py: 1,
                  backgroundColor: 'rgba(239, 68, 68, 0.08)',
                  borderRadius: 1,
                  border: '1px solid rgba(239, 68, 68, 0.2)',
                }}
              >
                <Typography variant="caption" sx={{ fontWeight: 500, color: '#ef4444', fontSize: '0.7rem' }}>
                  ⚠ {task.metadata.error as string}
                </Typography>
              </Box>
            )}
            <Button
              ref={startPlanningButtonRef}
              size="small"
              variant="contained"
              color="warning"
              startIcon={isStartingPlanning ? <CircularProgress size={16} color="inherit" /> : <PlayIcon />}
              onClick={handleStartPlanning}
              disabled={isPlanningFull || isStartingPlanning}
              fullWidth
            >
              {isStartingPlanning
                ? 'Starting...'
                : task.metadata?.error
                ? (task.just_do_it_mode ? 'Retry' : 'Retry Planning')
                : isPlanningFull
                ? 'Planning Full'
                : task.just_do_it_mode
                ? 'Just Do It'
                : 'Start Planning'}
            </Button>
            {isPlanningFull && (
              <Typography
                variant="caption"
                sx={{ mt: 0.75, display: 'block', textAlign: 'center', color: '#ef4444', fontSize: '0.7rem' }}
              >
                Planning column at capacity ({planningColumn?.limit})
              </Typography>
            )}
          </Box>
        )}

        {/* Review phase - only show button if design docs have been pushed */}
        {task.phase === 'review' && task.design_docs_pushed_at && onReviewDocs && (
          <Box sx={{ mt: 1.5 }}>
            <Button
              size="small"
              variant="contained"
              color="info"
              startIcon={<SpecIcon />}
              onClick={(e) => {
                e.stopPropagation()
                onReviewDocs(task)
              }}
              fullWidth
            >
              Review Spec
            </Button>
          </Box>
        )}

        {/* Implementation phase */}
        {task.status === 'implementation' && (
          <Box sx={{ mt: 1.5, display: 'flex', flexDirection: 'column', gap: 1 }}>
            <Box sx={{ display: 'flex', gap: 1 }}>              
              <Button
                size="small"
                variant="outlined"
                color="error"
                disabled={isArchiving}
                startIcon={isArchiving ? <CircularProgress size={14} color="inherit" /> : <CloseIcon />}
                onClick={(e) => {
                  e.stopPropagation()
                  if (onArchiveTask) {
                    // Parent handles the async operation and manages isArchiving state
                    onArchiveTask(task, true)
                  }
                }}
                sx={{ flex: 1 }}
              >
                {isArchiving ? 'Rejecting...' : 'Reject'}
              </Button>

              <Button
                size="small"
                variant="contained"
                color="success"
                startIcon={approveImplementationMutation.isPending ? <CircularProgress size={14} color="inherit" /> : <ApproveIcon />}
                onClick={(e) => {
                  e.stopPropagation()
                  approveImplementationMutation.mutate()
                }}
                disabled={approveImplementationMutation.isPending}
                sx={{ flex: 1 }}
              >
                {approveImplementationMutation.isPending
                  ? (hasExternalRepo ? 'Opening PR...' : 'Accepting...')
                  : (hasExternalRepo ? 'Open PR' : 'Accept')}
              </Button>
            </Box>
          </Box>
        )}

        {/* Implementation review phase */}
        {task.status === 'implementation_review' && (
          <Box sx={{ mt: 1.5, display: 'flex', flexDirection: 'column', gap: 1 }}>
            <Button
              size="small"
              variant="contained"
              color="primary"
              startIcon={<ViewIcon />}
              onClick={(e) => {
                e.stopPropagation()
                if (onTaskClick) onTaskClick(task)
              }}
              fullWidth
            >
              Review Implementation
            </Button>
            <Button
              size="small"
              variant="contained"
              color="success"
              startIcon={approveImplementationMutation.isPending ? <CircularProgress size={14} color="inherit" /> : <ApproveIcon />}
              onClick={(e) => {
                e.stopPropagation()
                approveImplementationMutation.mutate()
              }}
              disabled={approveImplementationMutation.isPending}
              fullWidth
            >
              {approveImplementationMutation.isPending ? 'Approving...' : 'Approve Implementation'}
            </Button>
            <Button
              size="small"
              variant="outlined"
              color="error"
              startIcon={stopAgentMutation.isPending ? <CircularProgress size={14} color="inherit" /> : <StopIcon />}
              onClick={(e) => {
                e.stopPropagation()
                stopAgentMutation.mutate()
              }}
              disabled={stopAgentMutation.isPending}
              fullWidth
            >
              {stopAgentMutation.isPending ? 'Stopping...' : 'Stop Agent'}
            </Button>
          </Box>
        )}

        {/* Pull Request phase - awaiting merge in external repo */}
        {task.phase === 'pull_request' && (
          <Box sx={{ mt: 1.5 }}>
            {task.pull_request_url && (
              <Button
                size="small"
                variant="contained"
                color="primary"
                startIcon={<LaunchIcon />}
                onClick={(e) => {
                  e.stopPropagation()
                  window.open(task.pull_request_url, '_blank')
                }}
                fullWidth
                sx={{ mb: 1 }}
              >
                View Pull Request
              </Button>
            )}
            <Typography
              variant="caption"
              sx={{
                fontSize: '0.7rem',
                color: 'text.secondary',
                fontStyle: 'italic',
                display: 'block',
                textAlign: 'center',
              }}
            >
              Address review comments with agent.<br />Moves to Merged when PR closes.
            </Typography>
          </Box>
        )}

        {/* Completed tasks */}
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
                  e.stopPropagation()
                  // TODO: Start exploratory session on main branch
                  console.log('Start exploratory session for', task.id)
                }}
                fullWidth
              >
                Start Exploratory Session
              </Button>
            </Alert>
          </Box>
        )}
      </CardContent>

      {/* Clone Task Dialog */}
      <CloneTaskDialog
        open={showCloneDialog}
        onClose={() => setShowCloneDialog(false)}
        taskId={task.id}
        taskName={task.name}
        sourceProjectId={projectId || ''}
      />

      {/* Clone Batch Progress Dialog */}
      {task.clone_group_id && (
        <Dialog
          open={showCloneBatchProgress}
          onClose={() => setShowCloneBatchProgress(false)}
          maxWidth="md"
          fullWidth
        >
          <DialogTitle sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
            Clone Batch Progress
            <IconButton size="small" onClick={() => setShowCloneBatchProgress(false)}>
              <CloseIcon />
            </IconButton>
          </DialogTitle>
          <DialogContent>
            <CloneGroupProgressFull
              groupId={task.clone_group_id}
              onTaskClick={(taskId, projectId) => {
                setShowCloneBatchProgress(false)
                if (onTaskClick) {
                  onTaskClick({ ...task, id: taskId })
                }
              }}
            />
          </DialogContent>
        </Dialog>
      )}
    </Card>
  )
}
