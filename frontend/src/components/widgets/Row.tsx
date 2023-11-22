import React, { FC } from 'react'
import Box from '@mui/material/Box'

const Row: FC<{}> = ({
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
      }}
    >
      { children }
    </Box>
  )
}

export default Row
