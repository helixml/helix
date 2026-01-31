import React, { FC, useState, useRef, useCallback, ReactNode } from 'react'
import Box from '@mui/material/Box'
import { SxProps } from '@mui/system'

const ScrollingLoader: FC<{
  sx?: SxProps,
  onLoad: {
    (): Promise<void>,
  },
  children?: ReactNode,
}> = ({
  sx = {},
  onLoad,
  children,
}) => {
  const [isLoading, setIsLoading] = useState(false)
  const divRef = useRef<HTMLDivElement>(null)

  const loadApi = useCallback(async () => {
    if (isLoading) return
    setIsLoading(true)
    await onLoad()
    setIsLoading(false)
  }, [
    isLoading,
    onLoad,
  ])

  const handleScroll = useCallback(() => {
    if(!divRef.current) return
    const { scrollTop, scrollHeight, clientHeight } = divRef.current
    if (scrollTop + clientHeight === scrollHeight) {
      loadApi()
    }
  }, [
    loadApi,
  ])

  return (
    <Box
      ref={ divRef }
      sx={ sx }
      onScroll={ handleScroll }
    >
      { children }
    </Box>
  )
}

export default ScrollingLoader