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
  reverse?: boolean,
  size?: number,
}> = ({
  modelName,
  reverse = false,
  size = 20,
}) => {
  const color = getColor(modelName)
  return (
    <Box
      sx={{
        width: size,
        height: size,
        backgroundColor: reverse ? 'transparent' : color,
        borderRadius: '50%',
        border: `2px solid ${color}`,
        boxShadow: `0 0 4px ${color}`,
        display: 'flex',
        alignItems: 'center', 
        justifyContent: 'center',
        position: 'relative',
        '&::after': {
          content: '""',
          position: 'absolute',
          top: 2,
          left: 2,
          right: 2,
          bottom: 2,
          borderRadius: '50%',
          backgroundColor: reverse ? color : 'transparent',
          opacity: reverse ? 0.5 : 1,
        }
      }}
    >
    </Box>
  )
}

export default SessionBadge