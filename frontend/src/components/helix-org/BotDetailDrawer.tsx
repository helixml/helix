import { FC, useEffect, useMemo, useState } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Chip from '@mui/material/Chip'
import Divider from '@mui/material/Divider'
import FormControl from '@mui/material/FormControl'
import InputLabel from '@mui/material/InputLabel'
import MenuItem from '@mui/material/MenuItem'
import Select from '@mui/material/Select'
import Stack from '@mui/material/Stack'
import Typography from '@mui/material/Typography'
import ArrowForwardRoundedIcon from '@mui/icons-material/ArrowForwardRounded'
import CheckCircleOutlineRoundedIcon from '@mui/icons-material/CheckCircleOutlineRounded'
import PendingOutlinedIcon from '@mui/icons-material/PendingOutlined'
import SmartToyOutlinedIcon from '@mui/icons-material/SmartToyOutlined'
import WorkOutlineRoundedIcon from '@mui/icons-material/WorkOutlineRounded'

import type { BotDTO } from '../../services/helixOrgService'
import type { SpecTask } from '../../services/specTaskService'
import type { TypesProject } from '../../api/api'
import useRouter from '../../hooks/useRouter'
import HelixOrgOverviewCard from './HelixOrgOverviewCard'
import HelixOrgSideDrawer from './HelixOrgSideDrawer'
import SpecTaskStatusBadge, { formatSpecTaskStatus } from './SpecTaskStatusBadge'

type BotDetailDrawerProps = {
  botId?: string
  bot?: BotDTO
  project?: TypesProject
  tasks: SpecTask[]
  onClose: () => void
}

const BotDetailDrawer: FC<BotDetailDrawerProps> = ({ botId, bot, project, tasks, onClose }) => {
  const router = useRouter()
  const [statusFilter, setStatusFilter] = useState('all')
  const open = Boolean(botId)

  useEffect(() => {
    setStatusFilter('all')
  }, [botId])

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

  const navigateToTask = (task: SpecTask) => {
    if (!task.id || !task.project_id) return
    router.navigate('org_project-task-detail', {
      org_id: router.params.org_id,
      id: task.project_id,
      taskId: task.id,
    })
  }

  return (
    <HelixOrgSideDrawer
      open={open}
      onClose={onClose}
      title={bot?.name || botId || 'Bot'}
      width={560}
      allowInteractionBehind
      headerAction={botId ? (
        <Button
          size="small"
          onClick={() => router.navigate('helix_org_bot_detail', {
            org_id: router.params.org_id,
            bot_id: botId,
          })}
        >
          Details
        </Button>
      ) : undefined}
    >
      {!bot ? (
        <Typography color="text.secondary">Bot not found.</Typography>
      ) : (
        <Stack spacing={2.5}>
          <HelixOrgOverviewCard
            title={bot.name || bot.id || 'Bot'}
            id={bot.id}
            icon={<SmartToyOutlinedIcon sx={{ fontSize: 20 }} />}
            status={(
              <Chip
                label={bot.agent_status === 'running' ? 'Online' : 'Offline'}
                size="small"
                sx={{
                  color: 'common.white',
                  backgroundColor: bot.agent_status === 'running' ? 'rgba(34,197,94,0.24)' : 'rgba(255,255,255,0.12)',
                  border: '1px solid rgba(255,255,255,0.22)',
                  flexShrink: 0,
                }}
              />
            )}
          >
            <Chip
              icon={<WorkOutlineRoundedIcon sx={{ color: 'inherit !important' }} />}
              label={`${tasks.length} spec task${tasks.length === 1 ? '' : 's'}`}
              size="small"
              sx={{ color: 'common.white', backgroundColor: 'rgba(255,255,255,0.11)' }}
            />
            {project?.name && (
              <Chip
                label={project.name}
                size="small"
                sx={{ maxWidth: '60%', color: 'common.white', backgroundColor: 'rgba(255,255,255,0.11)', '& .MuiChip-label': { overflow: 'hidden', textOverflow: 'ellipsis' } }}
              />
            )}
          </HelixOrgOverviewCard>

          <Divider />

          <Box>
            <Stack direction="row" alignItems="center" justifyContent="space-between" spacing={1} sx={{ mb: 1.25 }}>
              <Box>
                <Typography variant="subtitle1" sx={{ fontWeight: 700 }}>Project spec tasks</Typography>
                {project?.name && (
                  <Typography variant="caption" color="text.secondary">
                    {project.name}
                  </Typography>
                )}
              </Box>
              <FormControl size="small" sx={{ minWidth: 145 }}>
                <InputLabel id="bot-task-status-filter">Status</InputLabel>
                <Select
                  labelId="bot-task-status-filter"
                  value={statusFilter}
                  label="Status"
                  onChange={(event) => setStatusFilter(event.target.value)}
                >
                  <MenuItem value="all">All statuses</MenuItem>
                  {statusOptions.map((status) => <MenuItem key={status} value={status}>{formatSpecTaskStatus(status)}</MenuItem>)}
                </Select>
              </FormControl>
            </Stack>

            {filteredTasks.length === 0 ? (
              <Box sx={{ p: 2.5, textAlign: 'center', border: '1px dashed', borderColor: 'divider', borderRadius: 1.5 }}>
                <PendingOutlinedIcon sx={{ color: 'text.disabled', mb: 0.5 }} />
                <Typography variant="body2" color="text.secondary">
                  {tasks.length === 0 ? 'No spec tasks are linked to this bot yet.' : 'No tasks match this status.'}
                </Typography>
              </Box>
            ) : (
              <Stack spacing={1}>
                {filteredTasks.map((task) => {
                  const title = task.user_short_title || task.short_title || task.name || task.id || 'Untitled task'
                  const status = String(task.status ?? 'unknown')
                  return (
                    <Box
                      key={task.id}
                      component="button"
                      type="button"
                      onClick={() => navigateToTask(task)}
                      disabled={!task.id || !task.project_id}
                      sx={{
                        width: '100%',
                        p: 1.25,
                        display: 'block',
                        textAlign: 'left',
                        border: '1px solid',
                        borderColor: 'divider',
                        borderRadius: 1.5,
                        backgroundColor: 'background.paper',
                        color: 'inherit',
                        cursor: task.id && task.project_id ? 'pointer' : 'default',
                        transition: 'border-color 0.15s ease, background-color 0.15s ease, transform 0.15s ease',
                        '&:hover:not(:disabled)': { borderColor: 'primary.main', backgroundColor: 'action.hover', transform: 'translateX(2px)' },
                        '&:disabled': { opacity: 0.62 },
                      }}
                    >
                      <Stack direction="row" spacing={1.25} alignItems="center">
                        <Box sx={{ color: 'text.disabled', display: 'flex' }}>
                          {status === 'done' || status === 'completed' ? <CheckCircleOutlineRoundedIcon color="success" /> : <PendingOutlinedIcon />}
                        </Box>
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
            )}
          </Box>
        </Stack>
      )}
    </HelixOrgSideDrawer>
  )
}

export default BotDetailDrawer
