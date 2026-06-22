import { FC } from 'react'

import {
  SlidersHorizontal,
  User,
  Users,
  CreditCard,
  BarChart as ChartIcon,
  KeyRound,
  Plug,
} from 'lucide-react'

import useRouter from '../../hooks/useRouter'
import useAccount from '../../hooks/useAccount'
import ContextSidebar, { ContextSidebarSection } from '../system/ContextSidebar'

const OrgSidebar: FC = () => {
  const router = useRouter()
  const account = useAccount()
  const currentRouteName = router.name
  const orgId = router.params.org_id

  const handleNavigationClick = (routeName: string) => {
    if (orgId) {
      router.navigate(routeName, { org_id: orgId })
    }
    account.setMobileMenuOpen(false)
  }

  const sections: ContextSidebarSection[] = [
    {
      title: 'Settings',
      items: [
        {
          id: 'general',
          label: 'General',
          icon: <SlidersHorizontal size={20} />,
          isActive: currentRouteName === 'org_general' || currentRouteName === 'org_settings',
          onClick: () => handleNavigationClick('org_general'),
        },
      ],
    },
    {
      title: 'Members',
      items: [
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
      ],
    },
    {
      title: 'Cost',
      items: [
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
      ],
    },
    {
      title: 'Access',
      items: [
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
          isActive: currentRouteName === 'org_providers',
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
