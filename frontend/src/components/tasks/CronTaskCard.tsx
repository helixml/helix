import { FC, MouseEvent, useState } from 'react'
import Box from '@mui/material/Box'
import Card from '@mui/material/Card'
import CardContent from '@mui/material/CardContent'
import IconButton from '@mui/material/IconButton'
import Menu from '@mui/material/Menu'
import MenuItem from '@mui/material/MenuItem'
import Tooltip from '@mui/material/Tooltip'
import Typography from '@mui/material/Typography'
import useTheme from '@mui/material/styles/useTheme'
import DeleteIcon from '@mui/icons-material/Delete'
import EditIcon from '@mui/icons-material/Edit'
import MoreVertIcon from '@mui/icons-material/MoreVert'
import PauseCircleIcon from '@mui/icons-material/PauseCircleOutline'
import PauseIcon from '@mui/icons-material/Pause'
import PlayArrowIcon from '@mui/icons-material/PlayArrow'
import ScheduleIcon from '@mui/icons-material/Schedule'

import TasksTableExecutionsChart from './TasksTableExecutionsChart'
import useAccount from '../../hooks/useAccount'
import { TypesTriggerConfiguration } from '../../api/api'
import { generateCronShortSummary } from '../../utils/cronUtils'
import { IApp } from '../../types'

interface CronTaskCardProps {
  task: TypesTriggerConfiguration
  app?: IApp
  onEdit: (task: TypesTriggerConfiguration) => void
  onDelete: (task: TypesTriggerConfiguration) => void
  onToggleStatus: (task: TypesTriggerConfiguration) => void
}

const CronTaskCard: FC<CronTaskCardProps> = ({ task, app, onEdit, onDelete, onToggleStatus }) => {
  const theme = useTheme()
  const account = useAccount()
  const [anchorEl, setAnchorEl] = useState<null | HTMLElement>(null)

  const isEnabled = !!task.enabled && !task.archived
  const cronSchedule = task.trigger?.cron?.schedule || ''
  const scheduleDisplay = cronSchedule ? generateCronShortSummary(cronSchedule) : ''

  const handleMenuOpen = (e: MouseEvent<HTMLElement>) => {
    e.stopPropagation()
    setAnchorEl(e.currentTarget)
  }
  const handleMenuClose = () => setAnchorEl(null)

  return (
    <Card
      sx={{
        height: '100%',
        display: 'flex',
        flexDirection: 'column',
        backgroundColor: 'background.paper',
        border: '1px solid rgba(0, 0, 0, 0.08)',
        borderRadius: 1,
        boxShadow: 'none',
        transition: 'all 0.15s ease-in-out',
        '&:hover': {
          borderColor: 'rgba(0, 0, 0, 0.12)',
          backgroundColor: 'rgba(0, 0, 0, 0.01)',
        },
      }}
    >
      <CardContent
        sx={{
          flexGrow: 1,
          cursor: 'pointer',
          p: 2,
          '&:last-child': { pb: 2 },
          display: 'flex',
          flexDirection: 'column',
        }}
        onClick={() => onEdit(task)}
      >
        <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', mb: 1.5, gap: 1 }}>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, flex: 1, minWidth: 0 }}>
            <Tooltip title={isEnabled ? 'Enabled' : 'Disabled — schedule paused'}>
              {isEnabled ? (
                <ScheduleIcon sx={{ color: 'success.main', fontSize: 20, flexShrink: 0 }} />
              ) : (
                <PauseCircleIcon sx={{ color: 'text.disabled', fontSize: 20, flexShrink: 0 }} />
              )}
            </Tooltip>
            <Box sx={{ flex: 1, minWidth: 0 }}>
              <Typography
                variant="body2"
                sx={{
                  fontWeight: 600,
                  lineHeight: 1.4,
                  color: isEnabled ? 'text.primary' : 'text.secondary',
                  overflow: 'hidden',
                  textOverflow: 'ellipsis',
                  whiteSpace: 'nowrap',
                }}
              >
                {task.name || 'Unnamed Task'}
              </Typography>
              {scheduleDisplay && (
                <Typography
                  variant="caption"
                  sx={{
                    color: 'text.secondary',
                    fontSize: '0.7rem',
                    display: 'block',
                    overflow: 'hidden',
                    textOverflow: 'ellipsis',
                    whiteSpace: 'nowrap',
                  }}
                >
                  {scheduleDisplay}
                </Typography>
              )}
            </Box>
          </Box>
          <IconButton size="small" onClick={handleMenuOpen} sx={{ flexShrink: 0 }}>
            <MoreVertIcon sx={{ fontSize: 16 }} />
          </IconButton>
        </Box>

        {(app || (!app && task.app_id)) && (
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 1.5, flexWrap: 'wrap' }}>
            {app && (
              <Typography
                variant="caption"
                sx={{
                  color: theme.palette.primary.main,
                  fontSize: '0.75rem',
                  cursor: 'pointer',
                  overflow: 'hidden',
                  textOverflow: 'ellipsis',
                  whiteSpace: 'nowrap',
                }}
                onClick={(e) => {
                  e.stopPropagation()
                  if (app.id) account.orgNavigate('agent', { app_id: app.id })
                }}
              >
                {app.config?.helix?.name || 'Unnamed Agent'}
              </Typography>
            )}
            {!app && task.app_id && (
              <Typography variant="caption" color="text.secondary" sx={{ fontSize: '0.75rem' }}>
                Agent not found
              </Typography>
            )}
          </Box>
        )}

        <Box sx={{
          background: 'linear-gradient(145deg, rgba(255,255,255,0.03) 0%, rgba(255,255,255,0.01) 100%)',
          borderRadius: 2,
          border: '1px solid rgba(255,255,255,0.06)',
          p: 1.5,
          mt: 'auto',
        }}>
          <Typography variant="caption" sx={{ color: 'text.secondary', fontSize: '0.65rem', display: 'block', mb: 0.75 }}>
            Recent executions
          </Typography>
          <TasksTableExecutionsChart taskId={task.id || ''} />
        </Box>
      </CardContent>

      <Menu anchorEl={anchorEl} open={Boolean(anchorEl)} onClose={handleMenuClose}>
        <MenuItem onClick={(e) => { e.stopPropagation(); handleMenuClose(); onEdit(task) }}>
          <EditIcon sx={{ mr: 1, fontSize: 20 }} />
          Edit
        </MenuItem>
        <MenuItem onClick={(e) => { e.stopPropagation(); handleMenuClose(); onToggleStatus(task) }}>
          {isEnabled ? (
            <>
              <PauseIcon sx={{ mr: 1, fontSize: 20 }} />
              Pause
            </>
          ) : (
            <>
              <PlayArrowIcon sx={{ mr: 1, fontSize: 20 }} />
              Enable
            </>
          )}
        </MenuItem>
        <MenuItem onClick={(e) => { e.stopPropagation(); handleMenuClose(); onDelete(task) }}>
          <DeleteIcon sx={{ mr: 1, fontSize: 20 }} />
          Delete
        </MenuItem>
      </Menu>
    </Card>
  )
}

export default CronTaskCard
