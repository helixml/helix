import { FC } from 'react'
import Box from '@mui/material/Box'
import Tooltip from '@mui/material/Tooltip'

export const PRESENCE_ONLINE_COLOR = '#22c55e'
export const PRESENCE_OFFLINE_COLOR = 'rgba(148,163,184,0.55)'

type PresenceDotProps = {
  online: boolean
  size?: number
  /** Ring color so the dot reads when overlaid on an avatar. */
  ringColor?: string
  tooltip?: boolean
}

// Green/grey presence indicator driven by the members list's `online` flag.
const PresenceDot: FC<PresenceDotProps> = ({ online, size = 8, ringColor, tooltip = true }) => {
  const dot = (
    <Box
      component="span"
      role="img"
      aria-label={online ? 'Online' : 'Offline'}
      data-presence={online ? 'online' : 'offline'}
      sx={{
        display: 'inline-block',
        width: size,
        height: size,
        borderRadius: '50%',
        flexShrink: 0,
        backgroundColor: online ? PRESENCE_ONLINE_COLOR : PRESENCE_OFFLINE_COLOR,
        boxShadow: ringColor ? `0 0 0 2px ${ringColor}` : 'none',
      }}
    />
  )
  if (!tooltip) return dot
  return <Tooltip title={online ? 'Online' : 'Offline'}>{dot}</Tooltip>
}

export default PresenceDot
