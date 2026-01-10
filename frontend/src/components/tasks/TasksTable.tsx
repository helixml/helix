import React, { FC, useMemo, useState } from 'react'
import EditIcon from '@mui/icons-material/Edit'
import DeleteIcon from '@mui/icons-material/Delete'
import PauseIcon from '@mui/icons-material/Pause'
import PlayArrowIcon from '@mui/icons-material/PlayArrow'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import Tooltip from '@mui/material/Tooltip'
import Chip from '@mui/material/Chip'
import MoreVertIcon from '@mui/icons-material/MoreVert'
import Menu from '@mui/material/Menu'
import MenuItem from '@mui/material/MenuItem'
import IconButton from '@mui/material/IconButton'
import ScheduleIcon from '@mui/icons-material/Schedule'
import BlockIcon from '@mui/icons-material/Block'

import SimpleTable from '../widgets/SimpleTable'
import Row from '../widgets/Row'
import Cell from '../widgets/Cell'
import TasksTableExecutionsChart from './TasksTableExecutionsChart'

import useTheme from '@mui/material/styles/useTheme'
import useAccount from '../../hooks/useAccount'

import { TypesTriggerConfiguration } from '../../api/api'
import { generateCronShortSummary } from '../../utils/cronUtils'

import {
  IApp,
} from '../../types'

// Helper function to format cron schedule for display
const formatCronSchedule = (schedule: string): string => {
  return generateCronShortSummary(schedule)
}

// TasksTable contains the recurring agent tasks table (not the spec tasks table)
const TasksTable: FC<{
  authenticated: boolean,
  data: TypesTriggerConfiguration[],
  apps: IApp[],
  onEdit: (task: TypesTriggerConfiguration) => void,
  onDelete: (task: TypesTriggerConfiguration) => void,
  onToggleStatus: (task: TypesTriggerConfiguration) => void,
}> = ({
  authenticated,
  data,
  apps,
  onEdit,
  onDelete,
  onToggleStatus,
}) => {
  const theme = useTheme()
  const account = useAccount()
  const [anchorEl, setAnchorEl] = useState<null | HTMLElement>(null)
  const [currentTask, setCurrentTask] = useState<TypesTriggerConfiguration | null>(null)

  const handleMenuClick = (event: React.MouseEvent<HTMLElement>, task: TypesTriggerConfiguration) => {
    setAnchorEl(event.currentTarget)
    setCurrentTask(task)
  }

  const handleMenuClose = () => {
    setAnchorEl(null)
    setCurrentTask(null)
  }

  const handleEdit = () => {
    if (currentTask) {
      onEdit(currentTask)
    }
    handleMenuClose()
  }

  const handleDelete = () => {
    if (currentTask) {
      onDelete(currentTask)
    }
    handleMenuClose()
  }

  const handleToggleStatus = () => {
    if (currentTask) {
      onToggleStatus(currentTask)
    }
    handleMenuClose()
  }

  // Function to find app by ID
  const findAppById = (appId: string): IApp | undefined => {
    return apps.find(app => app.id === appId)
  }

  const tableData = useMemo(() => {
    return data.map(task => {
      const isEnabled = task.enabled && !task.archived
      const app = task.app_id ? findAppById(task.app_id) : undefined
      
      // Get the cron schedule from the trigger configuration
      const cronSchedule = task.trigger?.cron?.schedule || ''
      const scheduleDisplay = formatCronSchedule(cronSchedule)
      
      return {
        id: task.id,
        _data: task,
        name: (
          <Row>
            <Cell sx={{ pr: 2, display: 'flex', alignItems: 'center' }}>
              <ScheduleIcon
                sx={{ 
                  color: isEnabled ? 'success.main' : 'text.disabled',
                  fontSize: 20 
                }} 
              />  
            </Cell>
            <Cell grow>
              <Typography variant="body1">
                <a
                  style={{
                    textDecoration: 'none',
                    fontWeight: 'bold',
                    color: theme.palette.mode === 'dark' ? theme.palette.text.primary : theme.palette.text.secondary,
                  }}
                  href="#"
                  onClick={(e: React.MouseEvent<HTMLAnchorElement, MouseEvent>) => {
                    e.preventDefault()
                    e.stopPropagation()
                    onEdit(task)
                  }}
                >
                  {task.name || 'Unnamed Task'}
                </a>
              </Typography>
            </Cell>
          </Row>
        ),
        next_run: (
          <Typography variant="body2" color="text.secondary">
            {task.status}
          </Typography>
        ),
        agent: app ? (
          <Typography variant="body2">
            <a
              style={{
                textDecoration: 'none',
                color: theme.palette.primary.main,
              }}
              href="#"
              onClick={(e: React.MouseEvent<HTMLAnchorElement, MouseEvent>) => {
                e.preventDefault()
                e.stopPropagation()
                // Navigate to the app using the proper navigation method
                account.orgNavigate('app', { app_id: app.id })
              }}
            >
              {app.config.helix.name || 'Unnamed Agent'}
            </a>
          </Typography>
        ) : (
          <Typography variant="body2" color="text.secondary">
            {task.app_id ? 'Agent not found' : 'No agent'}
          </Typography>
        ),
        status: (
          <Box sx={{ display: 'flex', alignItems: 'center' }}>
            <Chip
              label={isEnabled ? 'Enabled' : 'Disabled'}
              size="small"
              color={isEnabled ? 'success' : 'default'}
              icon={isEnabled ? <ScheduleIcon /> : <BlockIcon />}
              sx={{
                backgroundColor: isEnabled 
                  ? theme.palette.success.main 
                  : theme.palette.mode === 'dark' 
                    ? theme.palette.grey[700] 
                    : theme.palette.grey[300],
                color: isEnabled 
                  ? theme.palette.success.contrastText 
                  : theme.palette.text.secondary,
                '& .MuiChip-icon': {
                  color: 'inherit',
                }
              }}
            />
          </Box>
        ),
        executions: (
          <TasksTableExecutionsChart taskId={task.id || ''} />
        ),
      }
    })
  }, [data, apps, theme, onEdit, account])

  const getActions = (task: any) => {
    return (
      <IconButton
        aria-label="more"
        aria-controls="long-menu"
        aria-haspopup="true"
        onClick={(e) => handleMenuClick(e, task._data as TypesTriggerConfiguration)}
      >
        <MoreVertIcon />
      </IconButton>
    )
  }

  return (
    <>
      <SimpleTable
        authenticated={ authenticated }
        fields={[
          {
            name: 'name',
            title: 'Name',
          },
          {
            name: 'next_run',
            title: 'Next Run',
          },
          {
            name: 'agent',
            title: 'Agent',
          },
          {
            name: 'executions',
            title: 'Executions',
          }
        ]}
        data={tableData}
        getActions={getActions}
      />
      <Menu
        id="long-menu"
        anchorEl={anchorEl}
        open={Boolean(anchorEl)}
        onClose={handleMenuClose}
      >
        <MenuItem onClick={handleEdit}>
          <EditIcon sx={{ mr: 1, fontSize: 20 }} />
          Edit
        </MenuItem>
        <MenuItem onClick={handleToggleStatus}>
          {currentTask?.enabled && !currentTask?.archived ? (
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
        <MenuItem onClick={handleDelete}>
          <DeleteIcon sx={{ mr: 1, fontSize: 20 }} />
          Delete
        </MenuItem>
      </Menu>
    </>
  )
}

export default TasksTable 