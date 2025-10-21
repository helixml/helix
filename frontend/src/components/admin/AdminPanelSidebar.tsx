import React, { FC } from 'react'
import Typography from '@mui/material/Typography'

import ApiIcon from '@mui/icons-material/Api'
import DnsIcon from '@mui/icons-material/Dns'
import VpnKeyIcon from '@mui/icons-material/VpnKey'
import DirectionsRunIcon from '@mui/icons-material/DirectionsRun'
import ModelTrainingIcon from '@mui/icons-material/ModelTraining'
import SettingsIcon from '@mui/icons-material/Settings'

import useRouter from '../../hooks/useRouter'
import ContextSidebar, { ContextSidebarSection } from '../system/ContextSidebar'
import { UsersIcon } from 'lucide-react'

const AdminPanelSidebar: FC = () => {
  const router = useRouter()
  const { tab } = router.params
  const currentTab = tab || 'llm_calls'

  const handleNavigationClick = (tabValue: string) => {
    router.setParams({ tab: tabValue })  
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
          id: 'runners',
          label: 'GPU Runners',
          icon: <DirectionsRunIcon />,
          isActive: currentTab === 'runners',
          onClick: () => handleNavigationClick('runners')
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
          id: 'system_settings',
          label: 'System Settings',
          icon: <SettingsIcon />,
          isActive: currentTab === 'system_settings',
          onClick: () => handleNavigationClick('system_settings')
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