import { FC } from 'react'
import Chip from '@mui/material/Chip'
import Tooltip from '@mui/material/Tooltip'

import { TypesSandboxStatus } from '../../api/api'

interface Props {
  status?: TypesSandboxStatus | string
  message?: string
}

const colorMap: Record<string, 'default' | 'primary' | 'success' | 'warning' | 'error' | 'info'> = {
  pending: 'info',
  running: 'success',
  stopping: 'warning',
  stopped: 'default',
  failed: 'error',
}

const SandboxStatusBadge: FC<Props> = ({ status, message }) => {
  const label = status ?? 'unknown'
  const color = colorMap[label] ?? 'default'
  const chip = <Chip size="small" label={label} color={color} />
  if (message) {
    return <Tooltip title={message}><span>{chip}</span></Tooltip>
  }
  return chip
}

export default SandboxStatusBadge
