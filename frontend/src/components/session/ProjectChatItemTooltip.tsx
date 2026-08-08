import type { FC, ReactElement } from 'react'
import Box from '@mui/material/Box'
import Tooltip from '@mui/material/Tooltip'
import Typography from '@mui/material/Typography'
import { BrainCircuit, FolderGit2, GitBranch } from 'lucide-react'

import useApps from '../../hooks/useApps'
import AgentHarness, { getAgentHarnessModel, getAgentHarnessRuntime } from '../agent/AgentHarness'
import type { SidebarItem } from './ProjectChatSidebar.logic'

type ProjectChatItemTooltipProps = {
  item: SidebarItem
  repository?: string
  branch?: string
  children: ReactElement
}

const runtimeLabel = (runtime?: string): string => {
  switch (runtime) {
    case 'claude_code': return 'Claude Code'
    case 'codex_cli': return 'Codex'
    case 'qwen_code': return 'Qwen Code'
    case 'goose_code': return 'Goose'
    case 'zed_agent': return 'Zed Agent'
    case 'zed_external': return 'External Agent'
    case 'helix': return 'Helix'
    default: return runtime || ''
  }
}

const ProjectChatItemTooltip: FC<ProjectChatItemTooltipProps> = ({
  item,
  repository,
  branch,
  children,
}) => {
  const { apps } = useApps()
  const configuredAppID = item.task?.helix_app_id || item.session?.app_id
  const configuredApp = apps.find((app) => app.id === configuredAppID)
  const runtime = configuredApp
    ? getAgentHarnessRuntime(configuredApp)
    : item.session?.metadata?.code_agent_runtime || item.session?.metadata?.agent_type
  const harness = runtimeLabel(runtime)
  const model = configuredApp
    ? getAgentHarnessModel(configuredApp)
    : item.session?.model_name
  const rows = [
    repository && { icon: <FolderGit2 size={13} />, value: repository },
    branch && { icon: <GitBranch size={13} />, value: branch },
    harness && {
      icon: <Box sx={{ width: 13, height: 13, display: 'inline-flex', alignItems: 'center', justifyContent: 'center' }}>
        <AgentHarness runtime={runtime || ''} variant="short" size={13} />
      </Box>,
      value: harness,
    },
    model && { icon: <BrainCircuit size={13} />, value: model },
  ].filter(Boolean) as Array<{ icon: ReactElement; value: string }>

  return (
    <Tooltip
      placement="right-start"
      enterDelay={75}
      enterNextDelay={40}
      leaveDelay={0}
      disableInteractive
      TransitionProps={{ timeout: 0 }}
      slotProps={{
        popper: { modifiers: [{ name: 'offset', options: { offset: [0, 8] } }] },
        tooltip: {
          sx: {
            maxWidth: 300,
            p: 1.25,
            border: '1px solid rgba(255,255,255,0.12)',
            borderRadius: '7px',
            backgroundColor: '#171717',
            color: '#f4f4f5',
            boxShadow: '0 10px 24px rgba(0,0,0,0.35)',
          },
        },
      }}
      title={(
        <Box sx={{ minWidth: 170 }}>
          <Typography sx={{ mb: rows.length ? 0.75 : 0, fontSize: '12px', fontWeight: 600, lineHeight: 1.35 }}>
            {item.title}
          </Typography>
          {rows.map((row) => (
            <Box key={row.value} sx={{ display: 'flex', alignItems: 'center', gap: 0.75, minHeight: 20, color: '#a1a1aa' }}>
              <Box sx={{ width: 14, display: 'inline-flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0 }}>
                {row.icon}
              </Box>
              <Typography sx={{ minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', fontSize: '11px', lineHeight: 1.35 }}>
                {row.value}
              </Typography>
            </Box>
          ))}
        </Box>
      )}
    >
      {children}
    </Tooltip>
  )
}

export default ProjectChatItemTooltip
