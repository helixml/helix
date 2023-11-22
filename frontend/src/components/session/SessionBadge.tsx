import React, { FC } from 'react'
import Box from '@mui/material/Box'

import {
  ISession,
} from '../../types'

import {
  getColor,
} from '../../utils/session'

export const SessionBadge: FC<{
  session: ISession,
  size?: number,
}> = ({
  session,
  size = 20,
}) => {
  const color = getColor(session)
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