import React, { FC } from 'react'
import Box from '@mui/material/Box'

import {
  ISessionMode,
} from '../../types'

import {
  getColor,
} from '../../utils/session'

export const SessionBadge: FC<{
  modelName: string,
  mode: ISessionMode,
  reverse?: boolean,
  size?: number,
}> = ({
  modelName,
  mode,
  reverse = false,
  size = 20,
}) => {
  const color = getColor(modelName, mode)
  return (
    <Box
      sx={{
        width: size,
        height: size,
        backgroundColor: reverse ? '' : color,
        borderRadius: '50%',
        border: `1px solid ${reverse ? color : '#000000'}`
      }}
    >
    </Box>
  )
}

export default SessionBadge