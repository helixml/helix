import React, { FC } from 'react'

import SettingsIcon from '@mui/icons-material/Settings'
import VpnKeyIcon from '@mui/icons-material/VpnKey'
import ChatIcon from '@mui/icons-material/Chat'

import ContextSidebar, { ContextSidebarSection } from '../system/ContextSidebar'

interface AccountSidebarProps {
  activeTab?: string
  onTabChange?: (tab: string) => void
}

const AccountSidebar: FC<AccountSidebarProps> = ({ activeTab = 'general', onTabChange }) => {
  const handleNavigationClick = (tabValue: string) => {
    if (onTabChange) {
      onTabChange(tabValue)
    }
  }

  const sections: ContextSidebarSection[] = [
    {
      items: [
        {
          id: 'general',
          label: 'General Settings',
          icon: <SettingsIcon />,
          isActive: activeTab === 'general',
          onClick: () => handleNavigationClick('general')
        },
        {
          id: 'chat',
          label: 'Chat',
          icon: <ChatIcon />,
          isActive: activeTab === 'chat',
          onClick: () => handleNavigationClick('chat')
        },
        {
          id: 'api_keys',
          label: 'API Keys',
          icon: <VpnKeyIcon />,
          isActive: activeTab === 'api_keys',
          onClick: () => handleNavigationClick('api_keys')
        }
      ]
    }
  ]

  return (
    <ContextSidebar
      menuType="account"
      sections={sections}
      density="compact"
    />
  )
}

export default AccountSidebar
