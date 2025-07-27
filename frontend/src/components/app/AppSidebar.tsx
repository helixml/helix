import React, { FC } from 'react'

import PaletteIcon from '@mui/icons-material/Palette'
import SettingsIcon from '@mui/icons-material/Settings'
import MenuBookIcon from '@mui/icons-material/MenuBook'
import EmojiObjectsIcon from '@mui/icons-material/EmojiObjects'
import VpnKeyIcon from '@mui/icons-material/VpnKey'
import CodeIcon from '@mui/icons-material/Code'
import BarChartIcon from '@mui/icons-material/BarChart'
import CloudDownloadIcon from '@mui/icons-material/CloudDownload'
import GroupIcon from '@mui/icons-material/Group'
import ApiIcon from '@mui/icons-material/Api'
import BugReportIcon from '@mui/icons-material/BugReport'

import useRouter from '../../hooks/useRouter'
import useApp from '../../hooks/useApp'
import ContextSidebar, { ContextSidebarSection } from '../system/ContextSidebar'

const AppSidebar: FC = () => {
  const router = useRouter()
  const { tab, app_id } = router.params
  const currentTab = tab || 'appearance'
  
  // Get app data and user access information
  const appTools = useApp(app_id)
  const { userAccess, app } = appTools

  const handleNavigationClick = (tabValue: string) => {
    router.setParams({ tab: tabValue })
  }

  const sections: ContextSidebarSection[] = [
    {
      title: 'Agent Configuration',
      items: [
        {
          id: 'appearance',
          label: 'Appearance',
          icon: <PaletteIcon />,
          isActive: currentTab === 'appearance',
          onClick: () => handleNavigationClick('appearance')
        },
        {
          id: 'settings',
          label: 'Settings',
          icon: <SettingsIcon />,
          isActive: currentTab === 'settings',
          onClick: () => handleNavigationClick('settings')
        },
        {
          id: 'triggers',
          label: 'Triggers',
          icon: <ApiIcon />,
          isActive: currentTab === 'triggers',
          onClick: () => handleNavigationClick('triggers')
        }
      ]
    },
    {
      title: 'Agent Capabilities',
      items: [
        {
          id: 'knowledge',
          label: 'Knowledge',
          icon: <MenuBookIcon />,
          isActive: currentTab === 'knowledge',
          onClick: () => handleNavigationClick('knowledge')
        },
        {
          id: 'skills',
          label: 'Skills',
          icon: <EmojiObjectsIcon />,
          isActive: currentTab === 'skills',
          onClick: () => handleNavigationClick('skills')
        },
        {
          id: 'tests',
          label: 'Tests',
          icon: <BugReportIcon />,
          isActive: currentTab === 'tests',
          onClick: () => handleNavigationClick('tests')
        }
      ]
    },
    {
      title: 'Integration & Management',
      items: [
        {
          id: 'apikeys',
          label: 'Keys',
          icon: <VpnKeyIcon />,
          isActive: currentTab === 'apikeys',
          onClick: () => handleNavigationClick('apikeys')
        },
        {
          id: 'mcp',
          label: 'MCP',
          icon: <CodeIcon />,
          isActive: currentTab === 'mcp',
          onClick: () => handleNavigationClick('mcp')
        },
        {
          id: 'usage',
          label: 'Usage',
          icon: <BarChartIcon />,
          isActive: currentTab === 'usage',
          onClick: () => handleNavigationClick('usage')
        },
        {
          id: 'developers',
          label: 'Export',
          icon: <CloudDownloadIcon />,
          isActive: currentTab === 'developers',
          onClick: () => handleNavigationClick('developers')
        }
      ]
    }
  ]

  // Add access management section if user is admin and organization exists
  if (app?.organization_id && userAccess?.isAdmin) {
    sections.push({
      title: 'Administration',
      items: [
        {
          id: 'access',
          label: 'Access',
          icon: <GroupIcon />,
          isActive: currentTab === 'access',
          onClick: () => handleNavigationClick('access')
        }
      ]
    })
  }

  return (
    <ContextSidebar
      menuType="app"
      sections={sections}
    />
  )
}

export default AppSidebar 