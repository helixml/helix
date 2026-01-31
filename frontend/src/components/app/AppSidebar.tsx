import { FC } from 'react'

import {
  Settings,
  Palette,
  Webhook,
  LibraryBig,
  Lightbulb,
  Bug,
  Key,
  Code,
  ChartArea,
  CloudDownload,
  Users,
  Brain,
} from 'lucide-react'

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
          icon: <Palette size={20} /> ,
          isActive: currentTab === 'appearance',
          onClick: () => handleNavigationClick('appearance')
        },
        {
          id: 'settings',
          label: 'Settings',
          icon: <Settings size={20} />,
          isActive: currentTab === 'settings',
          onClick: () => handleNavigationClick('settings')
        },
        {
          id: 'triggers',
          label: 'Triggers',
          icon: <Webhook size={20} />,
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
          icon: <LibraryBig size={20} />,
          isActive: currentTab === 'knowledge',
          onClick: () => handleNavigationClick('knowledge')
        },
        {
          id: 'skills',
          label: 'Skills',
          icon: <Lightbulb size={20} />,
          isActive: currentTab === 'skills',
          onClick: () => handleNavigationClick('skills')
        },
        {
          id: 'tests',
          label: 'Tests',
          icon: <Bug size={20} />,
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
          icon: <Key size={20} />,
          isActive: currentTab === 'apikeys',
          onClick: () => handleNavigationClick('apikeys')
        },
        {
          id: 'mcp',
          label: 'MCP',
          icon: <Code size={20} />,
          isActive: currentTab === 'mcp',
          onClick: () => handleNavigationClick('mcp')
        },
        {
          id: 'usage',
          label: 'Usage',
          icon: <ChartArea size={20} />,
          isActive: currentTab === 'usage',
          onClick: () => handleNavigationClick('usage')
        },
        {
          id: 'memories',
          label: 'Memories',
          icon: <Brain size={20} />,
          isActive: currentTab === 'memories',
          onClick: () => handleNavigationClick('memories')
        },
        {
          id: 'developers',
          label: 'Export',
          icon: < CloudDownload size={20} />,
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
          icon: <Users size={20} />,
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