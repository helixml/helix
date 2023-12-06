import React, { FC } from 'react'
import Box from '@mui/material/Box'
import { SxProps } from '@mui/system'

const Cell: FC<{
  flexGrow?: number,
  grow?: boolean,
  sx?: SxProps,
}> = ({
  flexGrow = 0,
  grow = false,
  sx = {},
  children,
}) => {
  return (
    <Box
      sx={{
        flexGrow: grow ? 1 : flexGrow,
        ...sx
      }}
    >
      { children }
    </Box>
  )
}

export default Cell
