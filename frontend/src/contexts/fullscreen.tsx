/**
 * FullscreenContext - Provides tooltip configuration for fullscreen containers
 *
 * Problem: When an element goes fullscreen, portaled tooltips render to document.body
 * which is outside the fullscreen element, making them invisible.
 *
 * Solution: Components that go fullscreen wrap their content in FullscreenProvider,
 * which provides the container ref. Child components can use useFullscreenTooltipProps()
 * to get the correct slotProps for their tooltips.
 *
 * Usage:
 *
 * // In fullscreen container:
 * const containerRef = useRef<HTMLDivElement>(null);
 * <FullscreenProvider containerRef={containerRef}>
 *   <Box ref={containerRef}>
 *     <ChildComponent />
 *   </Box>
 * </FullscreenProvider>
 *
 * // In child component:
 * const tooltipProps = useFullscreenTooltipProps();
 * <Tooltip title="Hello" {...tooltipProps}>...</Tooltip>
 */

import React, { createContext, useContext, ReactNode, RefObject } from 'react'
import { TooltipProps } from '@mui/material'

interface FullscreenContextValue {
  containerRef: RefObject<HTMLElement | null> | null
  isFullscreen: boolean
}

const FullscreenContext = createContext<FullscreenContextValue>({
  containerRef: null,
  isFullscreen: false,
})

interface FullscreenProviderProps {
  children: ReactNode
  containerRef: RefObject<HTMLElement | null>
  isFullscreen?: boolean
}

/**
 * Wrap content inside a fullscreen container with this provider
 * to ensure tooltips render correctly in fullscreen mode
 */
export const FullscreenProvider: React.FC<FullscreenProviderProps> = ({
  children,
  containerRef,
  isFullscreen = false,
}) => {
  return (
    <FullscreenContext.Provider value={{ containerRef, isFullscreen }}>
      {children}
    </FullscreenContext.Provider>
  )
}

/**
 * Returns slotProps for Tooltip that work correctly in both
 * normal and fullscreen modes.
 *
 * When inside a FullscreenProvider with isFullscreen=true,
 * returns props that render the tooltip inside the container
 * instead of portaling to document.body.
 */
export function useFullscreenTooltipProps(): Partial<TooltipProps> {
  const { containerRef, isFullscreen } = useContext(FullscreenContext)

  if (isFullscreen && containerRef?.current) {
    return {
      slotProps: {
        popper: {
          disablePortal: true,
          container: containerRef.current,
          sx: { zIndex: 100001 },
        },
      },
    }
  }

  // In non-fullscreen mode, the theme's MuiTooltip defaultProps handle z-index
  // Return empty object to use theme defaults
  return {}
}

/**
 * Hook to get the fullscreen context value directly
 * for components that need more control
 */
export function useFullscreenContext(): FullscreenContextValue {
  return useContext(FullscreenContext)
}

export default FullscreenContext
