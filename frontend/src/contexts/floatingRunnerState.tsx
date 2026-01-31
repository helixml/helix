import React, { createContext, useContext, useState, ReactNode } from 'react'

interface FloatingRunnerStateContextType {
  isVisible: boolean
  clickPosition: { x: number; y: number } | null
  showFloatingRunnerState: (clickPosition?: { x: number; y: number }) => void
  hideFloatingRunnerState: () => void
  toggleFloatingRunnerState: (clickPosition?: { x: number; y: number }) => void
}

const FloatingRunnerStateContext = createContext<FloatingRunnerStateContextType | undefined>(undefined)

export const useFloatingRunnerState = (): FloatingRunnerStateContextType => {
  const context = useContext(FloatingRunnerStateContext)
  if (!context) {
    throw new Error('useFloatingRunnerState must be used within a FloatingRunnerStateProvider')
  }
  return context
}

interface FloatingRunnerStateProviderProps {
  children: ReactNode
}

export const FloatingRunnerStateProvider: React.FC<FloatingRunnerStateProviderProps> = ({ children }) => {
  const [isVisible, setIsVisible] = useState(false)
  const [clickPosition, setClickPosition] = useState<{ x: number; y: number } | null>(null)

  const showFloatingRunnerState = (clickPos?: { x: number; y: number }) => {
    setIsVisible(true)
    if (clickPos) {
      setClickPosition(clickPos)
    }
  }
  
  const hideFloatingRunnerState = () => {
    setIsVisible(false)
    setClickPosition(null)
  }
  
  const toggleFloatingRunnerState = (clickPos?: { x: number; y: number }) => {
    if (isVisible) {
      hideFloatingRunnerState()
    } else {
      showFloatingRunnerState(clickPos)
    }
  }

  const value: FloatingRunnerStateContextType = {
    isVisible,
    clickPosition,
    showFloatingRunnerState,
    hideFloatingRunnerState,
    toggleFloatingRunnerState,
  }

  return (
    <FloatingRunnerStateContext.Provider value={value}>
      {children}
    </FloatingRunnerStateContext.Provider>
  )
} 