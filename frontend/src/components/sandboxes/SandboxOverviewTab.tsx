import { FC } from 'react'
import Box from '@mui/material/Box'
import Stack from '@mui/material/Stack'
import Typography from '@mui/material/Typography'

import SandboxStatusBadge from './SandboxStatusBadge'
import { TypesSandbox } from '../../api/api'

interface Props {
  sandbox: TypesSandbox
}

const isHeadless = (sandbox: TypesSandbox): boolean =>
  (sandbox.runtime || '').includes('headless')

const Row: FC<{ label: string; value: React.ReactNode }> = ({ label, value }) => (
  <Box display="flex" gap={2} alignItems="baseline">
    <Typography variant="body2" color="text.secondary" sx={{ width: 160, flexShrink: 0 }}>
      {label}
    </Typography>
    <Typography variant="body2" sx={{ fontFamily: 'monospace' }}>
      {value}
    </Typography>
  </Box>
)

const SandboxOverviewTab: FC<Props> = ({ sandbox }) => {
  return (
    <Box sx={{ p: 2, borderRadius: 1, border: '1px solid', borderColor: 'divider' }}>
      <Stack spacing={1.5}>
        <Row label="ID" value={sandbox.id} />
        <Row label="Name" value={sandbox.name || '-'} />
        <Row label="Status" value={<SandboxStatusBadge status={sandbox.status} message={sandbox.status_message} />} />
        <Row label="Runtime" value={sandbox.runtime || 'ubuntu-desktop'} />
        <Row label="Image" value={sandbox.image || '-'} />
        <Row label="vCPU / Memory" value={`${sandbox.vcpus ?? 1} CPU / ${sandbox.memory_mb ?? 2048} MB`} />
        {!isHeadless(sandbox) && (
          <Row label="Display" value={`${sandbox.display_width ?? 0}x${sandbox.display_height ?? 0} @ ${sandbox.display_fps ?? 0} fps`} />
        )}
        <Row label="Container" value={sandbox.container_id || '-'} />
        <Row label="Host" value={sandbox.host_device_id || '-'} />
        <Row label="Created" value={sandbox.created_at ? new Date(sandbox.created_at).toLocaleString() : '-'} />
        <Row label="Started" value={sandbox.started_at ? new Date(sandbox.started_at).toLocaleString() : '-'} />
        <Row label="Expires" value={sandbox.expires_at ? new Date(sandbox.expires_at).toLocaleString() : 'Never'} />
      </Stack>
    </Box>
  )
}

export default SandboxOverviewTab
