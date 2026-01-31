/**
 * Smart window positioning and sizing utilities for floating windows
 */

export interface WindowBounds {
  x: number
  y: number
  width: number
  height: number
}

/**
 * Get bounds of all existing floating windows on the page
 */
function getExistingWindowBounds(): WindowBounds[] {
  const existingWindows = Array.from(document.querySelectorAll('.MuiPaper-root')).filter(el => {
    const style = window.getComputedStyle(el)
    return style.position === 'absolute' || style.position === 'fixed'
  })

  return existingWindows.map(el => {
    const rect = el.getBoundingClientRect()
    return {
      x: rect.left,
      y: rect.top,
      width: rect.width,
      height: rect.height,
    }
  })
}

/**
 * Check if two rectangles overlap
 */
function rectanglesOverlap(a: WindowBounds, b: WindowBounds): boolean {
  return !(
    a.x + a.width < b.x ||
    b.x + b.width < a.x ||
    a.y + a.height < b.y ||
    b.y + b.height < a.y
  )
}

/**
 * Calculate smart initial position that avoids overlapping with existing windows
 */
export function getSmartInitialPosition(windowWidth: number, windowHeight: number): { x: number; y: number } {
  const existingWindows = getExistingWindowBounds()

  if (existingWindows.length === 0) {
    // First window - center it
    return {
      x: Math.max(50, (window.innerWidth - windowWidth) / 2),
      y: Math.max(50, (window.innerHeight - windowHeight) / 2),
    }
  }

  // Try cascade positioning first
  const cascadeOffset = 40
  let candidatePos = { x: 100, y: 100 }

  for (let i = 0; i < existingWindows.length + 1; i++) {
    candidatePos = {
      x: 100 + (i * cascadeOffset),
      y: 100 + (i * cascadeOffset),
    }

    const candidate: WindowBounds = {
      ...candidatePos,
      width: windowWidth,
      height: windowHeight,
    }

    // Check if this position would fit on screen
    if (
      candidate.x + candidate.width > window.innerWidth - 50 ||
      candidate.y + candidate.height > window.innerHeight - 50
    ) {
      break // Would go off screen
    }

    // Check if it overlaps with any existing window
    const hasOverlap = existingWindows.some(existing => rectanglesOverlap(candidate, existing))

    if (!hasOverlap) {
      return candidatePos
    }
  }

  // If cascade didn't work, try to find empty space on the right side
  const rightSideX = window.innerWidth / 2
  if (rightSideX + windowWidth < window.innerWidth - 50) {
    const rightCandidate: WindowBounds = {
      x: rightSideX,
      y: 100,
      width: windowWidth,
      height: windowHeight,
    }

    const hasOverlap = existingWindows.some(existing => rectanglesOverlap(rightCandidate, existing))
    if (!hasOverlap) {
      return { x: rightSideX, y: 100 }
    }
  }

  // Fallback: Just offset from top-left
  return { x: 100, y: 100 }
}

/**
 * Calculate smart size that fits available screen space
 */
export function getSmartInitialSize(
  preferredWidth: number,
  preferredHeight: number,
  minWidth: number = 600,
  minHeight: number = 400
): { width: number; height: number } {
  const existingWindows = getExistingWindowBounds()

  // If no existing windows, use preferred size
  if (existingWindows.length === 0) {
    return {
      width: Math.min(preferredWidth, window.innerWidth * 0.9),
      height: Math.min(preferredHeight, window.innerHeight * 0.9),
    }
  }

  // Calculate available space
  const maxWidth = window.innerWidth * 0.9
  const maxHeight = window.innerHeight * 0.9

  // Try preferred size first
  if (preferredWidth <= maxWidth && preferredHeight <= maxHeight) {
    return {
      width: preferredWidth,
      height: preferredHeight,
    }
  }

  // If screen is crowded, use smaller size
  if (existingWindows.length >= 2) {
    return {
      width: Math.max(minWidth, Math.min(preferredWidth * 0.8, maxWidth)),
      height: Math.max(minHeight, Math.min(preferredHeight * 0.8, maxHeight)),
    }
  }

  // Default to preferred size constrained by screen
  return {
    width: Math.max(minWidth, Math.min(preferredWidth, maxWidth)),
    height: Math.max(minHeight, Math.min(preferredHeight, maxHeight)),
  }
}
