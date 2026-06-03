import { FC } from 'react'
import { Network } from 'lucide-react'

import useRouter from '../../hooks/useRouter'
import useAccount from '../../hooks/useAccount'
import ContextSidebar, { ContextSidebarSection } from '../system/ContextSidebar'

// HelixOrgSidebar is the secondary navigation column for the
// helix-org alpha. Sits between the primary org-menu rail and the
// page body. Today the only surface is the chart; future Settings /
// Streams / Audit pages slot in here without touching the page
// components.
const HelixOrgSidebar: FC = () => {
  const router = useRouter()
  const account = useAccount()
  const currentRouteName = router.name
  const orgId = router.params.org_id

  const navigateTo = (routeName: string) => {
    if (orgId) {
      router.navigate(routeName, { org_id: orgId })
    }
    account.setMobileMenuOpen(false)
  }

  const sections: ContextSidebarSection[] = [
    {
      items: [
        {
          id: 'chart',
          label: 'Chart',
          icon: <Network size={18} />,
          isActive: currentRouteName === 'helix_org_chart' || currentRouteName === 'helix_org_root',
          onClick: () => navigateTo('helix_org_chart'),
        },
      ],
    },
  ]

  return <ContextSidebar menuType="orgs" sections={sections} />
}

export default HelixOrgSidebar
