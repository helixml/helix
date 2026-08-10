import { FC, MouseEvent } from 'react'
import Box from '@mui/material/Box'
import Tooltip from '@mui/material/Tooltip'
import Typography from '@mui/material/Typography'
import { ListChecks, TerminalSquare } from 'lucide-react'

import useRouter from '../../hooks/useRouter'
import { TypesSandbox } from '../../api/api'

interface SandboxSourceProps {
  sandbox: TypesSandbox
}

// SandboxSource marks who provisioned the container behind a sandbox row.
//
// Spec-task desktops are metered through the same sandbox rows as
// user-created sandboxes, so the list mixes two very different things: the
// runners behind a task, and containers someone created from the Sandboxes
// API. Without a marker the list reads as a pile of unexplained rows.
const SandboxSource: FC<SandboxSourceProps> = ({ sandbox }) => {
  const router = useRouter()

  if (!sandbox.spec_task_id) {
    const label = sandbox.session_id ? 'Session' : 'Sandbox API'
    return (
      <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.75, minWidth: 0 }}>
        <TerminalSquare size={14} color="currentColor" style={{ flexShrink: 0, opacity: 0.6 }} />
        <Typography variant="body2" color="text.secondary" noWrap>
          {label}
        </Typography>
      </Box>
    )
  }

  const canNavigate = Boolean(sandbox.project_id && router.params.org_id)
  const openTask = (e: MouseEvent) => {
    e.preventDefault()
    e.stopPropagation()
    if (!canNavigate) return
    router.navigate('org_project-task-detail', {
      org_id: router.params.org_id,
      id: sandbox.project_id,
      taskId: sandbox.spec_task_id,
    })
  }

  return (
    <Tooltip title={canNavigate ? 'Open the spec task this runner belongs to' : 'Spec task runner'}>
      <Box
        component={canNavigate ? 'a' : 'span'}
        href={canNavigate ? '#' : undefined}
        onClick={canNavigate ? openTask : undefined}
        sx={{
          display: 'flex',
          alignItems: 'center',
          gap: 0.75,
          minWidth: 0,
          textDecoration: 'none',
          color: 'text.secondary',
          cursor: canNavigate ? 'pointer' : 'default',
          '&:hover': canNavigate ? { color: 'text.primary' } : undefined,
        }}
      >
        <ListChecks size={14} style={{ flexShrink: 0 }} />
        <Typography variant="body2" color="inherit" noWrap>
          Spec task
        </Typography>
      </Box>
    </Tooltip>
  )
}

export default SandboxSource
