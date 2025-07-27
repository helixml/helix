import React, { FC } from 'react'
import Typography from '@mui/material/Typography'

import ApiIcon from '@mui/icons-material/Api'
import DnsIcon from '@mui/icons-material/Dns'
import VpnKeyIcon from '@mui/icons-material/VpnKey'
import DirectionsRunIcon from '@mui/icons-material/DirectionsRun'
import ModelTrainingIcon from '@mui/icons-material/ModelTraining'

import useRouter from '../../hooks/useRouter'
import ContextSidebar, { ContextSidebarSection } from '../system/ContextSidebar'

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