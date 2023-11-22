import React, { FC } from 'react'
import Box from '@mui/material/Box'

import {
  ISessionType,
  ISessionMode,
} from '../../types'

import {
  getColor,
} from '../../utils/session'

export const SessionBadge: FC<{
  type: ISessionType,
  mode: ISessionMode
  size?: number,
}> = ({
  type,
  mode,
  size = 20,
}) => {
  const color = getColor(type, mode)
  return (
    <Box
      sx={{
        width: size,
        height: size,
        backgroundColor: color,
        borderRadius: '50%',
        border: '1px solid #000000'
      }}
    >
    </Box>
  )
}

export default SessionBadge