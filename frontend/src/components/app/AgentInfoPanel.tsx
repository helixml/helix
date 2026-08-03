import { FC } from 'react'
import Box from '@mui/material/Box'
import Divider from '@mui/material/Divider'
import Link from '@mui/material/Link'
import Paper from '@mui/material/Paper'
import Stack from '@mui/material/Stack'
import Typography from '@mui/material/Typography'

import { BotDTO, useHelixOrgBot } from '../../services/helixOrgService'
import useAccount from '../../hooks/useAccount'
import { CODE_AGENT_RUNTIME_DISPLAY_NAMES, CodeAgentRuntime, getModelDisplayName } from '../../contexts/apps'
import { AGENT_TYPE_ZED_EXTERNAL, IApp, IAppFlatState } from '../../types'
import { formatDate } from '../../utils/format'

const timestamp = (value?: string | Date) => {
  if (!value) return '-'
  return formatDate(value instanceof Date ? value.toISOString() : value) || '-'
}

const AgentInfoPanel: FC<{
  app: IApp
  settings: IAppFlatState
  orgAgent?: BotDTO
}> = ({ app, settings, orgAgent }) => {
  const account = useAccount()
  const { data: detail } = useHelixOrgBot(orgAgent?.id, { enabled: !!orgAgent?.id })
  const node = detail?.bot ?? orgAgent
  const runtime = node?.code_agent_runtime ?? node?.agent_runtime ?? settings.code_agent_runtime
  const runtimeName = runtime
    ? CODE_AGENT_RUNTIME_DISPLAY_NAMES[runtime as CodeAgentRuntime] ?? runtime
    : settings.default_agent_type === AGENT_TYPE_ZED_EXTERNAL ? 'Zed Agent' : 'Helix Agent'
  const model = node?.model ?? node?.agent_model ?? settings.model ?? settings.generation_model ?? settings.reasoning_model
  const projectID = detail?.project_id
  const status = node?.agent_status

  return (
    <Box sx={{ p: 2 }}>
      <Paper variant="outlined" sx={{ p: 2 }}>
        <Stack spacing={2}>
          <Box>
            <Typography variant="caption" color="text.secondary">Agent ID</Typography>
            <Typography variant="body2" sx={{ fontFamily: 'monospace', wordBreak: 'break-all' }}>
              {node?.id ?? app.id}
            </Typography>
          </Box>
          <Box>
            <Typography variant="caption" color="text.secondary">Created</Typography>
            <Typography variant="body2">{timestamp(node?.created_at ?? app.created)}</Typography>
          </Box>
          <Box>
            <Typography variant="caption" color="text.secondary">Updated</Typography>
            <Typography variant="body2">{timestamp(node?.updated_at ?? app.updated)}</Typography>
          </Box>
          {orgAgent && (
            <Box>
              <Typography variant="caption" color="text.secondary">Status</Typography>
              <Stack direction="row" alignItems="center" spacing={1}>
                {status && (
                  <Box
                    sx={{
                      width: 8,
                      height: 8,
                      borderRadius: '50%',
                      backgroundColor: status === 'running' ? 'success.main' : 'text.disabled',
                    }}
                  />
                )}
                <Typography variant="body2">
                  {status === 'running' ? 'Running' : status ? 'Stopped' : '-'}
                </Typography>
              </Stack>
            </Box>
          )}
          {orgAgent && projectID && (
            <Box>
              <Typography variant="caption" color="text.secondary" sx={{ display: 'block' }}>Project</Typography>
              <Link
                component="button"
                variant="body2"
                underline="hover"
                onClick={() => account.orgNavigate('project-specs', { id: projectID })}
                sx={{ fontFamily: 'monospace', textAlign: 'left', wordBreak: 'break-all' }}
              >
                {projectID}
              </Link>
            </Box>
          )}
          <Divider />
          <Box>
            <Typography variant="caption" color="text.secondary">Runtime</Typography>
            <Typography variant="body2">{runtimeName}</Typography>
          </Box>
          <Box>
            <Typography variant="caption" color="text.secondary">Model</Typography>
            <Typography variant="body2" title={model}>{model ? getModelDisplayName(model) : '-'}</Typography>
          </Box>
        </Stack>
      </Paper>
    </Box>
  )
}

export default AgentInfoPanel
