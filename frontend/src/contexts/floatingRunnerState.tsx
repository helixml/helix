import React, { createContext, useContext, useState, ReactNode } from 'react'

interface FloatingRunnerStateContextType {
  isVisible: boolean
  showFloatingRunnerState: () => void
  hideFloatingRunnerState: () => void
  toggleFloatingRunnerState: () => void
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

  const showFloatingRunnerState = () => setIsVisible(true)
  const hideFloatingRunnerState = () => setIsVisible(false)
  const toggleFloatingRunnerState = () => setIsVisible(prev => !prev)

  const value: FloatingRunnerStateContextType = {
    isVisible,
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