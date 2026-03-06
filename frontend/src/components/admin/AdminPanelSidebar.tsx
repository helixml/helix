import React, { FC } from 'react'
import Typography from '@mui/material/Typography'

import ApiIcon from '@mui/icons-material/Api'
import DnsIcon from '@mui/icons-material/Dns'
import VpnKeyIcon from '@mui/icons-material/VpnKey'
import LinkIcon from '@mui/icons-material/Link'
import DirectionsRunIcon from '@mui/icons-material/DirectionsRun'
import ModelTrainingIcon from '@mui/icons-material/ModelTraining'
import SettingsIcon from '@mui/icons-material/Settings'
import DeveloperBoardIcon from '@mui/icons-material/DeveloperBoard'
import AttachMoneyIcon from '@mui/icons-material/AttachMoney'
import CodeIcon from '@mui/icons-material/Code'

import ContextSidebar, { ContextSidebarSection } from '../system/ContextSidebar'
import { UsersIcon, BuildingIcon } from 'lucide-react'

interface AdminPanelSidebarProps {
  activeTab?: string
  onTabChange?: (tab: string) => void
}

const AdminPanelSidebar: FC<AdminPanelSidebarProps> = ({ activeTab = 'llm_calls', onTabChange }) => {
  const currentTab = activeTab

  const handleNavigationClick = (tabValue: string) => {
    if (onTabChange) {
      onTabChange(tabValue)
    }
  }

  const sections: ContextSidebarSection[] = [
    {
      title: 'Analytics & Monitoring',
      items: [
        {
          id: 'llm_calls',
          label: 'LLM Calls',
          icon: <ApiIcon />,
          isActive: currentTab === 'llm_calls',
          onClick: () => handleNavigationClick('llm_calls')
        }
      ]
    },
    {
      title: 'Infrastructure',
      items: [
        {
          id: 'providers',
          label: 'Inference Providers',
          icon: <DnsIcon />,
          isActive: currentTab === 'providers',
          onClick: () => handleNavigationClick('providers')
        },
        {
          id: 'oauth_providers',
          label: 'OAuth Providers',
          icon: <VpnKeyIcon />,
          isActive: currentTab === 'oauth_providers',
          onClick: () => handleNavigationClick('oauth_providers')
        },
        {
          id: 'service_connections',
          label: 'Service Connections',
          icon: <LinkIcon />,
          isActive: currentTab === 'service_connections',
          onClick: () => handleNavigationClick('service_connections')
        },
        {
          id: 'runners',
          label: 'GPU Runners',
          icon: <DirectionsRunIcon />,
          isActive: currentTab === 'runners',
          onClick: () => handleNavigationClick('runners')
        },
        {
          id: 'agent_sandboxes',
          label: 'Agent Sandboxes',
          icon: <DeveloperBoardIcon />,
          isActive: currentTab === 'agent_sandboxes',
          onClick: () => handleNavigationClick('agent_sandboxes')
        }
      ]
    },
    {
      title: 'Models & Configuration',
      items: [
        {
          id: 'helix_models',
          label: 'Helix Models',
          icon: <ModelTrainingIcon />,
          isActive: currentTab === 'helix_models',
          onClick: () => handleNavigationClick('helix_models')
        },
        {
          id: 'pricing',
          label: 'Pricing',
          icon: <AttachMoneyIcon />,
          isActive: currentTab === 'pricing',
          onClick: () => handleNavigationClick('pricing')
        },
        {
          id: 'system_settings',
          label: 'System Settings',
          icon: <SettingsIcon />,
          isActive: currentTab === 'system_settings',
          onClick: () => handleNavigationClick('system_settings')
        }
      ]
    },
    {
      title: 'Code Intelligence',
      items: [
        {
          id: 'kodit',
          label: 'Kodit Repositories',
          icon: <CodeIcon />,
          isActive: currentTab === 'kodit',
          onClick: () => handleNavigationClick('kodit')
        }
      ]
    },
    {
      title: 'User Management',
      items: [
        {
          id: 'users',
          label: 'Users',
          icon: <UsersIcon />,
          isActive: currentTab === 'users',
          onClick: () => handleNavigationClick('users')
        },
        {
          id: 'orgs',
          label: 'Organizations',
          icon: <BuildingIcon />,
          isActive: currentTab === 'orgs',
          onClick: () => handleNavigationClick('orgs')
        }
      ]
    }
  ]

  return (
    <ContextSidebar 
      menuType="admin"
      sections={sections}
    />
  )
}

export default AdminPanelSidebar 