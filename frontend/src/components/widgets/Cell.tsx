import React, { FC, useMemo } from 'react'
import { useTheme, Breakpoint, SxProps } from '@mui/material/styles'
import useMediaQuery from '@mui/material/useMediaQuery'
import Box from '@mui/material/Box'

const Cell: FC<{
  id?: string,
  flexGrow?: number,
  grow?: boolean,
  end?: boolean,
  breakpoint?: Breakpoint,
  sx?: SxProps,
}> = ({
  id,
  flexGrow = 0,
  grow = false,
  end = false,
  breakpoint,
  sx = {},
  children,
}) => {
  const theme = useTheme()
  const isLarge = useMediaQuery(theme.breakpoints.up(breakpoint || 'md'))

  const useFlexGrow = useMemo(() => {
    const useSx = sx as any
    if(useSx.flexGrow !== undefined) return useSx.flexGrow
    if(grow) return 1
    return flexGrow
  }, [
    flexGrow,
    grow,
    sx,
  ])

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

  const useSx: any = Object.assign({}, sx, {
    flexGrow: useFlexGrow,
  })

  if(end) {
    useSx.justifyContent = 'flex-end'
    useSx.textAlign = 'right'
  }

  return (
    <Box
      id={ id }
      sx={ useSx }
    >
      { children }
    </Box>
  )
}

export default Cell
