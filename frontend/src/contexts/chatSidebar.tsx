import React, { createContext, FC, ReactNode, useContext } from 'react'

type ChatSidebarContextValue = {
  collapsed: boolean
  collapse: () => void
  expand: () => void
}

const ChatSidebarContext = createContext<ChatSidebarContextValue>({
  collapsed: false,
  collapse: () => undefined,
  expand: () => undefined,
})

export const ChatSidebarProvider: FC<ChatSidebarContextValue & { children: ReactNode }> = ({
  collapsed,
  collapse,
  expand,
  children,
}) => (
  <ChatSidebarContext.Provider value={{ collapsed, collapse, expand }}>
    {children}
  </ChatSidebarContext.Provider>
)

export const useChatSidebar = (): ChatSidebarContextValue => useContext(ChatSidebarContext)
