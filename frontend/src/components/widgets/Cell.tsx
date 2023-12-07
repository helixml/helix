import React, { FC } from 'react'
import { useTheme, Breakpoint } from '@mui/material/styles'
import useMediaQuery from '@mui/material/useMediaQuery'
import Box from '@mui/material/Box'
import { SxProps } from '@mui/system'

const Cell: FC<{
  flexGrow?: number,
  grow?: boolean,
  breakpoint?: Breakpoint,
  sx?: SxProps,
}> = ({
  flexGrow = 0,
  grow = false,
  breakpoint,
  sx = {},
  children,
}) => {
  const theme = useTheme()
  const isLarge = useMediaQuery(theme.breakpoints.up(breakpoint || 'md'))

  // this is when the screen is small and the user has given a breakpoint
  if(breakpoint && !isLarge) {
    return (
      <Box
        sx={Object.assign({}, sx, {
          flexBasis: '100%',
        })}
      >
        { children }
      </Box>
    )
  }

  return (
    <Box
      sx={Object.assign({}, sx, {
        flexGrow: grow ? 1 : flexGrow
      })}
    >
      { children }
    </Box>
  )
}

export default Cell
