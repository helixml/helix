export const chatSidebarCollapsedStorageKey = (orgId: string): string => (
  `helix:project-chat-sidebar:hidden:${orgId}`
)

export const parseChatSidebarCollapsed = (storedValue: string | null): boolean => (
  storedValue === 'true'
)
