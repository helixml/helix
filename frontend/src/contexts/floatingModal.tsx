import React, { createContext, useContext, useState, ReactNode } from 'react'
import { TypesDashboardRunner } from '../api/api'

interface FloatingModalConfig {
  type: 'logs' | 'rdp' | 'exploratory_session'
  runner?: TypesDashboardRunner
  sessionId?: string
  wolfLobbyId?: string
  // Display settings for streaming (from app's ExternalAgentConfig)
  displayWidth?: number
  displayHeight?: number
  displayFps?: number
  // Prompt history sync (for exploratory_session)
  specTaskId?: string
  projectId?: string
}

interface FloatingModalContextType {
  isVisible: boolean
  modalConfig: FloatingModalConfig | null
  clickPosition: { x: number; y: number } | null
  showFloatingModal: (config: FloatingModalConfig, clickPosition?: { x: number; y: number }) => void
  hideFloatingModal: () => void
}

const FloatingModalContext = createContext<FloatingModalContextType | undefined>(undefined)

export const useFloatingModal = (): FloatingModalContextType => {
  const context = useContext(FloatingModalContext)
  if (!context) {
    throw new Error('useFloatingModal must be used within a FloatingModalProvider')
  }
  return context
}

interface FloatingModalProviderProps {
  children: ReactNode
}

export const FloatingModalProvider: React.FC<FloatingModalProviderProps> = ({ children }) => {
  const [isVisible, setIsVisible] = useState(false)
  const [modalConfig, setModalConfig] = useState<FloatingModalConfig | null>(null)
  const [clickPosition, setClickPosition] = useState<{ x: number; y: number } | null>(null)

  const showFloatingModal = (config: FloatingModalConfig, clickPos?: { x: number; y: number }) => {
    setIsVisible(true)
    setModalConfig(config)
    if (clickPos) {
      setClickPosition(clickPos)
    }
  }
  
  const hideFloatingModal = () => {
    setIsVisible(false)
    setModalConfig(null)
    setClickPosition(null)
  }

  const value: FloatingModalContextType = {
    isVisible,
    modalConfig,
    clickPosition,
    showFloatingModal,
    hideFloatingModal,
  }

  return (
    <FloatingModalContext.Provider value={value}>
      {children}
    </FloatingModalContext.Provider>
  )
}
