import { FC } from 'react'
import Box from '@mui/material/Box'
import Stack from '@mui/material/Stack'
import Typography from '@mui/material/Typography'

import { BotDTO } from '../../services/helixOrgService'
import { IApp } from '../../types'
import { formatDate } from '../../utils/format'

const timestamp = (value?: string | Date) => {
  if (!value) return '-'
  return formatDate(value instanceof Date ? value.toISOString() : value) || '-'
}

const AgentInfoPanel: FC<{
  app: IApp
  orgAgent?: BotDTO
}> = ({ app, orgAgent }) => {
  const status = orgAgent?.agent_status

  return (
    <Box sx={{ pt: 2 }}>
      <Stack spacing={2}>
        <Box>
          <Typography variant="caption" color="text.secondary">Agent ID</Typography>
          <Typography variant="body2" sx={{ fontFamily: 'monospace', wordBreak: 'break-all' }}>
            {orgAgent?.id ?? app.id}
          </Typography>
        </Box>
        <Box>
          <Typography variant="caption" color="text.secondary">Created</Typography>
          <Typography variant="body2">{timestamp(orgAgent?.created_at ?? app.created)}</Typography>
        </Box>
        <Box>
          <Typography variant="caption" color="text.secondary">Updated</Typography>
          <Typography variant="body2">{timestamp(orgAgent?.updated_at ?? app.updated)}</Typography>
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
      </Stack>
    </Box>
  )
}

export default AgentInfoPanel
