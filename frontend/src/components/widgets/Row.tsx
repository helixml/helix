import React, { FC } from 'react'
import Box from '@mui/material/Box'
import { SxProps } from '@mui/system'

const Row: FC<{
  sx?: SxProps,
}> = ({
  sx = {},
  children,
}) => {
  return (
    <Box
      sx={{
        width: '100%',
        display: 'flex',
        flexDirection: 'row',
        alignItems: 'center',
        justifyContent: 'flex-start',
        ...sx
      }}
    >
      { children }
    </Box>
  )
}

export default Row
