import React from 'react'
import {
  Card,
  CardContent,
  Box,
  Typography,
  Button,
  IconButton,
  Tooltip,
  Alert,
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
} from '@mui/icons-material'
import { useApproveImplementation, useStopAgent } from '../../services/specTaskWorkflowService'
import ExternalAgentDesktopViewer from '../external-agent/ExternalAgentDesktopViewer'

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

const LiveAgentScreenshot: React.FC<{ sessionId: string; projectId?: string }> = React.memo(
  ({ sessionId, projectId }) => {
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
          <ExternalAgentDesktopViewer sessionId={sessionId} height={180} mode="screenshot" />
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
  const approveImplementationMutation = useApproveImplementation(task.id!)
  const stopAgentMutation = useStopAgent(task.id!)

  // Check if planning column is full
  const planningColumn = columns.find((col) => col.id === 'planning')
  const isPlanningFull =
    planningColumn && planningColumn.limit ? planningColumn.tasks.length >= planningColumn.limit : false

  const handleStartPlanning = async (e: React.MouseEvent) => {
    e.stopPropagation()
    if (onStartPlanning) {
      await onStartPlanning(task)
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

        {/* Live screenshot for active sessions */}
        {task.planning_session_id && <LiveAgentScreenshot sessionId={task.planning_session_id} projectId={projectId} />}

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
              startIcon={<PlayIcon />}
              onClick={handleStartPlanning}
              disabled={isPlanningFull}
              fullWidth
            >
              {task.metadata?.error ? 'Retry Planning' : isPlanningFull ? 'Planning Full' : 'Start Planning'}
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
