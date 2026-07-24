import { FC } from 'react'
import HubOutlinedIcon from '@mui/icons-material/HubOutlined'
import { Bot, Network } from 'lucide-react'

import useRouter from '../../hooks/useRouter'
import useAccount from '../../hooks/useAccount'
import ContextSidebar, { ContextSidebarSection } from '../system/ContextSidebar'

// HelixOrgSidebar is the secondary navigation column for the
// helix-org alpha. Sits between the primary org-menu rail and the
// page body. Today: chart + bots + topics.
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

  const isBotsRoute =
    currentRouteName === 'helix_org_bots' || currentRouteName === 'helix_org_bot_detail'

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
        {
          id: 'bots',
          label: 'Bots',
          icon: <Bot size={18} />,
          isActive: isBotsRoute,
          onClick: () => navigateTo('helix_org_bots'),
        },
        {
          id: 'topics',
          label: 'Topics',
          // Same hub glyph as topic cards on the org chart / top nav.
          icon: <HubOutlinedIcon sx={{ fontSize: 18 }} />,
          isActive: currentRouteName === 'helix_org_topics',
          onClick: () => navigateTo('helix_org_topics'),
        },
      ],
    },
  ]

  return <ContextSidebar menuType="orgs" sections={sections} />
}

export default HelixOrgSidebar
