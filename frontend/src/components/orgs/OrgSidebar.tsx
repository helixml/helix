import React, { FC } from 'react'

import GroupIcon from '@mui/icons-material/Group'
import SettingsIcon from '@mui/icons-material/Settings'
import TeamsIcon from '@mui/icons-material/Groups'

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
          icon: <GroupIcon />,
          isActive: currentRouteName === 'org_people',
          onClick: () => handleNavigationClick('org_people')
        },
        {
          id: 'teams',
          label: 'Teams',
          icon: <TeamsIcon />,
          isActive: currentRouteName === 'org_teams',
          onClick: () => handleNavigationClick('org_teams')
        },
        {
          id: 'settings',
          label: 'Settings',
          icon: <SettingsIcon />,
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