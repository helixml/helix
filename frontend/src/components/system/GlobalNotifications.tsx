import React, { useState, useMemo } from 'react'
import Box from '@mui/material/Box'
import IconButton from '@mui/material/IconButton'
import Badge from '@mui/material/Badge'
import Popover from '@mui/material/Popover'
import Typography from '@mui/material/Typography'
import Chip from '@mui/material/Chip'
import { Bell } from 'lucide-react'

import { useListProjects } from '../../services/projectService'
import { useSpecTasks } from '../../services/specTaskService'
import useAccount from '../../hooks/useAccount'
import useLightTheme from '../../hooks/useLightTheme'
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

const REVIEW_STATUSES = [
  TypesSpecTaskStatus.TaskStatusSpecReview,
  TypesSpecTaskStatus.TaskStatusImplementationReview,
]

const GlobalNotifications: React.FC<GlobalNotificationsProps> = ({
  organizationId,
}) => {
  const account = useAccount()
  const lightTheme = useLightTheme()
  const [anchorEl, setAnchorEl] = useState<HTMLButtonElement | null>(null)

  const { data: projects = [] } = useListProjects(organizationId, {
    enabled: true,
  })

  const projectsWithReviews = useMemo(() => {
    return projects.filter(p => p.stats?.pending_review_tasks && p.stats.pending_review_tasks > 0)
  }, [projects])

  const { data: allTasks = [] } = useSpecTasks({
    enabled: projectsWithReviews.length > 0,
    refetchInterval: 30000,
  })

  const notifications = useMemo<TaskNotification[]>(() => {
    const projectMap = new Map(projects.map(p => [p.id, p.name || 'Unnamed Project']))
    
    return allTasks
      .filter((task: TypesSpecTask) => 
        task.status && REVIEW_STATUSES.includes(task.status) &&
        projectMap.has(task.project_id || '')
      )
      .map((task: TypesSpecTask) => ({
        taskId: task.id || '',
        projectId: task.project_id || '',
        projectName: projectMap.get(task.project_id || '') || 'Unnamed Project',
        shortTitle: task.user_short_title || task.short_title || 'Untitled Task',
        description: task.description || '',
      }))
  }, [allTasks, projects])

  const totalCount = notifications.length

  const handleClick = (event: React.MouseEvent<HTMLButtonElement>) => {
    setAnchorEl(event.currentTarget)
  }

  const handleClose = () => {
    setAnchorEl(null)
  }

  const handleTaskClick = (notification: TaskNotification) => {
    handleClose()
    account.orgNavigate('project-specs', { id: notification.taskId })
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
          color="error"
          sx={{
            '& .MuiBadge-badge': {
              fontSize: '0.65rem',
              height: 16,
              minWidth: 16,
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
