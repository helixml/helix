import React, { FC } from 'react'
import Box from '@mui/material/Box'
import { SxProps } from '@mui/system'

const Row: FC<{
  id?: string,
  sx?: SxProps,
  center?: boolean,
  vertical?: boolean,
}> = ({
  id,
  sx = {},
  center = false,
  vertical = false,
  children,
}) => {
  return (
    <Box
      id={ id }
      sx={{
        width: '100%',
        display: 'flex',
        flexDirection: vertical ? 'column' : 'row',
        alignItems: 'center',
        justifyContent: center ? 'center' : 'flex-start',
        ...sx
      }}
    >
      { children }
    </Box>
  )
}

export default Row
