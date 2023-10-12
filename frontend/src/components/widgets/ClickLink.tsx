import React, { FC, useCallback, MouseEvent } from 'react'

import Box from '@mui/material/Box'

interface JobViewLinkProps {
  className?: string,
  textDecoration?: boolean,
  onClick: {
    (): void,
  },
}

const ClickLink: FC<React.PropsWithChildren<JobViewLinkProps>> = ({
  className,
  textDecoration = false,
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
      }}
    >
      { children }
    </Box>
  )
}

export default ClickLink