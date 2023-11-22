import React, { FC } from 'react'
import Box from '@mui/material/Box'
import { SxProps } from '@mui/system'

const Cell: FC<{
  flexGrow?: number,
  sx?: SxProps,
}> = ({
  flexGrow = 1,
  sx = {},
  children,
}) => {
  return (
    <Box
      sx={{
        flexGrow,
        ...sx
      }}
    >
      { children }
    </Box>
  )
}

export default Cell
