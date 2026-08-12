import { FC, useEffect, useMemo, useState } from 'react'
import Box from '@mui/material/Box'
import CircularProgress from '@mui/material/CircularProgress'
import FormControl from '@mui/material/FormControl'
import InputLabel from '@mui/material/InputLabel'
import MenuItem from '@mui/material/MenuItem'
import Select from '@mui/material/Select'
import Stack from '@mui/material/Stack'
import Typography from '@mui/material/Typography'
import ArrowForwardRoundedIcon from '@mui/icons-material/ArrowForwardRounded'
import PendingOutlinedIcon from '@mui/icons-material/PendingOutlined'

import useRouter from '../../hooks/useRouter'
import { SpecTask, useSpecTasks } from '../../services/specTaskService'
import SpecTaskStatusBadge, { formatSpecTaskStatus } from './SpecTaskStatusBadge'

type Props = {
  botID: string
  projectID?: string
}

const EmptyState: FC<{ children: string }> = ({ children }) => (
  <Box sx={{ p: 3, textAlign: 'center', m: 'auto' }}>
    <Typography variant="body2" color="text.secondary">{children}</Typography>
  </Box>
)

const HelixOrgBotPanelTab: FC<Props> = ({ botID, projectID }) => {
  const router = useRouter()
  const tasksQuery = useSpecTasks({
    projectId: projectID,
    enabled: !!projectID,
    refetchInterval: 5000,
  })
  const [statusFilter, setStatusFilter] = useState('all')

  useEffect(() => setStatusFilter('all'), [botID])

  const tasks = (tasksQuery.data ?? []) as SpecTask[]
  const statusOptions = useMemo(
    () => Array.from(new Set(tasks.map((task) => String(task.status ?? 'unknown'))))
      .sort((a, b) => formatSpecTaskStatus(a).localeCompare(formatSpecTaskStatus(b))),
    [tasks],
  )
  const filteredTasks = useMemo(
    () => statusFilter === 'all'
      ? tasks
      : tasks.filter((task) => String(task.status ?? 'unknown') === statusFilter),
    [statusFilter, tasks],
  )

  if (!projectID) {
    return <EmptyState>This agent has no project for spec tasks.</EmptyState>
  }
  if (tasksQuery.isLoading) {
    return <Box sx={{ m: 'auto' }}><CircularProgress size={24} /></Box>
  }
  if (tasksQuery.isError) {
    return <EmptyState>Could not load this agent's project tasks.</EmptyState>
  }

  return (
    <Stack spacing={1.25} sx={{ flex: 1, minHeight: 0, p: 1.5, overflow: 'auto' }}>
      <FormControl size="small" fullWidth>
        <InputLabel id="bot-task-status-filter">Status</InputLabel>
        <Select
          labelId="bot-task-status-filter"
          value={statusFilter}
          label="Status"
          onChange={(event) => setStatusFilter(event.target.value)}
        >
          <MenuItem value="all">All statuses</MenuItem>
          {statusOptions.map((status) => (
            <MenuItem key={status} value={status}>{formatSpecTaskStatus(status)}</MenuItem>
          ))}
        </Select>
      </FormControl>
      {filteredTasks.length === 0 ? (
        <Box sx={{ p: 2.5, textAlign: 'center', border: '1px dashed', borderColor: 'divider', borderRadius: 1.5 }}>
          <PendingOutlinedIcon sx={{ color: 'text.disabled', mb: 0.5 }} />
          <Typography variant="body2" color="text.secondary">
            {tasks.length === 0 ? 'No spec tasks are linked to this agent yet.' : 'No tasks match this status.'}
          </Typography>
        </Box>
      ) : filteredTasks.map((task) => {
        const title = task.user_short_title || task.short_title || task.name || task.id || 'Untitled task'
        const status = String(task.status ?? 'unknown')
        return (
          <Box
            key={task.id}
            component="button"
            type="button"
            onClick={() => {
              if (!task.id || !task.project_id) return
              router.navigate('org_project-task-detail', {
                org_id: router.params.org_id,
                id: task.project_id,
                taskId: task.id,
              })
            }}
            disabled={!task.id || !task.project_id}
            sx={{
              width: '100%',
              p: 1.25,
              textAlign: 'left',
              border: '1px solid',
              borderColor: 'divider',
              borderRadius: 1.5,
              backgroundColor: 'background.paper',
              color: 'inherit',
              cursor: task.id && task.project_id ? 'pointer' : 'default',
              '&:hover:not(:disabled)': { borderColor: 'primary.main', backgroundColor: 'action.hover' },
              '&:disabled': { opacity: 0.62 },
            }}
          >
            <Stack direction="row" spacing={1} alignItems="center">
              <Box sx={{ flex: 1, minWidth: 0 }}>
                <Typography variant="body2" sx={{ fontWeight: 650, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                  {title}
                </Typography>
                <Typography variant="caption" color="text.secondary" sx={{ fontFamily: 'monospace' }}>
                  {task.task_number ? `#${task.task_number}` : task.id}
                </Typography>
              </Box>
              <SpecTaskStatusBadge status={status} />
              <ArrowForwardRoundedIcon sx={{ fontSize: 18, color: 'text.disabled' }} />
            </Stack>
          </Box>
        )
      })}
    </Stack>
  )
}

export default HelixOrgBotPanelTab
