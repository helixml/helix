import React, { FC, useCallback, MouseEvent } from 'react'
import { SxProps } from '@mui/system'
import Box from '@mui/material/Box'

interface JobViewLinkProps {
  className?: string,
  textDecoration?: boolean,
  sx?: SxProps,
  onClick: {
    (): void,
  },
}

const ClickLink: FC<React.PropsWithChildren<JobViewLinkProps>> = ({
  className,
  textDecoration = false,
  sx = {},
  onClick,
  children,
}) => {

  const onOpen = useCallback((e: MouseEvent) => {
    e.preventDefault()
    e.stopPropagation()
    onClick()
    return false
  }, [
    onClick,
  ])

  return (
    <Box
      component={'a'}
      href='#'
      onClick={ onOpen }
      className={ className }
      sx={{
        color: 'primary.main',
        textDecoration: textDecoration ? 'underline' : 'none',
        ...sx
      }}
    >
      { children }
    </Box>
  )
}

export default ClickLink