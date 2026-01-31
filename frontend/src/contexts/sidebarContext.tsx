import React, { createContext, useContext, ReactNode } from 'react'

interface SidebarContextValue {
  userMenuHeight: number
}

const SidebarContext = createContext<SidebarContextValue>({
  userMenuHeight: 0
})

export const useSidebarContext = () => {
  return useContext(SidebarContext)
}

interface SidebarProviderProps {
  children: ReactNode
  userMenuHeight: number
}

export const SidebarProvider: React.FC<SidebarProviderProps> = ({ 
  children, 
  userMenuHeight 
}) => {
  return (
    <SidebarContext.Provider value={{ userMenuHeight }}>
      {children}
    </SidebarContext.Provider>
  )
}

export default SidebarContext 