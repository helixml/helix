import { FC } from 'react'

import {
  Settings,
  Cpu,
  Webhook,
  LibraryBig,
  Lightbulb,
  Bug,
  FlaskConical,
  Key,
  Code,
  ChartArea,
  CloudDownload,
  Users,
  Brain,
} from 'lucide-react'

import useRouter from '../../hooks/useRouter'
import useApp from '../../hooks/useApp'
import { isOrgAgent, usesFocusedAgentDetails } from '../../utils/apps'
import ContextSidebar, { ContextSidebarSection } from '../system/ContextSidebar'

const AppSidebar: FC = () => {
  const router = useRouter()
  const { tab, app_id } = router.params
  const currentTab = !tab || ['settings', 'instructions', 'appearance'].includes(tab) ? 'general' : tab
  
  // Get app data and user access information
  const appTools = useApp(app_id)
  const { userAccess, app } = appTools

  if (!app || usesFocusedAgentDetails(app)) return null

  const handleNavigationClick = (tabValue: string) => {
    router.setParams({ tab: tabValue })
  }

  const items: ContextSidebarSection['items'] = [
    {
      id: 'general',
      label: 'General',
      icon: <Settings size={20} />,
      isActive: currentTab === 'general',
      onClick: () => handleNavigationClick('general')
    },
    {
      id: 'runtime',
      label: 'Runtime',
      icon: <Cpu size={20} />,
      isActive: currentTab === 'runtime',
      onClick: () => handleNavigationClick('runtime')
    },
    {
      id: 'triggers',
      label: 'Triggers',
      icon: <Webhook size={20} />,
      isActive: currentTab === 'triggers',
      onClick: () => handleNavigationClick('triggers')
    },
    {
      id: 'knowledge',
      label: 'Knowledge',
      icon: <LibraryBig size={20} />,
      isActive: currentTab === 'knowledge',
      onClick: () => handleNavigationClick('knowledge')
    },
    {
      id: 'skills',
      label: app && isOrgAgent(app) ? 'MCPs & APIs' : 'Tools',
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
    },
    {
      id: 'evaluation',
      label: 'Evaluation',
      icon: <FlaskConical size={20} />,
      isActive: currentTab === 'evaluation',
      onClick: () => handleNavigationClick('evaluation')
    },
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
      icon: <CloudDownload size={20} />,
      isActive: currentTab === 'developers',
      onClick: () => handleNavigationClick('developers')
    }
  ]

  const sections: ContextSidebarSection[] = [{ items }]

  if (app?.organization_id && (userAccess?.isAdmin || isOrgAgent(app))) {
    items.push({
      id: 'access',
      label: 'Access',
      icon: <Users size={20} />,
      isActive: currentTab === 'access',
      onClick: () => handleNavigationClick('access')
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
