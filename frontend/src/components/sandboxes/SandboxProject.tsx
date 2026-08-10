import { FC } from 'react'
import Typography from '@mui/material/Typography'

import useAccount from '../../hooks/useAccount'
import { useListProjects } from '../../services/projectService'
import { TypesSandbox } from '../../api/api'

interface SandboxProjectProps {
  sandbox: TypesSandbox
  variant?: 'caption' | 'body2'
}

// SandboxProject names the project a sandbox belongs to.
//
// Sandbox names are labels, not identifiers — a spec-task runner takes its
// task's name, and two projects in the same org routinely have a task called
// "test". The org-wide list would then show several identically-named rows
// with no way to tell them apart, which defeats the point of a list whose job
// is cost attribution. The project is the axis that separates them.
//
// The projects query is shared: React Query dedupes it across every row, so a
// list of N sandboxes still issues one request.
const SandboxProject: FC<SandboxProjectProps> = ({ sandbox, variant = 'caption' }) => {
  const account = useAccount()
  const orgId = account.organizationTools.organization?.id
  const { data: projects } = useListProjects(orgId, { enabled: Boolean(orgId) })

  if (!sandbox.project_id) {
    return (
      <Typography variant={variant} color="text.secondary" noWrap>
        Org-scoped
      </Typography>
    )
  }

  const project = projects?.find(p => p.id === sandbox.project_id)
  return (
    <Typography variant={variant} color="text.secondary" noWrap>
      {project?.name || sandbox.project_id}
    </Typography>
  )
}

export default SandboxProject
