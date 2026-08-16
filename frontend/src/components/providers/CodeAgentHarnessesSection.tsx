import { FC } from 'react'
import Box from '@mui/material/Box'
import CircularProgress from '@mui/material/CircularProgress'
import Stack from '@mui/material/Stack'
import Switch from '@mui/material/Switch'
import Typography from '@mui/material/Typography'

import {
  TypesOrgCodeAgentHarnessStatus,
  TypesOrgCodeAgentHarnessUpdate,
} from '../../api/api'
import AgentHarness from '../agent/AgentHarness'

const CodeAgentHarnessesSection: FC<{
  harnesses: TypesOrgCodeAgentHarnessStatus[]
  loading?: boolean
  readOnly?: boolean
  onChange: (update: TypesOrgCodeAgentHarnessUpdate) => void
}> = ({ harnesses, loading = false, readOnly = false, onChange }) => {
  if (loading) {
    return (
      <Stack direction="row" alignItems="center" spacing={1.5} sx={{ py: 3 }}>
        <CircularProgress size={16} />
        <Typography variant="body2" color="text.secondary">
          Loading coding harnesses…
        </Typography>
      </Stack>
    )
  }

  return (
    <Box sx={{ borderTop: '1px solid', borderColor: 'divider' }}>
      {harnesses.map((harness) => (
        <Stack
          key={harness.runtime}
          direction="row"
          alignItems="center"
          spacing={1.5}
          sx={{ minHeight: 58, px: 1, borderBottom: '1px solid', borderColor: 'divider' }}
        >
          <Box
            aria-hidden="true"
            sx={{
              width: 8,
              height: 8,
              borderRadius: '50%',
              bgcolor: harness.enabled ? 'success.main' : 'action.disabled',
              flexShrink: 0,
            }}
          />
          <Box sx={{ minWidth: 0, flex: 1 }}>
            <AgentHarness runtime={harness.runtime} variant="long" size={18} showTooltip={false} />
            <Typography variant="caption" color="text.secondary" display="block" sx={{ mt: 0.25 }}>
              {harness.enabled
                ? 'Available for tasks in this organization'
                : 'Not available for tasks in this organization'}
            </Typography>
          </Box>
          <Switch
            checked={harness.enabled ?? false}
            disabled={readOnly}
            inputProps={{ 'aria-label': `Enable ${harness.runtime}` }}
            onChange={(event) => onChange({
              runtime: harness.runtime!,
              enabled: event.target.checked,
            })}
          />
        </Stack>
      ))}
    </Box>
  )
}

export default CodeAgentHarnessesSection
