import React, { FC } from 'react'

import SettingsIcon from '@mui/icons-material/Settings'
import CodeIcon from '@mui/icons-material/Code'
import { Bot } from 'lucide-react'
import ViewKanbanIcon from '@mui/icons-material/ViewKanban'
import VpnKeyIcon from '@mui/icons-material/VpnKey'
import HubIcon from '@mui/icons-material/Hub'
import WarningIcon from '@mui/icons-material/Warning'

import ContextSidebar, { ContextSidebarSection } from '../system/ContextSidebar'

export type ProjectSettingsTab = 'general' | 'sandbox' | 'agents' | 'board' | 'secrets' | 'skills' | 'danger'

interface ProjectSettingsSidebarProps {
  activeTab?: ProjectSettingsTab
  onTabChange?: (tab: ProjectSettingsTab) => void
}

const ProjectSettingsSidebar: FC<ProjectSettingsSidebarProps> = ({ activeTab = 'general', onTabChange }) => {
  const handleClick = (tab: ProjectSettingsTab) => {
    if (onTabChange) {
      onTabChange(tab)
    }
  }

  const sections: ContextSidebarSection[] = [
    {
      items: [
        {
          id: 'general',
          label: 'General',
          icon: <SettingsIcon />,
          isActive: activeTab === 'general',
          onClick: () => handleClick('general'),
        },
        {
          id: 'sandbox',
          label: 'Sandbox',
          icon: <CodeIcon />,
          isActive: activeTab === 'sandbox',
          onClick: () => handleClick('sandbox'),
        },
        {
          id: 'agents',
          label: 'Agents',
          icon: <Bot size={22} />,
          isActive: activeTab === 'agents',
          onClick: () => handleClick('agents'),
        },
        {
          id: 'board',
          label: 'Board Automation',
          icon: <ViewKanbanIcon />,
          isActive: activeTab === 'board',
          onClick: () => handleClick('board'),
        },
        {
          id: 'secrets',
          label: 'Secrets',
          icon: <VpnKeyIcon />,
          isActive: activeTab === 'secrets',
          onClick: () => handleClick('secrets'),
        },
        {
          id: 'skills',
          label: 'Skills',
          icon: <HubIcon />,
          isActive: activeTab === 'skills',
          onClick: () => handleClick('skills'),
        },
      ],
    },
    {
      title: '',
      items: [
        {
          id: 'danger',
          label: 'Danger Zone',
          icon: <WarningIcon />,
          isActive: activeTab === 'danger',
          onClick: () => handleClick('danger'),
        },
      ],
    },
  ]

  return (
    <ContextSidebar
      menuType="project-settings"
      sections={sections}
      density="compact"
    />
  )
}

export default ProjectSettingsSidebar
