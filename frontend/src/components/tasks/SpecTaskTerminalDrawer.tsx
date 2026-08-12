import { FC, PointerEvent as ReactPointerEvent, useEffect, useRef, useState } from 'react'
import Box from '@mui/material/Box'

import SessionTerminal from '../session/SessionTerminal'
import {
  clampSpecTaskTerminalHeight,
  DEFAULT_SPEC_TASK_TERMINAL_HEIGHT,
} from './specTaskTerminalDrawerState'

interface Props {
  sessionId: string
  running: boolean
  height: number
  onHeightChange: (height: number) => void
  onClose: () => void
}

const SpecTaskTerminalDrawer: FC<Props> = ({
  sessionId,
  running,
  height,
  onHeightChange,
  onClose,
}) => {
  const [renderedHeight, setRenderedHeight] = useState(() =>
    clampSpecTaskTerminalHeight(height),
  )
  const renderedHeightRef = useRef(renderedHeight)
  const dragRef = useRef<{
    pointerId: number
    startY: number
    startHeight: number
  } | null>(null)

  useEffect(() => {
    if (dragRef.current) return
    const clamped = clampSpecTaskTerminalHeight(height)
    renderedHeightRef.current = clamped
    setRenderedHeight(clamped)
  }, [height])

  useEffect(() => {
    const handleWindowResize = () => {
      const clamped = clampSpecTaskTerminalHeight(renderedHeightRef.current)
      renderedHeightRef.current = clamped
      setRenderedHeight(clamped)
      onHeightChange(clamped)
    }
    window.addEventListener('resize', handleWindowResize)
    return () => window.removeEventListener('resize', handleWindowResize)
  }, [onHeightChange])

  const handlePointerDown = (event: ReactPointerEvent<HTMLDivElement>) => {
    if (event.button !== 0) return
    event.preventDefault()
    event.currentTarget.setPointerCapture(event.pointerId)
    dragRef.current = {
      pointerId: event.pointerId,
      startY: event.clientY,
      startHeight: renderedHeightRef.current,
    }
  }

  const handlePointerMove = (event: ReactPointerEvent<HTMLDivElement>) => {
    const drag = dragRef.current
    if (!drag || drag.pointerId !== event.pointerId) return
    event.preventDefault()
    const nextHeight = clampSpecTaskTerminalHeight(
      drag.startHeight + drag.startY - event.clientY,
    )
    renderedHeightRef.current = nextHeight
    setRenderedHeight(nextHeight)
  }

  const handlePointerEnd = (event: ReactPointerEvent<HTMLDivElement>) => {
    const drag = dragRef.current
    if (!drag || drag.pointerId !== event.pointerId) return
    dragRef.current = null
    if (event.currentTarget.hasPointerCapture(event.pointerId)) {
      event.currentTarget.releasePointerCapture(event.pointerId)
    }
    onHeightChange(renderedHeightRef.current)
  }

  return (
    <Box
      component="aside"
      aria-label="Task terminal drawer"
      sx={{
        position: 'relative',
        height: renderedHeight,
        flexShrink: 0,
        borderTop: '1px solid',
        borderColor: 'divider',
        backgroundColor: '#090909',
      }}
    >
      <Box
        role="separator"
        aria-label="Resize terminal drawer"
        aria-orientation="horizontal"
        onPointerDown={handlePointerDown}
        onPointerMove={handlePointerMove}
        onPointerUp={handlePointerEnd}
        onPointerCancel={handlePointerEnd}
        onDoubleClick={() => {
          const nextHeight = clampSpecTaskTerminalHeight(
            DEFAULT_SPEC_TASK_TERMINAL_HEIGHT,
          )
          renderedHeightRef.current = nextHeight
          setRenderedHeight(nextHeight)
          onHeightChange(nextHeight)
        }}
        sx={{
          position: 'absolute',
          top: -3,
          left: 0,
          right: 0,
          zIndex: 2,
          height: 7,
          cursor: 'row-resize',
          touchAction: 'none',
          '&::after': {
            content: '""',
            position: 'absolute',
            top: 3,
            left: 'calc(50% - 24px)',
            width: 48,
            height: 2,
            borderRadius: 1,
            backgroundColor: 'divider',
          },
        }}
      />
      <SessionTerminal
        key={sessionId}
        sessionId={sessionId}
        running={running}
        fillContainer
        onRequestClose={onClose}
      />
    </Box>
  )
}

export default SpecTaskTerminalDrawer
