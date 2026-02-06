import React, { useState, useMemo, useEffect, useCallback } from 'react'
import Box from '@mui/material/Box'
import IconButton from '@mui/material/IconButton'
import Badge from '@mui/material/Badge'
import Popover from '@mui/material/Popover'
import Typography from '@mui/material/Typography'
import Chip from '@mui/material/Chip'
import { Bell } from 'lucide-react'
import { useQueries } from '@tanstack/react-query'

import { useListProjects } from '../../services/projectService'
import useAccount from '../../hooks/useAccount'
import useLightTheme from '../../hooks/useLightTheme'
import useApi from '../../hooks/useApi'
import { TypesSpecTask, TypesSpecTaskStatus } from '../../api/api'

interface GlobalNotificationsProps {
  organizationId?: string
}

interface TaskNotification {
  taskId: string
  projectId: string
  projectName: string
  shortTitle: string
  description: string
}

interface SeenNotificationsData {
  taskIds: string[]
  timestamp: number
}

const REVIEW_STATUSES = [
  TypesSpecTaskStatus.TaskStatusSpecReview,
  TypesSpecTaskStatus.TaskStatusImplementationReview,
  TypesSpecTaskStatus.TaskStatusPullRequest,
]

const STORAGE_KEY = 'helix_seen_notifications'
const EIGHT_HOURS_MS = 8 * 60 * 60 * 1000

const getSeenNotifications = (): SeenNotificationsData | null => {
  try {
    const stored = localStorage.getItem(STORAGE_KEY)
    if (!stored) return null
    const data: SeenNotificationsData = JSON.parse(stored)
    if (Date.now() - data.timestamp > EIGHT_HOURS_MS) {
      localStorage.removeItem(STORAGE_KEY)
      return null
    }
    return data
  } catch {
    return null
  }
}

const setSeenNotifications = (taskIds: string[]) => {
  const data: SeenNotificationsData = {
    taskIds,
    timestamp: Date.now(),
  }
  localStorage.setItem(STORAGE_KEY, JSON.stringify(data))
}

const GlobalNotifications: React.FC<GlobalNotificationsProps> = ({
  organizationId,
}) => {
  const account = useAccount()
  const lightTheme = useLightTheme()
  const api = useApi()
  const [anchorEl, setAnchorEl] = useState<HTMLButtonElement | null>(null)
  const [seenTaskIds, setSeenTaskIds] = useState<Set<string>>(() => {
    const stored = getSeenNotifications()
    return stored ? new Set(stored.taskIds) : new Set()
  })

  const { data: projects = [] } = useListProjects(organizationId, {
    enabled: true,
  })

  const projectsWithReviews = useMemo(() => {
    return projects.filter(p => p.stats?.pending_review_tasks && p.stats.pending_review_tasks > 0)
  }, [projects])

  const taskQueries = useQueries({
    queries: projectsWithReviews.map(project => ({
      queryKey: ['spec-tasks', 'notifications', project.id],
      queryFn: async () => {
        const response = await api.getApiClient().v1SpecTasksList({
          project_id: project.id,
        })
        return { projectId: project.id, tasks: response.data || [] }
      },
      enabled: !!project.id,
      refetchInterval: 30000,
    })),
  })

  const notifications = useMemo<TaskNotification[]>(() => {
    const projectMap = new Map(projects.map(p => [p.id, p.name || 'Unnamed Project']))
    
    const allTasks = taskQueries
      .filter(q => q.data)
      .flatMap(q => q.data!.tasks)
    
    return allTasks
      .filter((task: TypesSpecTask) => 
        task.status && REVIEW_STATUSES.includes(task.status) &&
        projectMap.has(task.project_id || '')
      )
      .map((task: TypesSpecTask) => ({
        taskId: task.id || '',
        projectId: task.project_id || '',
        projectName: projectMap.get(task.project_id || '') || 'Unnamed Project',
        shortTitle: task.name,
        description: task.description || '',
      }))
  }, [taskQueries, projects])

  const totalCount = notifications.length
  const currentTaskIds = useMemo(() => new Set(notifications.map(n => n.taskId)), [notifications])
  
  const hasNewNotifications = useMemo(() => {
    if (currentTaskIds.size === 0) return false
    for (const taskId of currentTaskIds) {
      if (!seenTaskIds.has(taskId)) return true
    }
    return false
  }, [currentTaskIds, seenTaskIds])

  const markAsSeen = useCallback(() => {
    if (currentTaskIds.size > 0) {
      const allSeen = new Set([...seenTaskIds, ...currentTaskIds])
      setSeenTaskIds(allSeen)
      setSeenNotifications(Array.from(allSeen))
    }
  }, [currentTaskIds, seenTaskIds])

  const handleClick = (event: React.MouseEvent<HTMLButtonElement>) => {
    setAnchorEl(event.currentTarget)
    markAsSeen()
  }

  const handleClose = () => {
    setAnchorEl(null)
  }

  const handleTaskClick = (notification: TaskNotification) => {
    handleClose()
    account.orgNavigate('project-task-detail', { id: notification.projectId, taskId: notification.taskId })
  }

  const open = Boolean(anchorEl)

  return (
    <>
      <IconButton
        onClick={handleClick}
        sx={{
          mr: 2,
          mb: 1,
          color: 'rgba(255,255,255,0.7)',
          '&:hover': {
            color: 'rgba(255,255,255,0.9)',
            backgroundColor: 'rgba(255,255,255,0.08)',
          },
        }}
      >
        <Badge
          badgeContent={totalCount}
          color={hasNewNotifications ? 'error' : 'default'}
          sx={{
            '& .MuiBadge-badge': {
              fontSize: '0.65rem',
              height: 16,
              minWidth: 16,
              ...(!hasNewNotifications && {
                backgroundColor: 'rgba(255,255,255,0.3)',
                color: 'rgba(0,0,0,0.7)',
              }),
            },
          }}
        >
          <Bell size={20} />
        </Badge>
      </IconButton>

      <Popover
        open={open}
        anchorEl={anchorEl}
        onClose={handleClose}
        anchorOrigin={{
          vertical: 'bottom',
          horizontal: 'right',
        }}
        transformOrigin={{
          vertical: 'top',
          horizontal: 'right',
        }}
        slotProps={{
          paper: {
            sx: {
              mt: 1,
              minWidth: 320,
              maxWidth: 400,
              maxHeight: 480,
              backgroundColor: lightTheme.backgroundColor,
              border: lightTheme.border,
              borderRadius: 1,
              overflow: 'hidden',
            },
          },
        }}
      >
        <Box sx={{ p: 2, borderBottom: lightTheme.border }}>
          <Typography
            variant="subtitle2"
            sx={{
              fontWeight: 600,
              color: lightTheme.textColor,
            }}
          >
            Notifications
          </Typography>
        </Box>

        <Box sx={{ overflowY: 'auto', maxHeight: 400 }}>
          {notifications.length === 0 ? (
            <Typography
              variant="body2"
              sx={{
                color: 'rgba(255,255,255,0.5)',
                py: 4,
                textAlign: 'center',
              }}
            >
              No pending reviews
            </Typography>
          ) : (
            <Box sx={{ display: 'flex', flexDirection: 'column' }}>
              {notifications.map((notification) => (
                <Box
                  key={notification.taskId}
                  onClick={() => handleTaskClick(notification)}
                  sx={{
                    display: 'flex',
                    alignItems: 'flex-start',
                    justifyContent: 'space-between',
                    gap: 1.5,
                    px: 2,
                    py: 1.5,
                    cursor: 'pointer',
                    borderBottom: '1px solid rgba(255,255,255,0.06)',
                    transition: 'background-color 0.15s ease',
                    '&:hover': {
                      backgroundColor: 'rgba(255,255,255,0.04)',
                    },
                    '&:last-child': {
                      borderBottom: 'none',
                    },
                  }}
                >
                  <Box sx={{ minWidth: 0, flex: 1 }}>
                    <Typography
                      variant="body2"
                      sx={{
                        fontWeight: 500,
                        color: lightTheme.textColor,
                        overflow: 'hidden',
                        textOverflow: 'ellipsis',
                        whiteSpace: 'nowrap',
                        mb: 0.25,
                      }}
                    >
                      {notification.shortTitle}
                    </Typography>
                    <Typography
                      variant="caption"
                      sx={{
                        color: 'rgba(255,255,255,0.5)',
                        display: 'block',
                        overflow: 'hidden',
                        textOverflow: 'ellipsis',
                        whiteSpace: 'nowrap',
                      }}
                    >
                      {notification.projectName}
                    </Typography>
                  </Box>
                  <Chip
                    label="Pending Review"
                    size="small"
                    sx={{
                      flexShrink: 0,
                      height: 22,
                      fontSize: '0.7rem',
                      fontWeight: 500,
                      backgroundColor: 'rgba(251, 146, 60, 0.15)',
                      color: 'rgb(251, 146, 60)',
                      border: '1px solid rgba(251, 146, 60, 0.3)',
                      '& .MuiChip-label': {
                        px: 1,
                      },
                    }}
                  />
                </Box>
              ))}
            </Box>
          )}
        </Box>
      </Popover>
    </>
  )
}

export default GlobalNotifications
