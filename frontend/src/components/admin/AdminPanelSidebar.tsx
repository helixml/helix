import React, { FC } from 'react'

import ApiIcon from '@mui/icons-material/Api'
import DnsIcon from '@mui/icons-material/Dns'
import VpnKeyIcon from '@mui/icons-material/VpnKey'
import LinkIcon from '@mui/icons-material/Link'
import DirectionsRunIcon from '@mui/icons-material/DirectionsRun'
import ModelTrainingIcon from '@mui/icons-material/ModelTraining'
import SettingsIcon from '@mui/icons-material/Settings'
import AttachMoneyIcon from '@mui/icons-material/AttachMoney'
import CodeIcon from '@mui/icons-material/Code'
import QueueIcon from '@mui/icons-material/Queue'

import ContextSidebar, { ContextSidebarSection } from '../system/ContextSidebar'
import { UsersIcon, BuildingIcon } from 'lucide-react'

interface AdminNavSection {
  title: string
  items: { id: string; label: string; icon: React.ReactNode }[]
}

const ADMIN_NAV: AdminNavSection[] = [
  {
    title: 'Analytics & Monitoring',
    items: [
      { id: 'llm_calls', label: 'LLM Calls', icon: <ApiIcon /> },
    ]
  },
  {
    title: 'Infrastructure',
    items: [
      { id: 'providers', label: 'Inference Providers', icon: <DnsIcon /> },
      { id: 'oauth_providers', label: 'OAuth Providers', icon: <VpnKeyIcon /> },
      { id: 'service_connections', label: 'Service Connections', icon: <LinkIcon /> },
      { id: 'runners', label: 'Runners', icon: <DirectionsRunIcon /> },
    ]
  },
  {
    title: 'Models & Configuration',
    items: [
      { id: 'helix_models', label: 'Helix Models', icon: <ModelTrainingIcon /> },
      { id: 'runner_profiles', label: 'Runner Profiles', icon: <ModelTrainingIcon /> },
      { id: 'pricing', label: 'Pricing', icon: <AttachMoneyIcon /> },
      { id: 'system_settings', label: 'System Settings', icon: <SettingsIcon /> },
    ]
  },
  {
    title: 'Code Intelligence',
    items: [
      { id: 'kodit', label: 'Kodit Repositories', icon: <CodeIcon /> },
      { id: 'kodit_queue', label: 'Kodit Queue', icon: <QueueIcon /> },
    ]
  },
  {
    title: 'User Management',
    items: [
      { id: 'users', label: 'Users', icon: <UsersIcon /> },
      { id: 'orgs', label: 'Organizations', icon: <BuildingIcon /> },
    ]
  }
]

export const getAdminTabLabel = (tab: string): string =>
  ADMIN_NAV.flatMap((section) => section.items).find((item) => item.id === tab)?.label ?? 'Admin'

interface AdminPanelSidebarProps {
  activeTab?: string
  onTabChange?: (tab: string) => void
}

const AdminPanelSidebar: FC<AdminPanelSidebarProps> = ({ activeTab = 'llm_calls', onTabChange }) => {
  const sections: ContextSidebarSection[] = ADMIN_NAV.map((section) => ({
    title: section.title,
    items: section.items.map((item) => ({
      ...item,
      isActive: activeTab === item.id,
      onClick: () => onTabChange?.(item.id),
    })),
  }))

  return (
    <ContextSidebar 
      menuType="admin"
      sections={sections}
      density="compact"
    />
  )
}

export default AdminPanelSidebar 