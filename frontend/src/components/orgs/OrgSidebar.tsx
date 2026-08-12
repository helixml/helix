import { FC } from 'react'

import {
  SlidersHorizontal,
  User,
  Users,
  CreditCard,
  BarChart as ChartIcon,
  KeyRound,
  Plug,
  GitBranch,
  FileText,
} from 'lucide-react'

import useRouter from '../../hooks/useRouter'
import useAccount from '../../hooks/useAccount'
import ContextSidebar, { ContextSidebarSection } from '../system/ContextSidebar'

const OrgSidebar: FC = () => {
  const router = useRouter()
  const account = useAccount()
  const currentRouteName = router.name
  const orgId = router.params.org_id

  const handleNavigationClick = (routeName: string, params: Record<string, string> = {}) => {
    if (orgId) {
      router.navigate(routeName, { org_id: orgId, ...params })
    }
    account.setMobileMenuOpen(false)
  }

  const sections: ContextSidebarSection[] = [
    {
      items: [
        {
          id: 'general',
          label: 'General',
          icon: <SlidersHorizontal size={20} />,
          isActive: currentRouteName === 'org_general' || currentRouteName === 'org_settings',
          onClick: () => handleNavigationClick('org_general'),
        },
        {
          id: 'people',
          label: 'People',
          icon: <User size={20} />,
          isActive: currentRouteName === 'org_people',
          onClick: () => handleNavigationClick('org_people'),
        },
        {
          id: 'teams',
          label: 'Teams',
          icon: <Users size={20} />,
          isActive: currentRouteName === 'org_teams',
          onClick: () => handleNavigationClick('org_teams'),
        },
        {
          id: 'repositories',
          label: 'Repositories',
          icon: <GitBranch size={20} />,
          isActive: currentRouteName === 'org_projects' && router.params.tab === 'repositories',
          onClick: () => handleNavigationClick('org_projects', { tab: 'repositories' }),
        },
        {
          id: 'guidelines',
          label: 'Guidelines',
          icon: <FileText size={20} />,
          isActive: currentRouteName === 'org_projects' && router.params.tab === 'guidelines',
          onClick: () => handleNavigationClick('org_projects', { tab: 'guidelines' }),
        },
        {
          id: 'billing',
          label: 'Billing',
          icon: <CreditCard size={20} />,
          isActive: currentRouteName === 'org_billing',
          onClick: () => handleNavigationClick('org_billing'),
        },
        {
          id: 'usage',
          label: 'Usage',
          icon: <ChartIcon size={20} />,
          isActive: currentRouteName === 'org_usage',
          onClick: () => handleNavigationClick('org_usage'),
        },
        {
          id: 'api_keys',
          label: 'API Keys',
          icon: <KeyRound size={20} />,
          isActive: currentRouteName === 'org_api_keys',
          onClick: () => handleNavigationClick('org_api_keys'),
        },
        {
          id: 'providers',
          label: 'Providers',
          icon: <Plug size={20} />,
          isActive: currentRouteName === 'org_providers' || currentRouteName === 'org_provider_detail',
          onClick: () => handleNavigationClick('org_providers'),
        },
      ],
    },
  ]

  return (
    <ContextSidebar
      menuType="orgs"
      sections={sections}
    />
  )
}

export default OrgSidebar
