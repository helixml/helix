import Alert from '@mui/material/Alert'
import Chip from '@mui/material/Chip'
import Tooltip from '@mui/material/Tooltip'
import { TriangleAlert } from 'lucide-react'
import { FC } from 'react'

export const ConfigurationWarningChip: FC<{ warning?: string }> = ({ warning }) => {
  if (!warning) return null
  return (
    <Tooltip title={warning} arrow>
      <Chip
        icon={<TriangleAlert size={14} />}
        label="Needs attention"
        color="warning"
        size="small"
        variant="outlined"
      />
    </Tooltip>
  )
}

export const ConfigurationWarningAlert: FC<{ warning?: string }> = ({ warning }) => {
  if (!warning) return null
  return <Alert severity="warning" sx={{ mb: 2 }}>{warning}</Alert>
}
