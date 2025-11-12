import { useState, useEffect, useCallback } from 'react'

interface UseResizeProps {
  initialSize: { width: number; height: number }
  minSize?: { width: number; height: number }
  maxSize?: { width: number; height: number }
  onResize?: (size: { width: number; height: number }, direction: string, delta: { x: number; y: number }) => void
}

interface ResizeHandle {
  direction: 'n' | 'ne' | 'e' | 'se' | 's' | 'sw' | 'w' | 'nw'
  onMouseDown: (e: React.MouseEvent) => void
  style: React.CSSProperties
}

export const useResize = ({
  initialSize,
  minSize = { width: 300, height: 200 },
  maxSize = { width: window.innerWidth - 40, height: window.innerHeight - 40 },
  onResize
}: UseResizeProps) => {
  const [size, setSize] = useState(initialSize)
  const [isResizing, setIsResizing] = useState(false)
  const [resizeDirection, setResizeDirection] = useState<string>('')
  const [startSize, setStartSize] = useState({ width: 0, height: 0 })
  const [startMouse, setStartMouse] = useState({ x: 0, y: 0 })

  const handleMouseDown = useCallback((direction: string) => (e: React.MouseEvent) => {
    e.preventDefault()
    e.stopPropagation()
    
    setIsResizing(true)
    setResizeDirection(direction)
    setStartSize(size)
    setStartMouse({ x: e.clientX, y: e.clientY })
  }, [size])

  useEffect(() => {
    const handleMouseMove = (e: MouseEvent) => {
      if (!isResizing) return

      const deltaX = e.clientX - startMouse.x
      const deltaY = e.clientY - startMouse.y

      let newWidth = startSize.width
      let newHeight = startSize.height

      // Calculate new dimensions based on resize direction
      if (resizeDirection.includes('e')) {
        newWidth = startSize.width + deltaX
      }
      if (resizeDirection.includes('w')) {
        newWidth = startSize.width - deltaX
      }
      if (resizeDirection.includes('s')) {
        newHeight = startSize.height + deltaY
      }
      if (resizeDirection.includes('n')) {
        newHeight = startSize.height - deltaY
      }

      // Apply constraints
      newWidth = Math.max(minSize.width, Math.min(maxSize.width, newWidth))
      newHeight = Math.max(minSize.height, Math.min(maxSize.height, newHeight))

      const newSize = { width: newWidth, height: newHeight }
      setSize(newSize)
      onResize?.(newSize, resizeDirection, { x: deltaX, y: deltaY })
    }

    const handleMouseUp = () => {
      setIsResizing(false)
      setResizeDirection('')
    }

    if (isResizing) {
      document.addEventListener('mousemove', handleMouseMove)
      document.addEventListener('mouseup', handleMouseUp)
    }

    return () => {
      document.removeEventListener('mousemove', handleMouseMove)
      document.removeEventListener('mouseup', handleMouseUp)
    }
  }, [isResizing, resizeDirection, startSize, startMouse, minSize, maxSize, onResize])

  const getResizeHandles = useCallback((): ResizeHandle[] => {
    const handleSize = 12
    const handles: ResizeHandle[] = [
      {
        direction: 'n',
        onMouseDown: handleMouseDown('n'),
        style: {
          position: 'absolute',
          top: 0,
          left: handleSize,
          right: handleSize,
          height: handleSize,
          cursor: 'n-resize',
          backgroundColor: 'transparent',
        }
      },
      {
        direction: 'ne',
        onMouseDown: handleMouseDown('ne'),
        style: {
          position: 'absolute',
          top: 0,
          right: 0,
          width: handleSize,
          height: handleSize,
          cursor: 'ne-resize',
          backgroundColor: 'transparent',
        }
      },
      {
        direction: 'e',
        onMouseDown: handleMouseDown('e'),
        style: {
          position: 'absolute',
          top: handleSize,
          right: 0,
          bottom: handleSize,
          width: handleSize,
          cursor: 'e-resize',
          backgroundColor: 'transparent',
        }
      },
      {
        direction: 'se',
        onMouseDown: handleMouseDown('se'),
        style: {
          position: 'absolute',
          bottom: 0,
          right: 0,
          width: handleSize,
          height: handleSize,
          cursor: 'se-resize',
          backgroundColor: 'transparent',
        }
      },
      {
        direction: 's',
        onMouseDown: handleMouseDown('s'),
        style: {
          position: 'absolute',
          bottom: 0,
          left: handleSize,
          right: handleSize,
          height: handleSize,
          cursor: 's-resize',
          backgroundColor: 'transparent',
        }
      },
      {
        direction: 'sw',
        onMouseDown: handleMouseDown('sw'),
        style: {
          position: 'absolute',
          bottom: 0,
          left: 0,
          width: handleSize,
          height: handleSize,
          cursor: 'sw-resize',
          backgroundColor: 'transparent',
        }
      },
      {
        direction: 'w',
        onMouseDown: handleMouseDown('w'),
        style: {
          position: 'absolute',
          top: handleSize,
          left: 0,
          bottom: handleSize,
          width: handleSize,
          cursor: 'w-resize',
          backgroundColor: 'transparent',
        }
      },
      {
        direction: 'nw',
        onMouseDown: handleMouseDown('nw'),
        style: {
          position: 'absolute',
          top: 0,
          left: 0,
          width: handleSize,
          height: handleSize,
          cursor: 'nw-resize',
          backgroundColor: 'transparent',
        }
      }
    ]

    return handles
  }, [handleMouseDown])

  return {
    size,
    setSize,
    isResizing,
    getResizeHandles
  }
}
