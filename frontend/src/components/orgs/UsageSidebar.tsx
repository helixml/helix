import { FC } from 'react'
import Button from '@mui/material/Button'
import Typography from '@mui/material/Typography'
import { ChevronLeft, Coins, Cpu, Gauge, LayoutDashboard, Table2 } from 'lucide-react'

import useRouter from '../../hooks/useRouter'
import useAccount from '../../hooks/useAccount'
import ContextSidebar from '../system/ContextSidebar'
import { USAGE_SECTIONS, UsageSectionKey, usageSection, usageSectionRoute } from './usageSections'

const ICONS: Record<UsageSectionKey, JSX.Element> = {
  overview: <LayoutDashboard size={20} />,
  tokens: <Coins size={20} />,
  performance: <Gauge size={20} />,
  compute: <Cpu size={20} />,
  breakdowns: <Table2 size={20} />,
}

// UsageSidebar replaces the org settings sidebar inside the Usage area, one
// level deeper: a way back to settings, then one entry per usage section.
const UsageSidebar: FC = () => {
  const router = useRouter()
  const account = useAccount()
  const orgId = router.params.org_id as string
  const active = router.name === 'org_usage' ? 'overview' : usageSection(router.params.section).key

  const go = (key: UsageSectionKey) => {
    const target = usageSectionRoute(key, orgId, window.location.search)
    router.navigate(target.name, target.params)
    account.setMobileMenuOpen(false)
  }

  return (
    <ContextSidebar
      menuType="org_usage"
      header={(
        <>
          <Button
            size="small"
            color="inherit"
            startIcon={<ChevronLeft size={16} />}
            onClick={() => {
              router.navigate('org_general', { org_id: orgId })
              account.setMobileMenuOpen(false)
            }}
            sx={{ textTransform: 'none', color: 'text.secondary', ml: -1, mb: 0.5, '&:hover': { color: 'text.primary' } }}
          >
            Settings
          </Button>
          <Typography variant="subtitle1" sx={{ fontWeight: 600, lineHeight: 1.3 }}>Usage</Typography>
        </>
      )}
      sections={[{
        items: USAGE_SECTIONS.map(section => ({
          id: section.key,
          label: section.title,
          icon: ICONS[section.key],
          isActive: active === section.key,
          onClick: () => go(section.key),
        })),
      }]}
    />
  )
}

export default UsageSidebar
