import React, { useState, useMemo } from 'react'
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
} from '@mui/icons-material'
import { useApproveImplementation, useStopAgent } from '../../services/specTaskWorkflowService'
import { useTaskProgress } from '../../services/specTaskService'
import ExternalAgentDesktopViewer from '../external-agent/ExternalAgentDesktopViewer'

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

type SpecTaskPhase = 'backlog' | 'planning' | 'review' | 'implementation' | 'completed'

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
        <Box sx={{ position: 'relative', height: 180 }}>
          <ExternalAgentDesktopViewer sessionId={sessionId} height={180} mode="screenshot" />
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
}: TaskCardProps) {
  const [isStartingPlanning, setIsStartingPlanning] = useState(false)
  const approveImplementationMutation = useApproveImplementation(task.id!)
  const stopAgentMutation = useStopAgent(task.id!)

  // Fetch checklist progress for active tasks (planning/implementation)
  const showProgress = task.phase === 'planning' || task.phase === 'implementation'
  const { data: progressData } = useTaskProgress(task.id, {
    enabled: showProgress,
    refetchInterval: 5000, // Refresh every 5 seconds for live updates
  })

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
      case 'completed':
        return '#6b7280'
      default:
        return '#e5e7eb'
    }
  }

  const accentColor = getPhaseAccent(task.phase)

  // Handle card click - open design docs if available, otherwise open session
  const handleCardClick = () => {
    if (task.design_docs_pushed_at && onReviewDocs) {
      // Design docs exist - open the spec review with tasks tab
      onReviewDocs(task)
    } else if (onTaskClick) {
      // No design docs - open the session viewer
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
            <Tooltip title={task.archived ? 'Restore' : 'Archive'}>
              <IconButton
                size="small"
                onClick={(e) => {
                  e.stopPropagation()
                  if (onArchiveTask) {
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
                {task.archived ? <RestoreIcon sx={{ fontSize: 16 }} /> : <CloseIcon sx={{ fontSize: 16 }} />}
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
                : 'Done'}
            </Typography>
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
            <Button
              size="small"
              variant="outlined"
              startIcon={<ViewIcon />}
              onClick={(e) => {
                e.stopPropagation()
                if (onTaskClick) onTaskClick(task)
              }}
              fullWidth
            >
              View Agent Session
            </Button>
            <Box sx={{ display: 'flex', gap: 1 }}>
              <Button
                size="small"
                variant="contained"
                color="success"
                startIcon={<ApproveIcon />}
                onClick={(e) => {
                  e.stopPropagation()
                  approveImplementationMutation.mutate()
                }}
                disabled={approveImplementationMutation.isPending}
                sx={{ flex: 1 }}
              >
                Accept
              </Button>
              <Button
                size="small"
                variant="outlined"
                color="error"
                startIcon={<CloseIcon />}
                onClick={(e) => {
                  e.stopPropagation()
                  if (onArchiveTask) {
                    onArchiveTask(task, true)
                  }
                }}
                sx={{ flex: 1 }}
              >
                Reject
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
              startIcon={<ApproveIcon />}
              onClick={(e) => {
                e.stopPropagation()
                approveImplementationMutation.mutate()
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
                e.stopPropagation()
                stopAgentMutation.mutate()
              }}
              disabled={stopAgentMutation.isPending}
              fullWidth
            >
              Stop Agent
            </Button>
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
    </Card>
  )
}
