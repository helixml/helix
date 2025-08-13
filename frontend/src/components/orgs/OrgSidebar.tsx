import { FC } from 'react'

import { User, Users, CreditCard, Settings } from 'lucide-react'

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
      title: 'Organization Management',
      items: [
        {
          id: 'people',
          label: 'People',
          icon: <User size={20} />,
          isActive: currentRouteName === 'org_people',
          onClick: () => handleNavigationClick('org_people')
        },
        {
          id: 'teams',
          label: 'Teams',
          icon: <Users size={20} />,
          isActive: currentRouteName === 'org_teams',
          onClick: () => handleNavigationClick('org_teams')
        },
        {
          id: 'billing',
          label: 'Billing',
          icon: <CreditCard size={20} />,
          isActive: currentRouteName === 'org_billing',
          onClick: () => handleNavigationClick('org_billing')
        },
        {
          id: 'settings',
          label: 'Settings',
          icon: <Settings size={20} />,
          isActive: currentRouteName === 'org_settings',
          onClick: () => handleNavigationClick('org_settings')
        }
      ]
    }
  ]

  return (
    <ContextSidebar
      menuType="orgs"
      sections={sections}
    />
  )
}

export default OrgSidebar 