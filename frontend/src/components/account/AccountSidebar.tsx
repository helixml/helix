import React, { FC } from 'react'

import ContextSidebar, { ContextSidebarSection } from '../system/ContextSidebar'
import { ACCOUNT_SECTIONS, DEFAULT_ACCOUNT_TAB } from './accountSections'

interface AccountSidebarProps {
  activeTab?: string
  onTabChange?: (tab: string) => void
}

const AccountSidebar: FC<AccountSidebarProps> = ({ activeTab = DEFAULT_ACCOUNT_TAB, onTabChange }) => {
  const sections: ContextSidebarSection[] = [
    {
      items: ACCOUNT_SECTIONS.map((section) => ({
        id: section.id,
        label: section.label,
        icon: section.icon,
        isActive: activeTab === section.id,
        onClick: () => onTabChange?.(section.id),
      })),
    },
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
