import React, { FC } from 'react'
import Box from '@mui/material/Box'

const Cell: FC<{
  flexGrow?: number,
}> = ({
  flexGrow = 1,
  children,
}) => {
  return (
    <Box
      sx={{
        flexGrow,
      }}
    >
      { children }
    </Box>
  )
}

export default Cell
